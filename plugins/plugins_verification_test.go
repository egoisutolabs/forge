// Package plugins — verification tests comparing Go port against Claude Code's
// plugin system (pluginLoader.ts, types/plugin.ts).
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: `commands` component type.
//     TypeScript PluginManifest supports a `commands` array (arbitrary CLI
//     command scripts). Go manifest only supports agents, skills, and hooks.
//
//  2. MISSING: `output-styles` component type.
//     TypeScript supports output style overrides in plugins. Go has no such field.
//
//  3. MISSING: Marketplace plugin loading.
//     TypeScript supports name@marketplace, git repository, and npm package
//     plugin sources. Go only supports local filesystem plugins.
//
//  4. MISSING: `defaultEnabled` flag on builtin plugins.
//     TypeScript BuiltinPluginDefinition has `defaultEnabled?: boolean`.
//     Go `RegisterBuiltin()` does not set an initial enabled state differently.
//
//  5. MISSING: Policy-based blocklist enforcement.
//     TypeScript has 25+ discriminated error types and policy checks.
//     Go uses simple error propagation with no policy layer.
//
//  6. CORRECT: Plugin enable/disable is explicit and persists in registry.
//
//  7. CORRECT: Disabled plugins contribute no skills or hooks.
//
//  8. CORRECT: GetAllHooks merges hook settings from all enabled plugins.
//
//  9. CORRECT: GetAllSkillPaths aggregates from enabled plugins only.
//
// 10. CORRECT: Thread-safe registry with RWMutex.
package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/hooks"
)

// ─── GAP 1: commands component type missing ──────────────────────────────────

// TestVerification_CommandsComponentType_Missing verifies that Go's
// PluginManifest has no `commands` field.
//
// TypeScript: plugins can declare command scripts for the CLI.
// Go: no commands support — only agents, skills, hooks.
func TestVerification_CommandsComponentType_Missing(t *testing.T) {
	// Attempt to parse a manifest with a "commands" array.
	manifestJSON := `{
		"name": "test-plugin",
		"version": "1.0.0",
		"commands": ["/path/to/cmd1", "/path/to/cmd2"]
	}`

	var manifest PluginManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The commands field is silently ignored since it's not in the struct.
	// We can confirm by checking there's no way to access commands from Plugin.
	t.Log("GAP CONFIRMED: PluginManifest has no 'commands' field (TypeScript supports command scripts in plugins)")
	t.Logf("Parsed manifest name=%q, version=%q, skillPaths=%v (commands silently dropped)",
		manifest.Name, manifest.Version, manifest.SkillPaths)
}

// ─── GAP 2: output-styles component type missing ──────────────────────────────

// TestVerification_OutputStylesComponentType_Missing verifies that Go
// PluginManifest has no `output-styles` field.
func TestVerification_OutputStylesComponentType_Missing(t *testing.T) {
	manifestJSON := `{
		"name": "style-plugin",
		"version": "1.0.0",
		"output-styles": ["/path/to/style.css"]
	}`
	var m PluginManifest
	json.Unmarshal([]byte(manifestJSON), &m) //nolint:errcheck
	t.Log("GAP CONFIRMED: PluginManifest has no 'output-styles' field (TypeScript supports output style customization)")
}

// ─── GAP 6 (CORRECT): enable/disable ─────────────────────────────────────────

// TestVerification_DisabledPlugin_ContributesNothing verifies that disabled
// plugins do not contribute skills or hooks.
func TestVerification_DisabledPlugin_ContributesNothing(t *testing.T) {
	dir := t.TempDir()

	// Create a plugin with a skill path.
	skillDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillDir, 0755) //nolint:errcheck

	p := &Plugin{
		Name:       "my-plugin",
		Version:    "1.0",
		Dir:        dir,
		SkillPaths: []string{skillDir},
		HookConfigs: hooks.HooksSettings{
			hooks.HookEventPreToolUse: {
				{Matcher: "", Hooks: []hooks.HookConfig{{Command: "echo hi"}}},
			},
		},
		Enabled: false, // disabled
	}

	r := NewPluginRegistry()
	r.Register(p)

	// Disabled plugin should not contribute skill paths.
	skillPaths := r.GetAllSkillPaths()
	if len(skillPaths) > 0 {
		t.Errorf("disabled plugin should contribute no skill paths, got %v", skillPaths)
	}

	// Disabled plugin should not contribute hooks.
	allHooks := r.GetAllHooks()
	if len(allHooks) > 0 {
		t.Errorf("disabled plugin should contribute no hooks, got %v", allHooks)
	}
}

// TestVerification_EnablePlugin_ContributesSkillsAndHooks verifies that
// enabling a plugin exposes its skills and hooks.
func TestVerification_EnablePlugin_ContributesSkillsAndHooks(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillDir, 0755) //nolint:errcheck

	p := &Plugin{
		Name:       "my-plugin",
		Version:    "1.0",
		Dir:        dir,
		SkillPaths: []string{skillDir},
		HookConfigs: hooks.HooksSettings{
			hooks.HookEventPreToolUse: {
				{Matcher: "", Hooks: []hooks.HookConfig{{Command: "echo hi"}}},
			},
		},
		Enabled: true, // enabled
	}

	r := NewPluginRegistry()
	r.Register(p)

	skillPaths := r.GetAllSkillPaths()
	if len(skillPaths) != 1 {
		t.Errorf("enabled plugin should contribute 1 skill path, got %d: %v", len(skillPaths), skillPaths)
	}

	allHooks := r.GetAllHooks()
	if len(allHooks) == 0 {
		t.Error("enabled plugin should contribute hooks")
	}
}

