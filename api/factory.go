package api

import (
	"strings"

	"github.com/egoisutolabs/forge/config"
)

// NewCaller creates a Caller for the given provider.
// If the provider is Anthropic (by name or base URL), returns an AnthropicCaller.
// Otherwise returns an OpenAICaller for OpenAI-compatible endpoints.
func NewCaller(provider *config.Provider) Caller {
	if isAnthropicProvider(provider) {
		return NewAnthropicCallerFromProvider(provider)
	}
	return NewOpenAICaller(provider)
}

// isAnthropicProvider returns true if the provider should use the Anthropic API format.
func isAnthropicProvider(p *config.Provider) bool {
	return p.Name == "anthropic" ||
		p.BaseURL == "" ||
		strings.Contains(p.BaseURL, "anthropic.com")
}
