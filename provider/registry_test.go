package provider

import (
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/config"
)

func TestRegistry_DefaultModel_ConfigDefault(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := &config.Config{
		DefaultModel: "claude-opus-4-6",
	}
	reg := NewRegistry(cfg)

	got := reg.DefaultModel()
	if got != "claude-opus-4-6" {
		t.Errorf("DefaultModel() = %q, want claude-opus-4-6", got)
	}
}

func TestRegistry_DefaultModel_FallbackToRecent(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	dir := t.TempDir()
	recentPath := filepath.Join(dir, "recent.json")
	RecordUsageTo(recentPath, "claude-sonnet-4-6")

	cfg := &config.Config{} // no default model
	reg := NewRegistryWithRecent(cfg, recentPath)

	got := reg.DefaultModel()
	if got != "claude-sonnet-4-6" {
		t.Errorf("DefaultModel() = %q, want claude-sonnet-4-6 (from recent)", got)
	}
}

func TestRegistry_DefaultModel_FallbackToFirstAvailable(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("OPENAI_API_KEY", "test-key")

	dir := t.TempDir()
	recentPath := filepath.Join(dir, "recent.json")

	cfg := &config.Config{}
	reg := NewRegistryWithRecent(cfg, recentPath)

	got := reg.DefaultModel()
	// Should be the first OpenAI bundled model.
	if got == "" {
		t.Error("DefaultModel() should return first available model, got empty")
	}
}

func TestRegistry_DefaultModel_NoProviders(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	dir := t.TempDir()
	recentPath := filepath.Join(dir, "recent.json")

	cfg := &config.Config{}
	reg := NewRegistryWithRecent(cfg, recentPath)

	got := reg.DefaultModel()
	if got != "" {
		t.Errorf("DefaultModel() with no providers = %q, want empty", got)
	}
}

func TestRegistry_HasModels(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{}
	reg := NewRegistry(cfg)
	if reg.HasModels() {
		t.Error("HasModels() should be false with no providers")
	}

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	reg = NewRegistry(cfg)
	if !reg.HasModels() {
		t.Error("HasModels() should be true with ANTHROPIC_API_KEY set")
	}
}

func TestRegistry_GetModel_Bundled(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	reg := NewRegistry(&config.Config{})
	m, ok := reg.GetModel("claude-sonnet-4-6")
	if !ok {
		t.Fatal("GetModel(claude-sonnet-4-6) returned false")
	}
	if m.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", m.Provider)
	}
	if m.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", m.ContextWindow)
	}
}

func TestRegistry_GetModel_Unknown(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	reg := NewRegistry(&config.Config{})
	_, ok := reg.GetModel("totally-fake-model")
	if ok {
		t.Error("GetModel(totally-fake-model) should return false")
	}
}

func TestRegistry_ListAvailable(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")

	reg := NewRegistry(&config.Config{})
	available := reg.ListAvailable()

	found := make(map[string]bool)
	for _, ap := range available {
		found[ap.Name] = true
	}
	if !found["anthropic"] {
		t.Error("expected anthropic in ListAvailable()")
	}
	if !found["openai"] {
		t.Error("expected openai in ListAvailable()")
	}
}

func TestRegistry_AllModels_NoDuplicates(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "anthropic", APIKey: "cfg-key", Models: []string{"claude-sonnet-4-6"}},
		},
	}
	reg := NewRegistry(cfg)

	models := reg.AllModels()
	seen := make(map[string]int)
	for _, m := range models {
		seen[m]++
		if seen[m] > 1 {
			t.Errorf("duplicate model %q in AllModels()", m)
		}
	}
}

func TestRegistry_DefaultModel_SkipsUnavailableRecent(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("OPENAI_API_KEY", "test-key")

	dir := t.TempDir()
	recentPath := filepath.Join(dir, "recent.json")
	// Record a model from a provider that isn't available.
	RecordUsageTo(recentPath, "claude-sonnet-4-6")
	// Also record one that IS available.
	RecordUsageTo(recentPath, "gpt-4o")

	cfg := &config.Config{}
	reg := NewRegistryWithRecent(cfg, recentPath)

	got := reg.DefaultModel()
	if got != "gpt-4o" {
		t.Errorf("DefaultModel() = %q, want gpt-4o (first available recent)", got)
	}
}

func TestRegistry_GetModel_CustomWithContextWindow(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "myprovider",
				APIKey: "test-key",
				Models: []string{"mymodel-v1"},
				ModelMeta: map[string]config.ModelConfig{
					"mymodel-v1": {
						Limit: config.ModelLimit{
							Context: 131072,
							Output:  8192,
						},
					},
				},
			},
		},
	}

	reg := NewRegistry(cfg)
	m, ok := reg.GetModel("mymodel-v1")
	if !ok {
		t.Fatal("GetModel(mymodel-v1) returned false")
	}
	if m.Provider != "myprovider" {
		t.Errorf("provider = %q, want myprovider", m.Provider)
	}
	if m.ContextWindow != 131072 {
		t.Errorf("ContextWindow = %d, want 131072", m.ContextWindow)
	}
}

func TestRegistry_GetModel_CustomWithOutputLimit(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "myprovider",
				APIKey: "test-key",
				Models: []string{"mymodel-v2"},
				ModelMeta: map[string]config.ModelConfig{
					"mymodel-v2": {
						Limit: config.ModelLimit{
							Context: 65536,
							Output:  16384,
						},
					},
				},
			},
		},
	}

	reg := NewRegistry(cfg)
	m, ok := reg.GetModel("mymodel-v2")
	if !ok {
		t.Fatal("GetModel(mymodel-v2) returned false")
	}
	if m.OutputLimit != 16384 {
		t.Errorf("OutputLimit = %d, want 16384", m.OutputLimit)
	}
}

func TestRegistry_ListAvailable_IncludesCustom(t *testing.T) {
	for _, key := range providerEnvKeys {
		t.Setenv(key, "")
	}

	cfg := &config.Config{
		Providers: []config.Provider{
			{
				Name:   "customai",
				APIKey: "test-key",
				Models: []string{"custom-model-x"},
			},
		},
	}

	reg := NewRegistry(cfg)
	available := reg.ListAvailable()

	found := false
	for _, ap := range available {
		if ap.Name == "customai" {
			found = true
			if len(ap.Models) != 1 || ap.Models[0] != "custom-model-x" {
				t.Errorf("customai models = %v, want [custom-model-x]", ap.Models)
			}
		}
	}
	if !found {
		t.Error("expected custom provider 'customai' in ListAvailable()")
	}
}
