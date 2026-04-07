package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/internal/config"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	store := &AuthStore{
		Providers: map[string]ProviderAuth{
			"anthropic": {Type: "api_key", APIKey: "sk-ant-test123"},
			"openai":    {Type: "api_key", APIKey: "sk-openai-test456"},
		},
	}

	if err := store.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if len(loaded.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(loaded.Providers))
	}
	if loaded.Providers["anthropic"].APIKey != "sk-ant-test123" {
		t.Errorf("anthropic key = %q, want %q", loaded.Providers["anthropic"].APIKey, "sk-ant-test123")
	}
	if loaded.Providers["openai"].APIKey != "sk-openai-test456" {
		t.Errorf("openai key = %q, want %q", loaded.Providers["openai"].APIKey, "sk-openai-test456")
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	store := &AuthStore{
		Providers: map[string]ProviderAuth{
			"anthropic": {Type: "api_key", APIKey: "sk-test"},
		},
	}

	if err := store.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %04o, want 0600", perm)
	}
}

func TestLoadNonExistentReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if len(store.Providers) != 0 {
		t.Errorf("expected empty providers, got %d", len(store.Providers))
	}
}

func TestLoadMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	os.WriteFile(path, []byte("{bad json"), 0600)

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGetAPIKeyPrecedence(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Set up all three sources.
	// 1. auth.json
	store := &AuthStore{
		Providers: map[string]ProviderAuth{
			"anthropic": {Type: "api_key", APIKey: "from-auth-json"},
		},
	}
	store.SaveTo(authPath)

	// 2. config
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic", APIKey: "from-config"},
		},
	}

	// 3. env var
	t.Setenv("ANTHROPIC_API_KEY", "from-env")

	// auth.json should win.
	got := GetAPIKeyFrom("anthropic", authPath, cfg)
	if got != "from-auth-json" {
		t.Errorf("GetAPIKeyFrom = %q, want %q", got, "from-auth-json")
	}
}

func TestGetAPIKeyFallsToConfig(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json") // doesn't exist

	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "openai", APIKey: "from-config"},
		},
	}

	got := GetAPIKeyFrom("openai", authPath, cfg)
	if got != "from-config" {
		t.Errorf("GetAPIKeyFrom = %q, want %q", got, "from-config")
	}
}

func TestGetAPIKeyFallsToEnv(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json") // doesn't exist

	t.Setenv("OPENAI_API_KEY", "from-env")

	got := GetAPIKeyFrom("openai", authPath, nil)
	if got != "from-env" {
		t.Errorf("GetAPIKeyFrom = %q, want %q", got, "from-env")
	}
}

func TestGetAPIKeyReturnsEmptyWhenNone(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Clear any env vars.
	t.Setenv("GROQ_API_KEY", "")

	got := GetAPIKeyFrom("groq", authPath, nil)
	if got != "" {
		t.Errorf("GetAPIKeyFrom = %q, want empty", got)
	}
}

func TestSetAPIKeyCreatesFile(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nested", "dir")
	path := filepath.Join(subdir, "auth.json")

	err := SetAPIKeyIn(path, "anthropic", "sk-new-key")
	if err != nil {
		t.Fatalf("SetAPIKeyIn: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Verify content.
	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if store.Providers["anthropic"].APIKey != "sk-new-key" {
		t.Errorf("key = %q, want %q", store.Providers["anthropic"].APIKey, "sk-new-key")
	}
}

func TestSetAPIKeyUpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	// Set initial key.
	SetAPIKeyIn(path, "anthropic", "old-key")
	SetAPIKeyIn(path, "openai", "openai-key")

	// Update anthropic key.
	SetAPIKeyIn(path, "anthropic", "new-key")

	store, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if store.Providers["anthropic"].APIKey != "new-key" {
		t.Errorf("anthropic = %q, want %q", store.Providers["anthropic"].APIKey, "new-key")
	}
	// openai should be preserved.
	if store.Providers["openai"].APIKey != "openai-key" {
		t.Errorf("openai = %q, want %q", store.Providers["openai"].APIKey, "openai-key")
	}
}

func TestEnvVarMappingAllProviders(t *testing.T) {
	expected := map[string]string{
		"anthropic":  "ANTHROPIC_API_KEY",
		"openai":     "OPENAI_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
		"groq":       "GROQ_API_KEY",
		"google":     "GOOGLE_API_KEY",
		"mistral":    "MISTRAL_API_KEY",
		"xai":        "XAI_API_KEY",
		"deepinfra":  "DEEPINFRA_API_KEY",
	}

	for provider, wantEnv := range expected {
		gotEnv, ok := EnvVarForProvider(provider)
		if !ok {
			t.Errorf("EnvVarForProvider(%q) not found", provider)
			continue
		}
		if gotEnv != wantEnv {
			t.Errorf("EnvVarForProvider(%q) = %q, want %q", provider, gotEnv, wantEnv)
		}
	}
}

func TestEnvVarUnknownProvider(t *testing.T) {
	_, ok := EnvVarForProvider("unknown-provider")
	if ok {
		t.Error("EnvVarForProvider should return false for unknown provider")
	}
}

func TestGetEnvKey(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "mistral-test-key")

	got := GetEnvKey("mistral")
	if got != "mistral-test-key" {
		t.Errorf("GetEnvKey = %q, want %q", got, "mistral-test-key")
	}

	got = GetEnvKey("unknown")
	if got != "" {
		t.Errorf("GetEnvKey(unknown) = %q, want empty", got)
	}
}

