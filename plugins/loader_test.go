package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/hooks"
)

// writeFile creates a file with the given content in dir.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return path
}

// makeManifest serialises a PluginManifest to JSON.
func makeManifest(t *testing.T, m PluginManifest) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return string(b)
}

// ---- LoadPlugin ----

func TestLoadPlugin_Minimal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:    "my-plugin",
		Version: "1.0.0",
	}))

	p, err := LoadPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "my-plugin" {
		t.Errorf("Name = %q, want my-plugin", p.Name)
	}
	if p.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", p.Version)
	}
	if p.Dir != dir {
		t.Errorf("Dir = %q, want %q", p.Dir, dir)
	}
	if !p.Enabled {
		t.Error("newly loaded plugin should be enabled by default")
	}
}

func TestLoadPlugin_NoManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing plugin.json")
	}
}

func TestLoadPlugin_MissingName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, `{"version":"1.0.0"}`)
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadPlugin_MissingVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, `{"name":"foo"}`)
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestLoadPlugin_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, `{not valid json}`)
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadPlugin_SkillPaths(t *testing.T) {
	dir := t.TempDir()
	// Create skill directory
	os.MkdirAll(filepath.Join(dir, "skills"), 0o755)

	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "skill-plugin",
		Version:    "1.0.0",
		SkillPaths: []string{"skills"},
	}))

	p, err := LoadPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.SkillPaths) != 1 {
		t.Fatalf("expected 1 skill path, got %d", len(p.SkillPaths))
	}
	if p.SkillPaths[0] != filepath.Join(dir, "skills") {
		t.Errorf("SkillPaths[0] = %q, want %q", p.SkillPaths[0], filepath.Join(dir, "skills"))
	}
}

func TestLoadPlugin_SkillPathNotExist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "bad-plugin",
		Version:    "1.0.0",
		SkillPaths: []string{"nonexistent-dir"},
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for nonexistent skill path")
	}
}

func TestLoadPlugin_AgentPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "agents"), 0o755)

	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "agent-plugin",
		Version:    "1.0.0",
		AgentPaths: []string{"agents"},
	}))

	p, err := LoadPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.AgentPaths) != 1 {
		t.Fatalf("expected 1 agent path, got %d", len(p.AgentPaths))
	}
}

func TestLoadPlugin_AgentPathNotExist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "bad-plugin",
		Version:    "1.0.0",
		AgentPaths: []string{"nonexistent-agents"},
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for nonexistent agent path")
	}
}

func TestLoadPlugin_HooksFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "hooks"), 0o755)

	hooksJSON, _ := json.Marshal(hooks.HooksSettings{
		hooks.HookEventPreToolUse: []hooks.HookMatcher{
			{Matcher: "Bash", Hooks: []hooks.HookConfig{{Command: "echo pre"}}},
		},
	})
	writeFile(t, dir, "hooks/hooks.json", string(hooksJSON))

	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:      "hook-plugin",
		Version:   "1.0.0",
		HooksFile: "hooks/hooks.json",
	}))

	p, err := LoadPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matchers, ok := p.HookConfigs[hooks.HookEventPreToolUse]
	if !ok || len(matchers) != 1 {
		t.Fatalf("expected 1 PreToolUse hook matcher, got hooks=%v", p.HookConfigs)
	}
	if matchers[0].Matcher != "Bash" {
		t.Errorf("matcher = %q, want Bash", matchers[0].Matcher)
	}
}

func TestLoadPlugin_HooksFileNotExist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:      "bad-plugin",
		Version:   "1.0.0",
		HooksFile: "nonexistent.json",
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for nonexistent hooks file")
	}
}

func TestLoadPlugin_HooksFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hooks.json", "{bad json}")
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:      "bad-plugin",
		Version:   "1.0.0",
		HooksFile: "hooks.json",
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for invalid hooks JSON")
	}
}

func TestLoadPlugin_Description(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:        "desc-plugin",
		Version:     "1.0.0",
		Description: "A useful plugin",
	}))
	p, err := LoadPlugin(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Description != "A useful plugin" {
		t.Errorf("Description = %q, want 'A useful plugin'", p.Description)
	}
}

