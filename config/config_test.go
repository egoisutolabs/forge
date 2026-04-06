package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "secret123")
	t.Setenv("TEST_URL", "https://example.com")

	tests := []struct {
		input string
		want  string
	}{
		{"${TEST_KEY}", "secret123"},
		{"prefix-${TEST_KEY}-suffix", "prefix-secret123-suffix"},
		{"${TEST_URL}/v1", "https://example.com/v1"},
		{"no-vars-here", "no-vars-here"},
		{"${UNSET_VAR_12345}", ""},
		{"${TEST_KEY}/${TEST_URL}", "secret123/https://example.com"},
	}
	for _, tt := range tests {
		got := expandEnv(tt.input)
		if got != tt.want {
			t.Errorf("expandEnv(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Non-existent file → zero value, no error.
	var cfg Config
	if err := loadFile(filepath.Join(dir, "nope.yaml"), &cfg); err != nil {
		t.Fatalf("loadFile non-existent: %v", err)
	}
	if len(cfg.Providers) != 0 {
		t.Fatal("expected zero config for missing file")
	}

	// Write a config and load it.
	yaml := `
providers:
  - name: anthropic
    api_key: test-key
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := loadFile(path, &cfg); err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].APIKey != "test-key" {
		t.Errorf("api_key = %q, want %q", cfg.Providers[0].APIKey, "test-key")
	}
	if cfg.DefaultModel != "claude-sonnet-4-6" {
		t.Errorf("default_model = %q, want %q", cfg.DefaultModel, "claude-sonnet-4-6")
	}
}

func TestMerge(t *testing.T) {
	global := Config{
		Providers: []Provider{
			{Name: "anthropic", APIKey: "global-key", Models: []string{"claude-sonnet-4-6"}},
			{Name: "ollama", BaseURL: "http://localhost:11434/v1", Models: []string{"llama3:70b"}},
		},
		DefaultModel: "claude-sonnet-4-6",
	}

	project := Config{
		Providers: []Provider{
			// Replaces "anthropic" entirely.
			{Name: "anthropic", APIKey: "project-key", Models: []string{"claude-opus-4-6"}},
			// New provider only in project.
			{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", Models: []string{"deepseek/deepseek-r1"}},
		},
		DefaultModel: "claude-opus-4-6",
	}

	merged := merge(global, project)

	if merged.DefaultModel != "claude-opus-4-6" {
		t.Errorf("default_model = %q, want %q", merged.DefaultModel, "claude-opus-4-6")
	}

	// Should have 3 providers: anthropic (overridden), ollama (from global), openrouter (new).
	if len(merged.Providers) != 3 {
		t.Fatalf("got %d providers, want 3", len(merged.Providers))
	}

	byName := make(map[string]Provider)
	for _, p := range merged.Providers {
		byName[p.Name] = p
	}

	if p, ok := byName["anthropic"]; !ok {
		t.Fatal("missing anthropic provider")
	} else if p.APIKey != "project-key" {
		t.Errorf("anthropic api_key = %q, want %q", p.APIKey, "project-key")
	}

	if _, ok := byName["ollama"]; !ok {
		t.Fatal("missing ollama provider (should be inherited from global)")
	}

	if _, ok := byName["openrouter"]; !ok {
		t.Fatal("missing openrouter provider (should be added from project)")
	}
}

func TestMergeModelCosts(t *testing.T) {
	global := Config{
		ModelCosts: map[string]Cost{
			"model-a": {Input: 1.0, Output: 2.0},
			"model-b": {Input: 3.0, Output: 4.0},
		},
	}
	project := Config{
		ModelCosts: map[string]Cost{
			"model-b": {Input: 5.0, Output: 6.0}, // override
			"model-c": {Input: 7.0, Output: 8.0}, // new
		},
	}
	merged := merge(global, project)

	if c, ok := merged.ModelCosts["model-a"]; !ok || c.Input != 1.0 {
		t.Error("model-a should be inherited from global")
	}
	if c, ok := merged.ModelCosts["model-b"]; !ok || c.Input != 5.0 {
		t.Error("model-b should be overridden by project")
	}
	if c, ok := merged.ModelCosts["model-c"]; !ok || c.Input != 7.0 {
		t.Error("model-c should be added from project")
	}
}

func TestLoadWithEnvExpansion(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MY_API_KEY", "expanded-key-value")

	yaml := `
providers:
  - name: anthropic
    api_key: ${MY_API_KEY}
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Load with projectDir = dir (reads dir/.forge/config.yaml).
	// Set HOME to a temp dir with no global config to isolate the test.
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].APIKey != "expanded-key-value" {
		t.Errorf("api_key = %q, want %q", cfg.Providers[0].APIKey, "expanded-key-value")
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	yaml := `
providers:
  - name: anthropic
    api_key: file-key
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("FORGE_MODEL", "override-model")
	t.Setenv("ANTHROPIC_API_KEY", "env-key")

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DefaultModel != "override-model" {
		t.Errorf("default_model = %q, want %q", cfg.DefaultModel, "override-model")
	}
	if cfg.Providers[0].APIKey != "env-key" {
		t.Errorf("anthropic api_key = %q, want %q", cfg.Providers[0].APIKey, "env-key")
	}
}

func TestResolveModel(t *testing.T) {
	cfg := &Config{
		Providers: []Provider{
			{Name: "anthropic", APIKey: "key1", Models: []string{"claude-sonnet-4-6", "claude-opus-4-6"}},
			{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", APIKey: "key2", Models: []string{"deepseek/deepseek-r1"}},
			{Name: "ollama", BaseURL: "http://localhost:11434/v1", Models: []string{"llama3:70b"}},
		},
	}

	tests := []struct {
		model    string
		wantName string
		wantErr  bool
	}{
		{"claude-sonnet-4-6", "anthropic", false},
		{"claude-opus-4-6", "anthropic", false},
		{"deepseek/deepseek-r1", "openrouter", false},
		{"llama3:70b", "ollama", false},
		{"unknown-model", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		p, err := cfg.ResolveModel(tt.model)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ResolveModel(%q): want error, got provider %q", tt.model, p.Name)
			}
			continue
		}
		if err != nil {
			t.Errorf("ResolveModel(%q): %v", tt.model, err)
			continue
		}
		if p.Name != tt.wantName {
			t.Errorf("ResolveModel(%q) = provider %q, want %q", tt.model, p.Name, tt.wantName)
		}
	}
}

func TestResolveModelSuffixMatch(t *testing.T) {
	cfg := &Config{
		Providers: []Provider{
			{Name: "openrouter", Models: []string{"deepseek-r1"}},
		},
	}

	// "vendor/deepseek-r1" should match "deepseek-r1" via suffix.
	p, err := cfg.ResolveModel("vendor/deepseek-r1")
	if err != nil {
		t.Fatalf("ResolveModel(vendor/deepseek-r1): %v", err)
	}
	if p.Name != "openrouter" {
		t.Errorf("got provider %q, want %q", p.Name, "openrouter")
	}
}

func TestResolveModelFirstMatchWins(t *testing.T) {
	cfg := &Config{
		Providers: []Provider{
			{Name: "first", Models: []string{"shared-model"}},
			{Name: "second", Models: []string{"shared-model"}},
		},
	}

	p, err := cfg.ResolveModel("shared-model")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "first" {
		t.Errorf("got provider %q, want %q (first match wins)", p.Name, "first")
	}
}

func TestLoadNoConfigFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load with no files: %v", err)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected zero providers, got %d", len(cfg.Providers))
	}
}

func TestLoadJSONFile(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	jsonConfig := `{
  "default_model": "my-model-a",
  "provider": {
    "myprovider": {
      "name": "My Provider",
      "options": {
        "baseURL": "https://api.myprovider.com/v1",
        "apiKey": "sk-test-123",
        "headers": {"X-Custom": "value"}
      },
      "models": {
        "my-model-a": {"name": "Model A"},
        "my-model-b": {"name": "Model B"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.DefaultModel != "my-model-a" {
		t.Errorf("default_model = %q, want %q", cfg.DefaultModel, "my-model-a")
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}

	p := cfg.Providers[0]
	if p.Name != "myprovider" {
		t.Errorf("Name = %q, want %q", p.Name, "myprovider")
	}
	if p.DisplayName != "My Provider" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "My Provider")
	}
	if p.BaseURL != "https://api.myprovider.com/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.myprovider.com/v1")
	}
	if p.APIKey != "sk-test-123" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-test-123")
	}
	if len(p.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(p.Models))
	}
	// Models are sorted alphabetically.
	if p.Models[0] != "my-model-a" || p.Models[1] != "my-model-b" {
		t.Errorf("Models = %v, want [my-model-a my-model-b]", p.Models)
	}
	if p.Headers["X-Custom"] != "value" {
		t.Errorf("Headers[X-Custom] = %q, want %q", p.Headers["X-Custom"], "value")
	}
	if len(p.ModelMeta) != 2 {
		t.Errorf("ModelMeta has %d entries, want 2", len(p.ModelMeta))
	}
}

func TestLoadJSONModelMeta(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	jsonConfig := `{
  "default_model": "big-model",
  "provider": {
    "testprov": {
      "name": "Test Provider",
      "options": {
        "baseURL": "https://api.test.com/v1"
      },
      "models": {
        "big-model": {
          "name": "Big Model",
          "limit": {"context": 200000, "output": 8192}
        },
        "small-model": {
          "name": "Small Model",
          "limit": {"context": 4096, "output": 1024}
        }
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}

	p := cfg.Providers[0]
	meta := p.ModelMeta

	if len(meta) != 2 {
		t.Fatalf("ModelMeta has %d entries, want 2", len(meta))
	}

	big, ok := meta["big-model"]
	if !ok {
		t.Fatal("missing big-model in ModelMeta")
	}
	if big.DisplayName != "Big Model" {
		t.Errorf("big-model name = %q, want %q", big.DisplayName, "Big Model")
	}
	if big.Limit.Context != 200000 {
		t.Errorf("big-model context = %d, want 200000", big.Limit.Context)
	}
	if big.Limit.Output != 8192 {
		t.Errorf("big-model output = %d, want 8192", big.Limit.Output)
	}

	small, ok := meta["small-model"]
	if !ok {
		t.Fatal("missing small-model in ModelMeta")
	}
	if small.Limit.Context != 4096 {
		t.Errorf("small-model context = %d, want 4096", small.Limit.Context)
	}
	if small.Limit.Output != 1024 {
		t.Errorf("small-model output = %d, want 1024", small.Limit.Output)
	}
}

func TestLoadJSONEnvColon(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MY_TEST_KEY", "expanded-test-key-value")

	jsonConfig := `{
  "default_model": "m1",
  "provider": {
    "prov": {
      "name": "Prov",
      "options": {
        "apiKey": "{env:MY_TEST_KEY}"
      },
      "models": {
        "m1": {"name": "M1"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}
	if cfg.Providers[0].APIKey != "expanded-test-key-value" {
		t.Errorf("apiKey = %q, want %q", cfg.Providers[0].APIKey, "expanded-test-key-value")
	}
}

func TestLoadYAMLAndJSONMerge(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
providers:
  - name: anthropic
    api_key: yaml-key
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	jsonConfig := `{
  "default_model": "my-model",
  "provider": {
    "myprovider": {
      "name": "My Provider",
      "options": {
        "baseURL": "https://api.myprovider.com/v1",
        "apiKey": "json-key"
      },
      "models": {
        "my-model": {"name": "My Model"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 2 {
		t.Fatalf("got %d providers, want 2", len(cfg.Providers))
	}

	byName := make(map[string]Provider)
	for _, p := range cfg.Providers {
		byName[p.Name] = p
	}

	if _, ok := byName["anthropic"]; !ok {
		t.Error("missing anthropic provider from YAML")
	}
	if _, ok := byName["myprovider"]; !ok {
		t.Error("missing myprovider provider from JSON")
	}
}

func TestLoadJSONProviderOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
providers:
  - name: anthropic
    api_key: yaml-key
    base_url: https://old-url.example.com
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	jsonConfig := `{
  "default_model": "claude-sonnet-4-6",
  "provider": {
    "anthropic": {
      "name": "Anthropic",
      "options": {
        "baseURL": "https://new-url.anthropic.com/v2",
        "apiKey": "json-key"
      },
      "models": {
        "claude-sonnet-4-6": {"name": "Claude Sonnet 4"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}

	p := cfg.Providers[0]
	if p.Name != "anthropic" {
		t.Errorf("Name = %q, want %q", p.Name, "anthropic")
	}
	if p.BaseURL != "https://new-url.anthropic.com/v2" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://new-url.anthropic.com/v2")
	}
	if p.APIKey != "json-key" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "json-key")
	}
	if p.DisplayName != "Anthropic" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Anthropic")
	}
}

func TestLoadJSONMissingFile(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write only a YAML file, no JSON file.
	yamlContent := `
providers:
  - name: anthropic
    api_key: key
    models:
      - claude-sonnet-4-6
default_model: claude-sonnet-4-6
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("got %d providers, want 1", len(cfg.Providers))
	}
}

func TestExpandEnvColon(t *testing.T) {
	t.Setenv("PRESENT_VAR", "hello")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "env colon variable present",
			input: "{env:PRESENT_VAR}",
			want:  "hello",
		},
		{
			name:  "env colon variable absent",
			input: "{env:MISSING_VAR_XYZ}",
			want:  "",
		},
		{
			name:  "mixed dollar and env colon",
			input: "${PRESENT_VAR}-{env:PRESENT_VAR}",
			want:  "hello-hello",
		},
		{
			name:  "env colon embedded in string",
			input: "key={env:PRESENT_VAR}&other=value",
			want:  "key=hello&other=value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandEnv(tt.input)
			if got != tt.want {
				t.Errorf("expandEnv(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestJSONProviderToProvider(t *testing.T) {
	jp := JSONProvider{
		Name:   "Display Name",
		NoAuth: false,
		Options: JSONProviderOptions{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "sk-abc",
			Headers: map[string]string{
				"X-Auth": "bearer-token",
			},
		},
		Models: map[string]ModelConfig{
			"zeta-model":  {DisplayName: "Zeta", Limit: ModelLimit{Context: 8000, Output: 2048}},
			"alpha-model": {DisplayName: "Alpha", Limit: ModelLimit{Context: 4000, Output: 1024}},
		},
	}

	p := jsonProviderToProvider("myid", jp)

	if p.Name != "myid" {
		t.Errorf("Name = %q, want %q", p.Name, "myid")
	}
	if p.DisplayName != "Display Name" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Display Name")
	}
	if p.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.example.com/v1")
	}
	if p.APIKey != "sk-abc" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-abc")
	}
	if p.NoAuth {
		t.Error("NoAuth should be false")
	}

	// Models should be sorted alphabetically.
	if len(p.Models) != 2 {
		t.Fatalf("Models has %d entries, want 2", len(p.Models))
	}
	if p.Models[0] != "alpha-model" || p.Models[1] != "zeta-model" {
		t.Errorf("Models = %v, want [alpha-model zeta-model]", p.Models)
	}

	// ModelMeta populated.
	if len(p.ModelMeta) != 2 {
		t.Fatalf("ModelMeta has %d entries, want 2", len(p.ModelMeta))
	}
	if p.ModelMeta["zeta-model"].Limit.Context != 8000 {
		t.Errorf("zeta-model context = %d, want 8000", p.ModelMeta["zeta-model"].Limit.Context)
	}
	if p.ModelMeta["alpha-model"].Limit.Output != 1024 {
		t.Errorf("alpha-model output = %d, want 1024", p.ModelMeta["alpha-model"].Limit.Output)
	}

	// Headers copied.
	if p.Headers["X-Auth"] != "bearer-token" {
		t.Errorf("Headers[X-Auth] = %q, want %q", p.Headers["X-Auth"], "bearer-token")
	}
}

func TestSaveCustomProvider(t *testing.T) {
	homeDir := t.TempDir()

	if err := SaveCustomProvider(homeDir, "myprov", "My Provider", "https://api.myprov.com/v1", "sk-123"); err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".forge", "config.json"))
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}

	var jc JSONConfig
	if err := json.Unmarshal(data, &jc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	p, ok := jc.Providers["myprov"]
	if !ok {
		t.Fatal("missing myprov provider in saved config")
	}
	if p.Name != "My Provider" {
		t.Errorf("name = %q, want %q", p.Name, "My Provider")
	}
	if p.Options.BaseURL != "https://api.myprov.com/v1" {
		t.Errorf("baseURL = %q, want %q", p.Options.BaseURL, "https://api.myprov.com/v1")
	}
	if p.Options.APIKey != "sk-123" {
		t.Errorf("apiKey = %q, want %q", p.Options.APIKey, "sk-123")
	}
	if p.NoAuth {
		t.Error("NoAuth should be false when apiKey is provided")
	}
}

func TestSaveCustomProviderPreservesExisting(t *testing.T) {
	homeDir := t.TempDir()
	forgeDir := filepath.Join(homeDir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	existingJSON := `{
  "provider": {
    "first": {
      "name": "First Provider",
      "options": {
        "baseURL": "https://first.example.com",
        "apiKey": "key-one"
      },
      "models": {
        "model-a": {"name": "Model A"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(existingJSON), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SaveCustomProvider(homeDir, "second", "Second Provider", "https://second.example.com", "key-two"); err != nil {
		t.Fatalf("SaveCustomProvider: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(forgeDir, "config.json"))
	if err != nil {
		t.Fatalf("reading config.json: %v", err)
	}

	var jc JSONConfig
	if err := json.Unmarshal(data, &jc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(jc.Providers) != 2 {
		t.Fatalf("got %d providers, want 2", len(jc.Providers))
	}

	first, ok := jc.Providers["first"]
	if !ok {
		t.Fatal("missing 'first' provider")
	}
	if first.Name != "First Provider" {
		t.Errorf("first name = %q, want %q", first.Name, "First Provider")
	}
	if len(first.Models) != 1 {
		t.Errorf("first models count = %d, want 1", len(first.Models))
	}

	second, ok := jc.Providers["second"]
	if !ok {
		t.Fatal("missing 'second' provider")
	}
	if second.Name != "Second Provider" {
		t.Errorf("second name = %q, want %q", second.Name, "Second Provider")
	}
	if second.Options.BaseURL != "https://second.example.com" {
		t.Errorf("second baseURL = %q, want %q", second.Options.BaseURL, "https://second.example.com")
	}
}

func TestNoAuthFromJSON(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0755); err != nil {
		t.Fatal(err)
	}

	jsonConfig := `{
  "default_model": "local-model",
  "provider": {
    "localai": {
      "name": "LocalAI",
      "no_auth": true,
      "options": {
        "baseURL": "http://localhost:8080/v1"
      },
      "models": {
        "local-model": {"name": "Local Model"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.json"), []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Providers) != 1 {
		t.Fatalf("got %d providers, want 1", len(cfg.Providers))
	}
	if !cfg.Providers[0].NoAuth {
		t.Error("NoAuth should be true")
	}
}
