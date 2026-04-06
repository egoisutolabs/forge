// Package config handles loading and merging forge configuration from YAML and
// JSON files.
package config

// Config is the top-level forge configuration.
type Config struct {
	Providers     []Provider      `yaml:"providers"`
	DefaultModel  string          `yaml:"default_model"`
	FallbackModel string          `yaml:"fallback_model,omitempty"`
	ModelCosts    map[string]Cost `yaml:"model_costs,omitempty"`
}

// Provider describes one API provider (e.g. anthropic, openrouter, ollama).
type Provider struct {
	Name        string                 `yaml:"name"`
	BaseURL     string                 `yaml:"base_url,omitempty"`
	APIKey      string                 `yaml:"api_key,omitempty"`
	Models      []string               `yaml:"models"`
	Headers     map[string]string      `yaml:"headers,omitempty"`
	DisplayName string                 `yaml:"-"` // human-friendly name from JSON config
	NoAuth      bool                   `yaml:"-"` // skip API key requirement (local providers)
	ModelMeta   map[string]ModelConfig `yaml:"-"` // per-model metadata from JSON config
}

// Cost holds per-model pricing in USD per million tokens.
type Cost struct {
	Input  float64 `yaml:"input"`
	Output float64 `yaml:"output"`
}

// ModelConfig holds per-model metadata from JSON provider config.
type ModelConfig struct {
	DisplayName string     `json:"name"`
	Limit       ModelLimit `json:"limit"`
}

// ModelLimit holds token limit configuration for a model.
type ModelLimit struct {
	Context int `json:"context"` // max context tokens
	Output  int `json:"output"`  // max output tokens
}

// JSONConfig is the top-level structure for config.json files.
type JSONConfig struct {
	DefaultModel string                  `json:"default_model"`
	Providers    map[string]JSONProvider `json:"provider"`
}

// JSONProvider represents a provider entry in config.json.
type JSONProvider struct {
	Name    string                 `json:"name"`
	NoAuth  bool                   `json:"no_auth"`
	Options JSONProviderOptions    `json:"options"`
	Models  map[string]ModelConfig `json:"models"`
}

// JSONProviderOptions holds the connection options for a JSON provider.
type JSONProviderOptions struct {
	BaseURL string            `json:"baseURL"`
	APIKey  string            `json:"apiKey"`
	Headers map[string]string `json:"headers"`
}
