package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkillFile creates a skill markdown file in dir.
func writeSkillFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadSkillsDir_MissingDir(t *testing.T) {
	r := NewRegistry()
	err := LoadSkillsDir("/nonexistent/path/xyz", "user", r)
	if err != nil {
		t.Errorf("should not error for missing dir, got: %v", err)
	}
	if r.Len() != 0 {
		t.Error("expected empty registry for missing dir")
	}
}

func TestLoadSkillsDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry()
	if err := LoadSkillsDir(dir, "user", r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Len() != 0 {
		t.Error("expected empty registry for empty dir")
	}
}

func TestLoadSkillsDir_SingleSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "commit.md", "---\ndescription: Commit changes\n---\n# Prompt")

	r := NewRegistry()
	if err := LoadSkillsDir(dir, "user", r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s := r.Lookup("commit")
	if s == nil {
		t.Fatal("expected 'commit' skill to be registered")
	}
	if s.Description != "Commit changes" {
		t.Errorf("Description = %q", s.Description)
	}
	if s.Source != "user" {
		t.Errorf("Source = %q, want %q", s.Source, "user")
	}
}

func TestLoadSkillsDir_NameFromFilename(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "my-skill.md", "# Body")

	r := NewRegistry()
	LoadSkillsDir(dir, "project", r) //nolint:errcheck

	if r.Lookup("my-skill") == nil {
		t.Error("expected skill named 'my-skill'")
	}
}

func TestLoadSkillsDir_SubdirectorySkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "sub/review.md", "---\ndescription: Review code\n---\n")

	r := NewRegistry()
	LoadSkillsDir(dir, "project", r) //nolint:errcheck

	if r.Lookup("sub/review") == nil {
		t.Error("expected skill named 'sub/review'")
	}
}

func TestLoadSkillsDir_IgnoresNonMd(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "notskill.txt", "some text")
	writeSkillFile(t, dir, "skill.md", "# Prompt")

	r := NewRegistry()
	LoadSkillsDir(dir, "user", r) //nolint:errcheck

	if r.Len() != 1 {
		t.Errorf("expected 1 skill, got %d", r.Len())
	}
}

func TestLoadSkillsDir_PromptFunctionReturnsBody(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "foo.md", "---\ndescription: Foo\n---\nFoo prompt body")

	r := NewRegistry()
	LoadSkillsDir(dir, "user", r) //nolint:errcheck

	s := r.Lookup("foo")
	if s == nil {
		t.Fatal("skill not found")
	}
	if s.Prompt == nil {
		t.Fatal("Prompt is nil")
	}
	if got := s.Prompt(""); got != "Foo prompt body" {
		t.Errorf("Prompt() = %q, want %q", got, "Foo prompt body")
	}
}

func TestLoadSkillsDir_DescriptionFallsBackToName(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "no-desc.md", "# Prompt without frontmatter description")

	r := NewRegistry()
	LoadSkillsDir(dir, "user", r) //nolint:errcheck

	s := r.Lookup("no-desc")
	if s == nil {
		t.Fatal("skill not found")
	}
	if s.Description != "no-desc" {
		t.Errorf("Description = %q, want %q (name fallback)", s.Description, "no-desc")
	}
}

func TestLoadSkillsDir_AllowedTools(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "safe.md", "---\nallowed-tools: [Read, Glob]\n---\n")

	r := NewRegistry()
	LoadSkillsDir(dir, "user", r) //nolint:errcheck

	s := r.Lookup("safe")
	if s == nil {
		t.Fatal("skill not found")
	}
	if len(s.AllowedTools) != 2 {
		t.Errorf("AllowedTools = %v, want [Read, Glob]", s.AllowedTools)
	}
}

func TestLoadSkillsDir_MultipleSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "a.md", "# A")
	writeSkillFile(t, dir, "b.md", "# B")
	writeSkillFile(t, dir, "c.md", "# C")

	r := NewRegistry()
	LoadSkillsDir(dir, "user", r) //nolint:errcheck

	if r.Len() != 3 {
		t.Errorf("expected 3 skills, got %d", r.Len())
	}
}

func TestLoadDefaultSkills_ProjectOverridesUser(t *testing.T) {
	// Create two dirs simulating user and project skill dirs.
	userDir := t.TempDir()
	projDir := t.TempDir()

	writeSkillFile(t, userDir, "shared.md", "---\ndescription: user version\n---\n")
	writeSkillFile(t, projDir, "shared.md", "---\ndescription: project version\n---\n")

	r := NewRegistry()
	// Load user first, then project (simulates DefaultSearchPaths order).
	LoadSkillsDir(userDir, "user", r)    //nolint:errcheck
	LoadSkillsDir(projDir, "project", r) //nolint:errcheck

	s := r.Lookup("shared")
	if s.Description != "project version" {
		t.Errorf("project skill should override user, got %q", s.Description)
	}
}
