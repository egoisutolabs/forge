package provider

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoisutolabs/forge/config"
)

// AvailableProvider represents a detected provider with its available models.
type AvailableProvider struct {
	Name   string   // e.g. "anthropic", "openai", "ollama"
	Models []string // model names available through this provider
}

// providerEnvKeys maps provider names to the environment variable that signals
// their availability.
var providerEnvKeys = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"groq":       "GROQ_API_KEY",
	"google":     "GOOGLE_API_KEY",
	"mistral":    "MISTRAL_API_KEY",
	"xai":        "XAI_API_KEY",
}

// DetectProviders scans environment variables, auth.json, config providers,
// and local Ollama to return a list of available providers.
func DetectProviders(cfg *config.Config) []AvailableProvider {
	seen := make(map[string]bool)
	var result []AvailableProvider

	add := func(name string, models []string) {
		if seen[name] || len(models) == 0 {
			return
		}
		seen[name] = true
		result = append(result, AvailableProvider{Name: name, Models: models})
	}

	// 1. Check config providers first (highest priority — explicit user config).
	if cfg != nil {
		for _, p := range cfg.Providers {
			if len(p.Models) > 0 && (p.APIKey != "" || p.NoAuth) {
				add(p.Name, p.Models)
			}
		}
	}

	// 2. Check environment variables.
	for provName, envKey := range providerEnvKeys {
		if os.Getenv(envKey) != "" {
			models := bundledModelsForProvider(provName)
			add(provName, models)
		}
	}

	// 3. Check auth.json.
	if authProviders := detectFromAuthJSON(); len(authProviders) > 0 {
		for _, ap := range authProviders {
			add(ap.Name, ap.Models)
		}
	}

	// 4. Check Ollama.
	if ollamaModels := detectOllama(); len(ollamaModels) > 0 {
		add("ollama", ollamaModels)
	}

	return result
}

// bundledModelsForProvider returns model names from the bundled catalog that
// belong to the given provider.
func bundledModelsForProvider(providerName string) []string {
	var models []string
	for _, m := range bundledModels {
		if m.Provider == providerName {
			models = append(models, m.Name)
		}
	}
	return models
}

// detectFromAuthJSON reads ~/.forge/auth.json if it exists, looking for
// provider keys that are stored there.
func detectFromAuthJSON() []AvailableProvider {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".forge", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var auth map[string]string
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil
	}
	var result []AvailableProvider
	for provName, envKey := range providerEnvKeys {
		if key, ok := auth[envKey]; ok && key != "" {
			models := bundledModelsForProvider(provName)
			if len(models) > 0 {
				result = append(result, AvailableProvider{Name: provName, Models: models})
			}
		}
	}
	return result
}

// ollamaTagsResponse matches the JSON from GET /api/tags.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// ollamaHTTPClient is overridable for testing.
var ollamaHTTPClient = &http.Client{Timeout: 500 * time.Millisecond}

// detectOllama probes the local Ollama server for available models.
func detectOllama() []string {
	resp, err := ollamaHTTPClient.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil
	}
	models := make([]string, 0, len(tags.Models))
	for _, m := range tags.Models {
		// Strip ":latest" suffix that Ollama often includes.
		name := strings.TrimSuffix(m.Name, ":latest")
		models = append(models, name)
	}
	return models
}