// ---- safePath / path traversal ----

func TestSafePath_NormalRelative(t *testing.T) {
	base := t.TempDir()
	got, err := safePath(base, "skills/foo.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "skills", "foo.md")
	if got != want {
		t.Errorf("safePath = %q, want %q", got, want)
	}
}

func TestSafePath_SubdirectoryPath(t *testing.T) {
	base := t.TempDir()
	got, err := safePath(base, "hooks/pre.sh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(base, "hooks", "pre.sh")
	if got != want {
		t.Errorf("safePath = %q, want %q", got, want)
	}
}

func TestSafePath_RejectsDotDotTraversal(t *testing.T) {
	base := t.TempDir()
	_, err := safePath(base, "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for ../../../etc/passwd")
	}
}

func TestSafePath_RejectsAbsolutePath(t *testing.T) {
	base := t.TempDir()
	_, err := safePath(base, "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path /etc/passwd")
	}
}

func TestSafePath_RejectsCleanButEscaping(t *testing.T) {
	base := t.TempDir()
	_, err := safePath(base, "foo/../../bar")
	if err == nil {
		t.Fatal("expected error for foo/../../bar")
	}
}

func TestLoadPlugin_SkillPathTraversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "evil-plugin",
		Version:    "1.0.0",
		SkillPaths: []string{"../../../etc/passwd"},
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for skill path traversal")
	}
}

func TestLoadPlugin_AgentPathTraversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:       "evil-plugin",
		Version:    "1.0.0",
		AgentPaths: []string{"/etc/passwd"},
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for absolute agent path")
	}
}

func TestLoadPlugin_HooksFileTraversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, manifestFileName, makeManifest(t, PluginManifest{
		Name:      "evil-plugin",
		Version:   "1.0.0",
		HooksFile: "foo/../../bar/hooks.json",
	}))
	_, err := LoadPlugin(dir)
	if err == nil {
		t.Fatal("expected error for hooks file path traversal")
	}
}

// ---- DiscoverPlugins ----

func TestDiscoverPlugins_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscoverPlugins_NotExist(t *testing.T) {
	// A non-existent base dir should return nil, nil (not an error)
	plugins, err := DiscoverPlugins("/tmp/forge-test-nonexistent-dir-12345")
	if err != nil {
		t.Fatalf("unexpected error for missing base dir: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscoverPlugins_SkipsNonDirs(t *testing.T) {
	dir := t.TempDir()
	// Plain file — should be ignored
	writeFile(t, dir, "some-file.txt", "not a plugin")

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscoverPlugins_SkipsDirsWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "not-a-plugin"), 0o755)

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins for dir without manifest, got %d", len(plugins))
	}
}

func TestDiscoverPlugins_MultiplePlugins(t *testing.T) {
	base := t.TempDir()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		d := filepath.Join(base, name)
		os.MkdirAll(d, 0o755)
		writeFile(t, d, manifestFileName, makeManifest(t, PluginManifest{
			Name:    name,
			Version: "0.1.0",
		}))
	}

	plugins, err := DiscoverPlugins(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}
}

func TestDiscoverPlugins_ContinuesAfterBadPlugin(t *testing.T) {
	base := t.TempDir()

	// Good plugin
	goodDir := filepath.Join(base, "good")
	os.MkdirAll(goodDir, 0o755)
	writeFile(t, goodDir, manifestFileName, makeManifest(t, PluginManifest{
		Name:    "good",
		Version: "1.0.0",
	}))

	// Bad plugin (missing version)
	badDir := filepath.Join(base, "bad")
	os.MkdirAll(badDir, 0o755)
	writeFile(t, badDir, manifestFileName, `{"name":"bad"}`)

	plugins, err := DiscoverPlugins(base)

	// Should return the good plugin AND an error about the bad one
	if err == nil {
		t.Error("expected error for bad plugin")
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 good plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "good" {
		t.Errorf("expected good plugin, got %q", plugins[0].Name)
	}
}
