package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/hooks"
)

// manifestFileName is the name of the plugin manifest file inside each plugin dir.
const manifestFileName = "plugin.json"

// safePath validates that rel is a safe relative path and that joining it with
// base does not escape base. It rejects absolute paths, ".." segments, and any
// result that resolves outside base after cleaning.
func safePath(base, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute path not allowed: %q", rel)
	}
	// Reject any ".." component.
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal not allowed: %q", rel)
		}
	}
	joined := filepath.Join(base, rel)
	cleanBase := filepath.Clean(base) + string(filepath.Separator)
	cleanJoined := filepath.Clean(joined)
	// The joined path must be equal to base or a child of base.
	if cleanJoined != filepath.Clean(base) && !strings.HasPrefix(cleanJoined, cleanBase) {
		return "", fmt.Errorf("path %q escapes plugin directory %q", rel, base)
	}
	return cleanJoined, nil
}

// LoadPlugin reads and validates a plugin from dir.
//
// It expects dir to contain a plugin.json manifest. All paths declared in the
// manifest are resolved relative to dir and validated to exist.
//
// Returns an error if:
//   - plugin.json is missing or unparseable
//   - name or version fields are absent
//   - any declared skill, agent, or hooks path does not exist on disk
func LoadPlugin(dir string) (*Plugin, error) {
	dir = filepath.Clean(dir)

	// Read manifest
	manifestPath := filepath.Join(dir, manifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin directory %q has no %s", dir, manifestFileName)
		}
		return nil, fmt.Errorf("reading %s: %w", manifestPath, err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", manifestPath, err)
	}

	// Validate required fields
	if err := validateManifest(&manifest, manifestPath); err != nil {
		return nil, err
	}

	plugin := &Plugin{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Dir:         dir,
		Enabled:     true, // enabled by default on load
	}

	// Resolve and validate skill paths
	for _, rel := range manifest.SkillPaths {
		abs, err := safePath(dir, rel)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: skill path: %w", manifest.Name, err)
		}
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("plugin %q: skill path %q does not exist", manifest.Name, abs)
			}
			return nil, fmt.Errorf("plugin %q: checking skill path %q: %w", manifest.Name, abs, err)
		}
		plugin.SkillPaths = append(plugin.SkillPaths, abs)
	}

	// Resolve and validate agent paths
	for _, rel := range manifest.AgentPaths {
		abs, err := safePath(dir, rel)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: agent path: %w", manifest.Name, err)
		}
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("plugin %q: agent path %q does not exist", manifest.Name, abs)
			}
			return nil, fmt.Errorf("plugin %q: checking agent path %q: %w", manifest.Name, abs, err)
		}
		plugin.AgentPaths = append(plugin.AgentPaths, abs)
	}

	// Load hook config (optional)
	if manifest.HooksFile != "" {
		hooksPath, err := safePath(dir, manifest.HooksFile)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: hooks file: %w", manifest.Name, err)
		}
		hooksData, err := os.ReadFile(hooksPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("plugin %q: hooks file %q does not exist", manifest.Name, hooksPath)
			}
			return nil, fmt.Errorf("plugin %q: reading hooks file %q: %w", manifest.Name, hooksPath, err)
		}
		var hooksSettings hooks.HooksSettings
		if err := json.Unmarshal(hooksData, &hooksSettings); err != nil {
			return nil, fmt.Errorf("plugin %q: parsing hooks file %q: %w", manifest.Name, hooksPath, err)
		}
		plugin.HookConfigs = hooksSettings
	}

	return plugin, nil
}

// DiscoverPlugins scans baseDir for plugin subdirectories.
//
// Each immediate subdirectory of baseDir that contains a plugin.json is
// attempted as a plugin. Subdirectories that lack a plugin.json are silently
// skipped. Subdirectories whose plugin.json is invalid are collected as errors
// but scanning continues.
//
// Returns all successfully loaded plugins and a combined error listing any
// failures. A non-nil error does not mean the returned slice is empty.
func DiscoverPlugins(baseDir string) ([]*Plugin, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No plugins dir yet — not an error, just empty
			return nil, nil
		}
		return nil, fmt.Errorf("scanning plugin directory %q: %w", baseDir, err)
	}

	var plugins []*Plugin
	var errs []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(baseDir, entry.Name())

		// Skip directories without a manifest
		manifestPath := filepath.Join(pluginDir, manifestFileName)
		if _, statErr := os.Stat(manifestPath); os.IsNotExist(statErr) {
			continue
		}

		plugin, loadErr := LoadPlugin(pluginDir)
		if loadErr != nil {
			errs = append(errs, loadErr.Error())
			continue
		}
		plugins = append(plugins, plugin)
	}

	if len(errs) > 0 {
		return plugins, fmt.Errorf("plugin load errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return plugins, nil
}

// validateManifest checks that the manifest has all required fields.
func validateManifest(m *PluginManifest, path string) error {
	var missing []string
	if strings.TrimSpace(m.Name) == "" {
		missing = append(missing, "name")
	}
	if strings.TrimSpace(m.Version) == "" {
		missing = append(missing, "version")
	}
	if len(missing) > 0 {
		return fmt.Errorf("plugin manifest %q missing required fields: %s",
			path, strings.Join(missing, ", "))
	}
	return nil
}
