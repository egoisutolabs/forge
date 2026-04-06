package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads global (~/.forge/config.yaml, ~/.forge/config.json) then project
// (<projectDir>/.forge/config.yaml, <projectDir>/.forge/config.json), merges
// them, expands ${ENV_VAR} and {env:ENV_VAR} references, and applies
// environment variable overrides.
// Returns a zero Config (not an error) if no files exist.
func Load(projectDir string) (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	var global Config
	if home != "" {
		globalPath := filepath.Join(home, ".forge", "config.yaml")
		if err := loadFile(globalPath, &global); err != nil {
			return nil, err
		}
		// Overlay JSON on top of YAML.
		globalJSONPath := filepath.Join(home, ".forge", "config.json")
		if err := loadJSONFile(globalJSONPath, &global); err != nil {
			return nil, err
		}
	}

	var project Config
	if projectDir != "" {
		projectPath := filepath.Join(projectDir, ".forge", "config.yaml")
		if err := loadFile(projectPath, &project); err != nil {
			return nil, err
		}
		// Overlay JSON on top of YAML.
		projectJSONPath := filepath.Join(projectDir, ".forge", "config.json")
		if err := loadJSONFile(projectJSONPath, &project); err != nil {
			return nil, err
		}
	}

	merged := merge(global, project)
	expandEnvInConfig(&merged)
	applyEnvOverrides(&merged)
	return &merged, nil
}

// loadFile reads a YAML config file into cfg. If the file does not exist, cfg
// is left at its zero value and no error is returned.
func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// loadJSONFile reads a JSON config file and merges provider entries into cfg.
// If the file does not exist, cfg is left unchanged and no error is returned.
func loadJSONFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var jc JSONConfig
	if err := json.Unmarshal(data, &jc); err != nil {
		return err
	}

	if jc.DefaultModel != "" {
		cfg.DefaultModel = jc.DefaultModel
	}

	for id, jp := range jc.Providers {
		p := jsonProviderToProvider(id, jp)

		// Replace existing provider with same name, or append.
		replaced := false
		for i, existing := range cfg.Providers {
			if existing.Name == p.Name {
				cfg.Providers[i] = p
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Providers = append(cfg.Providers, p)
		}
	}

	return nil
}

// jsonProviderToProvider converts a JSONProvider (map-keyed format) into a
// Provider (the internal representation used throughout the codebase).
func jsonProviderToProvider(id string, jp JSONProvider) Provider {
	name := id
	if jp.Name != "" {
		// jp.Name is the display name; id is the programmatic key.
	}

	// Collect model IDs in sorted order for deterministic behavior.
	models := make([]string, 0, len(jp.Models))
	for m := range jp.Models {
		models = append(models, m)
	}
	sort.Strings(models)

	// Copy model metadata.
	meta := make(map[string]ModelConfig, len(jp.Models))
	for k, v := range jp.Models {
		meta[k] = v
	}

	// Copy headers.
	headers := make(map[string]string, len(jp.Options.Headers))
	for k, v := range jp.Options.Headers {
		headers[k] = v
	}

	return Provider{
		Name:        name,
		DisplayName: jp.Name,
		BaseURL:     jp.Options.BaseURL,
		APIKey:      jp.Options.APIKey,
		Headers:     headers,
		NoAuth:      jp.NoAuth,
		Models:      models,
		ModelMeta:   meta,
	}
}

// merge returns a new Config that is the project config overlaid on the global config.
// Providers with matching names are replaced entirely; scalars use project value if non-zero.
func merge(global, project Config) Config {
	out := global

	if project.DefaultModel != "" {
		out.DefaultModel = project.DefaultModel
	}
	if project.FallbackModel != "" {
		out.FallbackModel = project.FallbackModel
	}

	if len(project.Providers) > 0 {
		// Build index of project providers by name.
		projByName := make(map[string]Provider, len(project.Providers))
		for _, p := range project.Providers {
			projByName[p.Name] = p
		}

		// Replace global providers that have a project-level override.
		merged := make([]Provider, 0, len(out.Providers)+len(project.Providers))
		seen := make(map[string]bool)
		for _, gp := range out.Providers {
			if pp, ok := projByName[gp.Name]; ok {
				merged = append(merged, pp)
			} else {
				merged = append(merged, gp)
			}
			seen[gp.Name] = true
		}
		// Append project-only providers (not in global).
		for _, pp := range project.Providers {
			if !seen[pp.Name] {
				merged = append(merged, pp)
			}
		}
		out.Providers = merged
	}

	if len(project.ModelCosts) > 0 {
		if out.ModelCosts == nil {
			out.ModelCosts = make(map[string]Cost)
		}
		for k, v := range project.ModelCosts {
			out.ModelCosts[k] = v
		}
	}

	return out
}

