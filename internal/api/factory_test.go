package api

import (
	"testing"

	"github.com/egoisutolabs/forge/internal/config"
)

func TestNewCaller_Anthropic(t *testing.T) {
	tests := []struct {
		name     string
		provider config.Provider
	}{
		{"by name", config.Provider{Name: "anthropic", APIKey: "k"}},
		{"by empty base URL", config.Provider{Name: "custom", APIKey: "k"}},
		{"by anthropic.com URL", config.Provider{Name: "x", BaseURL: "https://api.anthropic.com", APIKey: "k"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCaller(&tt.provider)
			if _, ok := c.(*AnthropicCaller); !ok {
				t.Errorf("expected *AnthropicCaller, got %T", c)
			}
		})
	}
}

func TestNewCaller_OpenAI(t *testing.T) {
	tests := []struct {
		name     string
		provider config.Provider
	}{
		{"openrouter", config.Provider{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", APIKey: "k", Models: []string{"gpt-4"}}},
		{"ollama", config.Provider{Name: "ollama", BaseURL: "http://localhost:11434/v1", Models: []string{"llama3"}}},
		{"local", config.Provider{Name: "local", BaseURL: "http://localhost:8080/v1", Models: []string{"m"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCaller(&tt.provider)
			if _, ok := c.(*OpenAICaller); !ok {
				t.Errorf("expected *OpenAICaller, got %T", c)
			}
		})
	}
}
