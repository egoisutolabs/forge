package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/egoisutolabs/forge/internal/config"
)

func TestDetectProviders_EnvVars(t *testing.T) {
	// Clear all provider env vars first.
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")

	providers := DetectProviders(nil)

	found := make(map[string]bool)
	for _, p := range providers {
		found[p.Name] = true
	}

	if !found["anthropic"] {
		t.Error("expected anthropic provider from ANTHROPIC_API_KEY")
	}
	if !found["openai"] {
		t.Error("expected openai provider from OPENAI_API_KEY")
	}
	if found["groq"] {
		t.Error("groq should not be detected without GROQ_API_KEY")
	}
}

func TestDetectProviders_ConfigProviders(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic", APIKey: "cfg-key", Models: []string{"claude-sonnet-4-6"}},
		},
	}

	providers := DetectProviders(cfg)

	if len(providers) == 0 {
		t.Fatal("expected at least one provider from config")
	}
	if providers[0].Name != "anthropic" {
		t.Errorf("first provider = %q, want anthropic", providers[0].Name)
	}
	// Config models should be used.
	if len(providers[0].Models) != 1 || providers[0].Models[0] != "claude-sonnet-4-6" {
		t.Errorf("config provider models = %v, want [claude-sonnet-4-6]", providers[0].Models)
	}
}

func TestDetectProviders_ConfigWithoutAPIKey(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic", Models: []string{"claude-sonnet-4-6"}}, // no API key
		},
	}

	providers := DetectProviders(cfg)
	for _, p := range providers {
		if p.Name == "anthropic" {
			t.Error("should not detect anthropic provider without API key in config")
		}
	}
}

func TestDetectProviders_Ollama(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	// Spin up a fake Ollama server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{
				{"name": "llama3.2:latest"},
				{"name": "codellama:7b"},
			},
		})
	}))
	defer srv.Close()

	// Override the HTTP client to hit our test server.
	origClient := ollamaHTTPClient
	ollamaHTTPClient = srv.Client()
	// Also need to override the URL - use a custom transport.
	ollamaHTTPClient = &http.Client{
		Transport: &rewriteTransport{base: srv.Client().Transport, url: srv.URL},
		Timeout:   origClient.Timeout,
	}
	defer func() { ollamaHTTPClient = origClient }()

	providers := DetectProviders(nil)

	found := false
	for _, p := range providers {
		if p.Name == "ollama" {
			found = true
			if len(p.Models) != 2 {
				t.Errorf("ollama models count = %d, want 2", len(p.Models))
			}
			// ":latest" should be stripped.
			if p.Models[0] != "llama3.2" {
				t.Errorf("ollama model[0] = %q, want llama3.2", p.Models[0])
			}
			if p.Models[1] != "codellama:7b" {
				t.Errorf("ollama model[1] = %q, want codellama:7b", p.Models[1])
			}
		}
	}
	if !found {
		t.Error("expected ollama provider from fake server")
	}
}

func TestDetectProviders_NoDuplicates(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	// Both config and env have anthropic.
	t.Setenv("ANTHROPIC_API_KEY", "env-key")
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic", APIKey: "cfg-key", Models: []string{"claude-sonnet-4-6"}},
		},
	}

	providers := DetectProviders(cfg)
	count := 0
	for _, p := range providers {
		if p.Name == "anthropic" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("anthropic appeared %d times, want 1 (config should take priority)", count)
	}
}

func TestBundledModelsForProvider(t *testing.T) {
	models := bundledModelsForProvider("anthropic")
	if len(models) < 3 {
		t.Errorf("anthropic bundled models = %d, want >= 3", len(models))
	}

	models = bundledModelsForProvider("nonexistent")
	if len(models) != 0 {
		t.Errorf("nonexistent provider models = %d, want 0", len(models))
	}
}

func TestDetectProviders_AllEnvKeys(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	// Set all env keys.
	for _, key := range providerEnvKeys {
		t.Setenv(key, "test-key")
	}

	providers := DetectProviders(nil)
	found := make(map[string]bool)
	for _, p := range providers {
		found[p.Name] = true
	}

	for provName := range providerEnvKeys {
		// Some providers might not have bundled models (e.g., openrouter).
		models := bundledModelsForProvider(provName)
		if len(models) > 0 && !found[provName] {
			t.Errorf("expected provider %q to be detected with env var set", provName)
		}
	}
}

// rewriteTransport rewrites all HTTP request URLs to point at a test server.
type rewriteTransport struct {
	base http.RoundTripper
	url  string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.url[len("http://"):]
	if t.base != nil {
		return t.base.RoundTrip(req)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestDetectOllama_NoServer(t *testing.T) {
	// With default client pointing at localhost:11434, if nothing is running
	// we should get nil, not panic.
	origClient := ollamaHTTPClient
	ollamaHTTPClient = &http.Client{
		Transport: &rewriteTransport{url: fmt.Sprintf("http://127.0.0.1:%d", 19999)},
		Timeout:   origClient.Timeout,
	}
	defer func() { ollamaHTTPClient = origClient }()

	models := detectOllama()
	if models != nil {
		t.Errorf("detectOllama with no server should return nil, got %v", models)
	}
}

func TestDetectProviders_CustomFromConfig(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "myprovider",
				APIKey: "custom-key",
				Models: []string{"custom-model-a", "custom-model-b"},
			},
		},
	}

	providers := DetectProviders(cfg)

	found := false
	for _, p := range providers {
		if p.Name == "myprovider" {
			found = true
			if len(p.Models) != 2 {
				t.Errorf("myprovider models count = %d, want 2", len(p.Models))
			}
			if p.Models[0] != "custom-model-a" {
				t.Errorf("myprovider model[0] = %q, want custom-model-a", p.Models[0])
			}
			if p.Models[1] != "custom-model-b" {
				t.Errorf("myprovider model[1] = %q, want custom-model-b", p.Models[1])
			}
		}
	}
	if !found {
		t.Error("expected custom provider 'myprovider' to be detected")
	}
}

func TestDetectProviders_CustomNoAuth(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "localprovider",
				NoAuth: true,
				Models: []string{"local-model-1"},
			},
		},
	}

	providers := DetectProviders(cfg)

	found := false
	for _, p := range providers {
		if p.Name == "localprovider" {
			found = true
		}
	}
	if !found {
		t.Error("expected provider with NoAuth=true to be detected even without APIKey")
	}
}

func TestDetectProviders_CustomNoKeyNoAuth(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "brokenprovider",
				Models: []string{"some-model"},
			},
		},
	}

	providers := DetectProviders(cfg)
	for _, p := range providers {
		if p.Name == "brokenprovider" {
			t.Error("provider without APIKey and NoAuth=false should not be detected")
		}
	}
}

// setAllEnvKeysEmpty is a test helper — not used but kept for documentation.
func clearProviderEnvVars(t *testing.T) {
	t.Helper()
	for _, key := range providerEnvKeys {
		if err := os.Unsetenv(key); err != nil {
			t.Logf("warning: could not unset %s: %v", key, err)
		}
	}
}