// envVarRE matches ${VAR_NAME} references (YAML-style).
var envVarRE = regexp.MustCompile(`\$\{([^}]+)\}`)

// envColonRE matches {env:VAR_NAME} references (JSON/OpenCode-style).
var envColonRE = regexp.MustCompile(`\{env:([^}]+)\}`)

// expandEnv replaces ${VAR} and {env:VAR} references with os.Getenv(VAR).
func expandEnv(s string) string {
	s = envVarRE.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		return os.Getenv(varName)
	})
	s = envColonRE.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[5 : len(match)-1]
		return os.Getenv(varName)
	})
	return s
}

// expandEnvInConfig expands ${VAR} and {env:VAR} in all string fields of the config.
func expandEnvInConfig(cfg *Config) {
	cfg.DefaultModel = expandEnv(cfg.DefaultModel)
	cfg.FallbackModel = expandEnv(cfg.FallbackModel)

	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		p.Name = expandEnv(p.Name)
		p.BaseURL = expandEnv(p.BaseURL)
		p.APIKey = expandEnv(p.APIKey)
		p.DisplayName = expandEnv(p.DisplayName)
		for j := range p.Models {
			p.Models[j] = expandEnv(p.Models[j])
		}
		if p.Headers != nil {
			expanded := make(map[string]string, len(p.Headers))
			for k, v := range p.Headers {
				expanded[expandEnv(k)] = expandEnv(v)
			}
			p.Headers = expanded
		}
		if p.ModelMeta != nil {
			for k, v := range p.ModelMeta {
				v.DisplayName = expandEnv(v.DisplayName)
				p.ModelMeta[k] = v
			}
		}
	}
}

// applyEnvOverrides applies FORGE_* environment variable overrides on top of the
// loaded config. Precedence: env var > project config > global config.
func applyEnvOverrides(cfg *Config) {
	if m := os.Getenv("FORGE_MODEL"); m != "" {
		cfg.DefaultModel = m
	}

	// ANTHROPIC_API_KEY targets the "anthropic" provider specifically.
	if ak := os.Getenv("ANTHROPIC_API_KEY"); ak != "" {
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == "anthropic" {
				cfg.Providers[i].APIKey = ak
			}
		}
	}

	// FORGE_API_KEY sets the key on the provider serving the default model.
	if fk := os.Getenv("FORGE_API_KEY"); fk != "" {
		if p := findProviderForModel(cfg, cfg.DefaultModel); p != nil {
			p.APIKey = fk
		}
	}

	// FORGE_BASE_URL overrides the base URL for the default model's provider.
	if bu := os.Getenv("FORGE_BASE_URL"); bu != "" {
		if p := findProviderForModel(cfg, cfg.DefaultModel); p != nil {
			p.BaseURL = bu
		}
	}
}

// findProviderForModel returns a pointer to the provider serving model, or nil.
func findProviderForModel(cfg *Config, model string) *Provider {
	if model == "" {
		return nil
	}
	for i := range cfg.Providers {
		for _, m := range cfg.Providers[i].Models {
			if m == model {
				return &cfg.Providers[i]
			}
		}
	}
	// Try suffix match for namespaced models like "deepseek/deepseek-r1".
	if strings.Contains(model, "/") {
		suffix := model[strings.LastIndex(model, "/")+1:]
		for i := range cfg.Providers {
			for _, m := range cfg.Providers[i].Models {
				if m == suffix {
					return &cfg.Providers[i]
				}
			}
		}
	}
	return nil
}

// SaveCustomProvider writes a custom provider entry to config.json at the given
// home directory. It reads the existing file (or creates a new one), upserts
// the provider, and writes it back.
func SaveCustomProvider(homeDir, id, displayName, baseURL, apiKey string) error {
	path := filepath.Join(homeDir, ".forge", "config.json")

	jc := JSONConfig{
		Providers: make(map[string]JSONProvider),
	}

	// Read existing config.json if it exists.
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &jc)
	}
	if jc.Providers == nil {
		jc.Providers = make(map[string]JSONProvider)
	}

	// Preserve existing model entries for this provider if updating.
	existing := jc.Providers[id]

	jc.Providers[id] = JSONProvider{
		Name:   displayName,
		NoAuth: apiKey == "",
		Options: JSONProviderOptions{
			BaseURL: baseURL,
			APIKey:  apiKey,
		},
		Models: existing.Models,
	}

	out, err := json.MarshalIndent(jc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
