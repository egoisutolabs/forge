package plugins

import (
	"fmt"
	"sync"

	"github.com/egoisutolabs/forge/hooks"
)

// PluginRegistry is a thread-safe store of plugins.
// It tracks both enabled and disabled plugins, and provides aggregated
// views of skill paths and hook configurations from enabled plugins.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin // keyed by plugin name
	order   []string           // insertion order for deterministic iteration
}

// NewPluginRegistry returns an empty registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]*Plugin),
	}
}

// Register adds a plugin to the registry.
// If a plugin with the same name already exists it is replaced.
func (r *PluginRegistry) Register(p *Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[p.Name]; !exists {
		r.order = append(r.order, p.Name)
	}
	r.plugins[p.Name] = p
}

// RegisterBuiltin adds a builtin plugin to the registry.
// Builtin plugins are marked with IsBuiltin=true and are always registered
// with Enabled=true unless the caller has explicitly set Enabled=false.
func (r *PluginRegistry) RegisterBuiltin(p *Plugin) {
	p.IsBuiltin = true
	r.Register(p)
}

// Enable activates the named plugin. Returns an error if the plugin is not found.
func (r *PluginRegistry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	p.Enabled = true
	return nil
}

// Disable deactivates the named plugin. Returns an error if the plugin is not found.
func (r *PluginRegistry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}
	p.Enabled = false
	return nil
}

// Get returns the named plugin and a boolean indicating whether it was found.
func (r *PluginRegistry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[name]
	return p, ok
}

// All returns all registered plugins in insertion order.
// The returned slice is a snapshot; modifying it does not affect the registry.
func (r *PluginRegistry) All() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Plugin, 0, len(r.order))
	for _, name := range r.order {
		if p, ok := r.plugins[name]; ok {
			out = append(out, p)
		}
	}
	return out
}

// GetAllSkillPaths returns the aggregated skill paths from all enabled plugins,
// in plugin registration order (then in the order each plugin declares them).
func (r *PluginRegistry) GetAllSkillPaths() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var paths []string
	for _, name := range r.order {
		p := r.plugins[name]
		if !p.Enabled {
			continue
		}
		paths = append(paths, p.SkillPaths...)
	}
	return paths
}

// GetAllHooks merges hook configurations from all enabled plugins into a
// single HooksSettings map. For each hook event, matchers are appended in
// plugin registration order. All hooks are tagged with Source="plugin" to
// enforce the trust boundary — plugin hooks require explicit user consent
// via TrustedSources to execute.
func (r *PluginRegistry) GetAllHooks() hooks.HooksSettings {
	r.mu.RLock()
	defer r.mu.RUnlock()

	merged := make(hooks.HooksSettings)
	for _, name := range r.order {
		p := r.plugins[name]
		if !p.Enabled || len(p.HookConfigs) == 0 {
			continue
		}
		for event, matchers := range p.HookConfigs {
			// Deep-copy matchers so we don't mutate the plugin's original config.
			tagged := make([]hooks.HookMatcher, len(matchers))
			for i, m := range matchers {
				taggedHooks := make([]hooks.HookConfig, len(m.Hooks))
				copy(taggedHooks, m.Hooks)
				for j := range taggedHooks {
					taggedHooks[j].Source = "plugin"
				}
				tagged[i] = hooks.HookMatcher{
					Matcher: m.Matcher,
					Hooks:   taggedHooks,
				}
			}
			merged[event] = append(merged[event], tagged...)
		}
	}
	return merged
}