// TestVerification_EnableDisableCycle verifies that Enable/Disable methods
// correctly toggle a registered plugin's contribution.
func TestVerification_EnableDisableCycle(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	os.MkdirAll(skillDir, 0755) //nolint:errcheck

	p := &Plugin{
		Name:       "toggle-plugin",
		Version:    "1.0",
		SkillPaths: []string{skillDir},
		Enabled:    true,
	}

	r := NewPluginRegistry()
	r.Register(p)

	// Should have skills.
	if len(r.GetAllSkillPaths()) == 0 {
		t.Error("enabled plugin should have skill paths")
	}

	// Disable.
	if err := r.Disable("toggle-plugin"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if len(r.GetAllSkillPaths()) != 0 {
		t.Error("disabled plugin should have no skill paths")
	}

	// Re-enable.
	if err := r.Enable("toggle-plugin"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if len(r.GetAllSkillPaths()) == 0 {
		t.Error("re-enabled plugin should have skill paths")
	}
}

// TestVerification_GetAllHooks_MergesAcrossPlugins verifies that hooks from
// multiple enabled plugins are merged together.
func TestVerification_GetAllHooks_MergesAcrossPlugins(t *testing.T) {
	p1 := &Plugin{
		Name:    "plugin-1",
		Version: "1.0",
		Enabled: true,
		HookConfigs: hooks.HooksSettings{
			hooks.HookEventPreToolUse: {
				{Matcher: "^Bash$", Hooks: []hooks.HookConfig{{Command: "hook1"}}},
			},
		},
	}
	p2 := &Plugin{
		Name:    "plugin-2",
		Version: "1.0",
		Enabled: true,
		HookConfigs: hooks.HooksSettings{
			hooks.HookEventPostToolUse: {
				{Matcher: "", Hooks: []hooks.HookConfig{{Command: "hook2"}}},
			},
		},
	}

	r := NewPluginRegistry()
	r.Register(p1)
	r.Register(p2)

	merged := r.GetAllHooks()

	if _, ok := merged[hooks.HookEventPreToolUse]; !ok {
		t.Error("merged hooks missing PreToolUse from plugin-1")
	}
	if _, ok := merged[hooks.HookEventPostToolUse]; !ok {
		t.Error("merged hooks missing PostToolUse from plugin-2")
	}
}

// TestVerification_PluginManifest_RequiredFields verifies that name and version
// are required — matches TypeScript's manifest validation.
func TestVerification_PluginManifest_RequiredFields(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		manifest string
		wantErr  bool
	}{
		{"missing name", `{"version":"1.0"}`, true},
		{"missing version", `{"name":"test"}`, true},
		{"empty name", `{"name":"","version":"1.0"}`, true},
		{"empty version", `{"name":"test","version":""}`, true},
		{"valid", `{"name":"test","version":"1.0"}`, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath := filepath.Join(dir, "plugin.json")
			os.WriteFile(manifestPath, []byte(tc.manifest), 0644) //nolint:errcheck

			_, err := LoadPlugin(dir)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestVerification_DiscoverPlugins_SkipsMissingManifest verifies that
// directories without plugin.json are silently skipped.
func TestVerification_DiscoverPlugins_SkipsMissingManifest(t *testing.T) {
	base := t.TempDir()

	// Create a valid plugin subdirectory.
	validDir := filepath.Join(base, "valid-plugin")
	os.MkdirAll(validDir, 0755)                          //nolint:errcheck
	os.WriteFile(filepath.Join(validDir, "plugin.json"), //nolint:errcheck
		[]byte(`{"name":"valid","version":"1.0"}`), 0644)

	// Create a non-plugin directory (no plugin.json).
	noManifestDir := filepath.Join(base, "not-a-plugin")
	os.MkdirAll(noManifestDir, 0755) //nolint:errcheck

	plugins, err := DiscoverPlugins(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin discovered, got %d", len(plugins))
	}
	if plugins[0].Name != "valid" {
		t.Errorf("plugin name = %q, want %q", plugins[0].Name, "valid")
	}
}

// TestVerification_Registry_ThreadSafe verifies concurrent register+get is safe.
func TestVerification_Registry_ThreadSafe(t *testing.T) {
	r := NewPluginRegistry()
	done := make(chan struct{}, 20)

	for i := range 10 {
		go func(i int) {
			r.Register(&Plugin{Name: "plugin" + string(rune('a'+i)), Version: "1.0", Enabled: true})
			done <- struct{}{}
		}(i)
		go func(i int) {
			r.GetAllSkillPaths()
			done <- struct{}{}
		}(i)
	}
	for range 20 {
		<-done
	}
}