func TestKnownProviders(t *testing.T) {
	providers := KnownProviders()
	if len(providers) != 8 {
		t.Errorf("KnownProviders() returned %d, want 8", len(providers))
	}

	// Verify sorted order.
	for i := 1; i < len(providers); i++ {
		if providers[i] < providers[i-1] {
			t.Errorf("KnownProviders not sorted: %q before %q", providers[i-1], providers[i])
		}
	}
}

func TestGetAuthSource(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	// Override global path for this test.
	origPath := storePath
	SetPath(authPath)
	defer SetPath(origPath)

	// No auth anywhere.
	t.Setenv("GROQ_API_KEY", "")
	src := GetAuthSource("groq", nil)
	if src != SourceNone {
		t.Errorf("expected SourceNone, got %q", src)
	}

	// Env var set.
	t.Setenv("GROQ_API_KEY", "from-env")
	src = GetAuthSource("groq", nil)
	if src != SourceEnvVar {
		t.Errorf("expected SourceEnvVar, got %q", src)
	}

	// Config set (takes precedence over env).
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "groq", APIKey: "from-config"},
		},
	}
	src = GetAuthSource("groq", cfg)
	if src != SourceConfig {
		t.Errorf("expected SourceConfig, got %q", src)
	}

	// Auth file set (takes precedence over all).
	SetAPIKeyIn(authPath, "groq", "from-auth")
	src = GetAuthSource("groq", cfg)
	if src != SourceAuthFile {
		t.Errorf("expected SourceAuthFile, got %q", src)
	}
}

func TestHasAnyAuth(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")

	origPath := storePath
	SetPath(authPath)
	defer SetPath(origPath)

	// Clear all known env vars.
	for _, p := range KnownProviders() {
		envVar, _ := EnvVarForProvider(p)
		t.Setenv(envVar, "")
	}

	if HasAnyAuth(nil) {
		t.Error("HasAnyAuth should be false with no auth configured")
	}

	// Add env var.
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	if !HasAnyAuth(nil) {
		t.Error("HasAnyAuth should be true with env var set")
	}
}

func TestSaveToJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")

	store := &AuthStore{
		Providers: map[string]ProviderAuth{
			"anthropic": {Type: "api_key", APIKey: "sk-test"},
		},
	}
	store.SaveTo(path)

	data, _ := os.ReadFile(path)
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}

	// Verify it's pretty-printed (has indentation).
	if len(data) < 10 {
		t.Error("saved JSON seems too short")
	}
}

func TestAllKnownProviders_NoConfig(t *testing.T) {
	got := AllKnownProviders(nil)
	want := KnownProviders()

	if len(got) != len(want) {
		t.Fatalf("AllKnownProviders(nil) returned %d providers, want %d", len(got), len(want))
	}
	for i, name := range got {
		if name != want[i] {
			t.Errorf("provider[%d] = %q, want %q", i, name, want[i])
		}
	}
}

func TestAllKnownProviders_WithCustom(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "mycustom"},
		},
	}

	got := AllKnownProviders(cfg)

	// Should include all hardcoded providers plus "mycustom".
	found := make(map[string]bool)
	for _, name := range got {
		found[name] = true
	}

	// Verify hardcoded providers are present.
	for _, name := range KnownProviders() {
		if !found[name] {
			t.Errorf("missing hardcoded provider %q in AllKnownProviders result", name)
		}
	}

	// Verify custom provider is present.
	if !found["mycustom"] {
		t.Error("expected custom provider 'mycustom' in AllKnownProviders result")
	}
}

func TestAllKnownProviders_NoDuplicates(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic"}, // already in hardcoded list
		},
	}

	got := AllKnownProviders(cfg)

	count := 0
	for _, name := range got {
		if name == "anthropic" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("anthropic appeared %d times, want exactly 1", count)
	}
}
