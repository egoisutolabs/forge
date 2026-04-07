package config

import (
	"fmt"
	"strings"
)

// ResolveModel finds the provider that serves the given model name.
//
// Resolution order:
//  1. Exact match in a provider's Models list
//  2. If model contains "/", try matching the part after "/" (suffix match)
//  3. Error: unknown model
//
// If a model appears in multiple providers, the first match wins (provider
// order in config = priority order).
func (c *Config) ResolveModel(model string) (*Provider, error) {
	if model == "" {
		return nil, fmt.Errorf("empty model name")
	}

	// Exact match.
	for i := range c.Providers {
		for _, m := range c.Providers[i].Models {
			if m == model {
				return &c.Providers[i], nil
			}
		}
	}

	// Suffix match for namespaced models (e.g. "deepseek/deepseek-r1" → "deepseek-r1").
	if strings.Contains(model, "/") {
		suffix := model[strings.LastIndex(model, "/")+1:]
		for i := range c.Providers {
			for _, m := range c.Providers[i].Models {
				if m == suffix {
					return &c.Providers[i], nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no provider found for model %q", model)
}
