package provider

import (
	"github.com/egoisutolabs/forge/internal/config"
)

// Registry combines bundled models, config providers, and detected providers
// into a unified model lookup.
type Registry struct {
	cfg        *config.Config
	available  []AvailableProvider
	recentPath string // empty → use default path
}

// NewRegistry creates a Registry by detecting available providers.
func NewRegistry(cfg *config.Config) *Registry {
	return &Registry{
		cfg:       cfg,
		available: DetectProviders(cfg),
	}
}

// NewRegistryWithRecent creates a Registry with an explicit recent-models path (for testing).
func NewRegistryWithRecent(cfg *config.Config, recentPath string) *Registry {
	return &Registry{
		cfg:        cfg,
		available:  DetectProviders(cfg),
		recentPath: recentPath,
	}
}

// ListAvailable returns all detected providers and their models.
func (r *Registry) ListAvailable() []AvailableProvider {
	out := make([]AvailableProvider, len(r.available))
	copy(out, r.available)
	return out
}

// AllModels returns a flat list of all available model names.
func (r *Registry) AllModels() []string {
	var models []string
	seen := make(map[string]bool)
	for _, ap := range r.available {
		for _, m := range ap.Models {
			if !seen[m] {
				seen[m] = true
				models = append(models, m)
			}
		}
	}
	return models
}

// GetModel returns the ModelInfo for the named model if it is in the bundled
// catalog. Models from dynamic providers (Ollama, config) that are not in the
// catalog return a ModelInfo enriched with metadata from config when available.
func (r *Registry) GetModel(name string) (ModelInfo, bool) {
	// Check bundled catalog first.
	if m, ok := LookupBundled(name); ok {
		return m, true
	}
	// Check if the model is available through a detected provider.
	for _, ap := range r.available {
		for _, m := range ap.Models {
			if m == name {
				mi := ModelInfo{Name: name, Provider: ap.Name, SupportsStreaming: true}
				// Enrich from config ModelMeta.
				if r.cfg != nil {
					for _, p := range r.cfg.Providers {
						if p.Name == ap.Name {
							if mc, ok := p.ModelMeta[name]; ok {
								mi.ContextWindow = mc.Limit.Context
								mi.OutputLimit = mc.Limit.Output
							}
						}
					}
				}
				return mi, true
			}
		}
	}
	return ModelInfo{}, false
}

// RecentModels returns the recently used model names.
func (r *Registry) RecentModels() []string {
	if r.recentPath != "" {
		return GetRecentFrom(r.recentPath)
	}
	return GetRecent()
}

// DefaultModel determines the best model to use via the fallback chain:
//  1. Config default_model (if set and a provider is available for it)
//  2. Most recently used model (if still available)
//  3. First available model from detected providers
//  4. "" (no models available)
func (r *Registry) DefaultModel() string {
	allModels := r.allModelSet()

	// 1. Config default.
	if r.cfg != nil && r.cfg.DefaultModel != "" {
		if allModels[r.cfg.DefaultModel] {
			return r.cfg.DefaultModel
		}
	}

	// 2. Most recent that is still available.
	for _, m := range r.RecentModels() {
		if allModels[m] {
			return m
		}
	}

	// 3. First available model.
	if len(r.available) > 0 && len(r.available[0].Models) > 0 {
		return r.available[0].Models[0]
	}

	return ""
}

// HasModels returns true if at least one model is available.
func (r *Registry) HasModels() bool {
	for _, ap := range r.available {
		if len(ap.Models) > 0 {
			return true
		}
	}
	return false
}

func (r *Registry) allModelSet() map[string]bool {
	set := make(map[string]bool)
	for _, ap := range r.available {
		for _, m := range ap.Models {
			set[m] = true
		}
	}
	return set
}
