package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/config"
)

// TestConfig_LoadFromYAML writes a temp config YAML, loads it, and verifies providers/models.
func TestConfig_LoadFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	forgeDir := filepath.Join(tmpDir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yamlContent := `
providers:
  - name: openrouter
    base_url: https://openrouter.ai/api/v1
    api_key: sk-test-key
    models:
      - deepseek/deepseek-r1
      - anthropic/claude-sonnet-4-20250514
  - name: ollama
    base_url: http://localhost:11434/v1
    models:
      - llama3
default_model: deepseek/deepseek-r1
fallback_model: llama3
model_costs:
  deepseek/deepseek-r1:
    input: 0.55
    output: 2.19
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 2 {
		t.Fatalf("Providers count = %d, want 2", len(cfg.Providers))
	}

	or := cfg.Providers[0]
	if or.Name != "openrouter" {
		t.Errorf("Providers[0].Name = %q, want openrouter", or.Name)
	}
	if or.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("Providers[0].BaseURL = %q", or.BaseURL)
	}
	if or.APIKey != "sk-test-key" {
		t.Errorf("Providers[0].APIKey = %q", or.APIKey)
	}
	if len(or.Models) != 2 {
		t.Fatalf("Providers[0].Models count = %d, want 2", len(or.Models))
	}
	if or.Models[0] != "deepseek/deepseek-r1" {
		t.Errorf("Providers[0].Models[0] = %q", or.Models[0])
	}

	ollama := cfg.Providers[1]
	if ollama.Name != "ollama" {
		t.Errorf("Providers[1].Name = %q, want ollama", ollama.Name)
	}
	if len(ollama.Models) != 1 || ollama.Models[0] != "llama3" {
		t.Errorf("Providers[1].Models = %v", ollama.Models)
	}

	if cfg.DefaultModel != "deepseek/deepseek-r1" {
		t.Errorf("DefaultModel = %q", cfg.DefaultModel)
	}
	if cfg.FallbackModel != "llama3" {
		t.Errorf("FallbackModel = %q", cfg.FallbackModel)
	}

	cost, ok := cfg.ModelCosts["deepseek/deepseek-r1"]
	if !ok {
		t.Fatal("ModelCosts missing deepseek/deepseek-r1")
	}
	if cost.Input != 0.55 || cost.Output != 2.19 {
		t.Errorf("ModelCosts = {Input:%v, Output:%v}", cost.Input, cost.Output)
	}
}

// TestConfig_EnvVarExpansion verifies ${ENV_VAR} references are expanded.
func TestConfig_EnvVarExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	forgeDir := filepath.Join(tmpDir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("TEST_FORGE_KEY", "my-secret-key-123")
	t.Setenv("TEST_FORGE_URL", "https://api.example.com/v1")

	yamlContent := `
providers:
  - name: test-provider
    base_url: ${TEST_FORGE_URL}
    api_key: ${TEST_FORGE_KEY}
    models:
      - test-model
default_model: test-model
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) == 0 {
		t.Fatal("no providers loaded")
	}

	p := cfg.Providers[0]
	if p.APIKey != "my-secret-key-123" {
		t.Errorf("APIKey not expanded: got %q", p.APIKey)
	}
	if p.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL not expanded: got %q", p.BaseURL)
	}
}

// TestConfig_ResolveModel verifies model resolution across providers.
func TestConfig_ResolveModel(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "openrouter", Models: []string{"deepseek-r1", "claude-sonnet"}},
			{Name: "ollama", Models: []string{"llama3", "codestral"}},
		},
	}

	// Exact match.
	p, err := cfg.ResolveModel("llama3")
	if err != nil {
		t.Fatalf("ResolveModel(llama3): %v", err)
	}
	if p.Name != "ollama" {
		t.Errorf("ResolveModel(llama3) provider = %q, want ollama", p.Name)
	}

	// Suffix match for namespaced model.
	p, err = cfg.ResolveModel("deepseek/deepseek-r1")
	if err != nil {
		t.Fatalf("ResolveModel(deepseek/deepseek-r1): %v", err)
	}
	if p.Name != "openrouter" {
		t.Errorf("ResolveModel(deepseek/deepseek-r1) provider = %q, want openrouter", p.Name)
	}

	// First provider wins (priority order).
	p, err = cfg.ResolveModel("claude-sonnet")
	if err != nil {
		t.Fatalf("ResolveModel(claude-sonnet): %v", err)
	}
	if p.Name != "openrouter" {
		t.Errorf("ResolveModel(claude-sonnet) provider = %q, want openrouter", p.Name)
	}

	// Unknown model.
	_, err = cfg.ResolveModel("nonexistent-model")
	if err == nil {
		t.Error("ResolveModel(nonexistent-model) should error")
	}

	// Empty model.
	_, err = cfg.ResolveModel("")
	if err == nil {
		t.Error("ResolveModel('') should error")
	}
}

// TestConfig_MissingConfig verifies graceful fallback when no config file exists.
func TestConfig_MissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	// No .forge/config.yaml written — Load should succeed with zero config.
	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load on missing config: %v", err)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(cfg.Providers))
	}
	if cfg.DefaultModel != "" {
		t.Errorf("expected empty DefaultModel, got %q", cfg.DefaultModel)
	}
}

// TestConfig_EnvOverride_FORGE_MODEL verifies FORGE_MODEL env override.
func TestConfig_EnvOverride_FORGE_MODEL(t *testing.T) {
	tmpDir := t.TempDir()
	forgeDir := filepath.Join(tmpDir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	yamlContent := `
providers:
  - name: test
    models:
      - model-a
default_model: model-a
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("FORGE_MODEL", "model-override")

	cfg, err := config.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultModel != "model-override" {
		t.Errorf("DefaultModel = %q, want model-override", cfg.DefaultModel)
	}
}
