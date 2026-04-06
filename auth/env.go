package auth

import (
	"os"

	"github.com/egoisutolabs/forge/config"
)

// providerEnvVars maps provider names to their conventional API key
// environment variable names.
var providerEnvVars = map[string]string{
	"anthropic":  "ANTHROPIC_API_KEY",
	"openai":     "OPENAI_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"groq":       "GROQ_API_KEY",
	"google":     "GOOGLE_API_KEY",
	"mistral":    "MISTRAL_API_KEY",
	"xai":        "XAI_API_KEY",
	"deepinfra":  "DEEPINFRA_API_KEY",
}

// EnvVarForProvider returns the environment variable name for a provider,
// and whether it was found.
func EnvVarForProvider(provider string) (string, bool) {
	v, ok := providerEnvVars[provider]
	return v, ok
}

// GetEnvKey returns the API key from the environment for the given provider,
// or empty string if not set.
func GetEnvKey(provider string) string {
	envVar, ok := providerEnvVars[provider]
	if !ok {
		return ""
	}
	return os.Getenv(envVar)
}

// knownProviderSet is the hardcoded set of built-in provider names.
var knownProviderSet = map[string]bool{
	"anthropic":  true,
	"deepinfra":  true,
	"google":     true,
	"groq":       true,
	"mistral":    true,
	"openai":     true,
	"openrouter": true,
	"xai":        true,
}

// KnownProviders returns a sorted list of all built-in provider names.
func KnownProviders() []string {
	return []string{
		"anthropic",
		"deepinfra",
		"google",
		"groq",
		"mistral",
		"openai",
		"openrouter",
		"xai",
	}
}

// AllKnownProviders returns built-in providers plus any custom providers
// defined in the config. Custom providers (those not in the built-in set)
// are appended after the hardcoded list.
func AllKnownProviders(cfg *config.Config) []string {
	base := KnownProviders()

	if cfg == nil {
		return base
	}

	// Collect custom provider names not in the built-in set.
	seen := make(map[string]bool, len(base))
	for _, p := range base {
		seen[p] = true
	}

	var custom []string
	for _, p := range cfg.Providers {
		if !seen[p.Name] {
			custom = append(custom, p.Name)
			seen[p.Name] = true
		}
	}

	if len(custom) == 0 {
		return base
	}

	result := make([]string, len(base)+len(custom))
	copy(result, base)
	copy(result[len(base):], custom)
	return result
}
