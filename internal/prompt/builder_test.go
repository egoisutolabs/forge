package prompt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuilder_DefaultSections(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:   "/tmp/project",
		Model: "claude-sonnet-4-6-20250514",
	})

	result := b.Build()

	// Should contain the key section headers
	for _, header := range []string{
		"# System",
		"# Doing tasks",
		"# Executing actions with care",
		"# Using your tools",
		"# Tone and style",
		"# Output efficiency",
		"# Environment",
	} {
		if !strings.Contains(result, header) {
			t.Errorf("missing section header: %q", header)
		}
	}
}

func TestBuilder_ContainsBoundaryMarker(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:   "/tmp/project",
		Model: "claude-sonnet-4-6-20250514",
	})

	result := b.Build()

	if !strings.Contains(result, DynamicBoundary) {
		t.Error("missing dynamic boundary marker")
	}

	// Static content should be before boundary, dynamic after
	parts := strings.SplitN(result, DynamicBoundary, 2)
	if len(parts) != 2 {
		t.Fatal("expected exactly one boundary marker")
	}
	staticPart := parts[0]
	dynamicPart := parts[1]

	if !strings.Contains(staticPart, "# System") {
		t.Error("# System should be in static part (before boundary)")
	}
	if !strings.Contains(dynamicPart, "# Environment") {
		t.Error("# Environment should be in dynamic part (after boundary)")
	}
}

func TestBuilder_EnvironmentSection(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:       "/home/user/project",
		Model:     "claude-sonnet-4-6-20250514",
		Platform:  "linux",
		Shell:     "bash",
		IsGitRepo: true,
	})

	result := b.Build()

	if !strings.Contains(result, "Primary working directory: /home/user/project") {
		t.Error("missing cwd in environment")
	}
	if !strings.Contains(result, "Platform: linux") {
		t.Error("missing platform in environment")
	}
	if !strings.Contains(result, "Shell: bash") {
		t.Error("missing shell in environment")
	}
	if !strings.Contains(result, "Is a git repository: true") {
		t.Error("missing git repo info")
	}
}

func TestBuilder_EnvironmentDetectsPlatform(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:   "/tmp",
		Model: "claude-sonnet-4-6-20250514",
		// Platform not set — should auto-detect
	})

	result := b.Build()

	if !strings.Contains(result, "Platform: "+runtime.GOOS) {
		t.Errorf("expected auto-detected platform %q in environment", runtime.GOOS)
	}
}

func TestBuilder_ModelInfo(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:   "/tmp",
		Model: "claude-opus-4-6-20250514",
	})

	result := b.Build()

	if !strings.Contains(result, "claude-opus-4-6-20250514") {
		t.Error("missing model ID in environment")
	}
}

func TestBuilder_ToolDescriptions(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:       "/tmp",
		Model:     "test-model",
		ToolNames: []string{"Bash", "Read", "Edit", "Write", "Glob", "AstGrep", "Grep"},
	})

	result := b.Build()

	// The "Using your tools" section should reference the tools
	if !strings.Contains(result, "# Using your tools") {
		t.Error("missing Using your tools section")
	}
	if !strings.Contains(result, "AstGrep FIRST") {
		t.Error("prompt should prefer AstGrep for code search")
	}
	if !strings.Contains(result, "fall back to Grep") {
		t.Error("prompt should position Grep as fallback")
	}
}

func TestBuilder_MemoryFromClaudeMD(t *testing.T) {
	// Create a temp dir with a CLAUDE.md
	dir := t.TempDir()
	claudeMD := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(claudeMD, []byte("Always use tabs for indentation."), 0644)

	b := NewBuilder(Config{
		Cwd:   dir,
		Model: "test-model",
	})

	result := b.Build()

	if !strings.Contains(result, "Always use tabs for indentation.") {
		t.Error("CLAUDE.md content not found in prompt")
	}
}

func TestBuilder_MemoryFromDotClaudeDir(t *testing.T) {
	dir := t.TempDir()
	dotClaude := filepath.Join(dir, ".claude")
	os.MkdirAll(dotClaude, 0755)
	os.WriteFile(filepath.Join(dotClaude, "CLAUDE.md"), []byte("Project uses Go 1.22."), 0644)

	b := NewBuilder(Config{
		Cwd:   dir,
		Model: "test-model",
	})

	result := b.Build()

	if !strings.Contains(result, "Project uses Go 1.22.") {
		t.Error(".claude/CLAUDE.md content not found in prompt")
	}
}

func TestBuilder_MemoryMissing(t *testing.T) {
	// No CLAUDE.md anywhere — should still produce valid prompt
	dir := t.TempDir()

	b := NewBuilder(Config{
		Cwd:   dir,
		Model: "test-model",
	})

	result := b.Build()

	if result == "" {
		t.Error("prompt should not be empty even without memory files")
	}
	// Should NOT contain memory header if no files found
	if strings.Contains(result, "project instructions") {
		t.Error("should not have memory section when no CLAUDE.md exists")
	}
}

func TestBuilder_UserMemoryFromHome(t *testing.T) {
	// Create a temp "home" with .claude/CLAUDE.md
	home := t.TempDir()
	dotClaude := filepath.Join(home, ".claude")
	os.MkdirAll(dotClaude, 0755)
	os.WriteFile(filepath.Join(dotClaude, "CLAUDE.md"), []byte("I prefer verbose output."), 0644)

	b := NewBuilder(Config{
		Cwd:     t.TempDir(), // different from home
		Model:   "test-model",
		HomeDir: home,
	})

	result := b.Build()

	if !strings.Contains(result, "I prefer verbose output.") {
		t.Error("user home CLAUDE.md not found in prompt")
	}
	if !strings.Contains(result, "global instructions") {
		t.Error("user memory should be labeled as global instructions")
	}
}

func TestBuilder_CustomSystemPrompt(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:                "/tmp",
		Model:              "test-model",
		CustomSystemPrompt: "You are a helpful pirate.",
	})

	result := b.Build()

	// Custom prompt replaces default
	if !strings.Contains(result, "You are a helpful pirate.") {
		t.Error("custom system prompt not found")
	}
	// Should NOT have the standard sections
	if strings.Contains(result, "# Doing tasks") {
		t.Error("standard sections should not appear with custom system prompt")
	}
}

func TestBuilder_AppendSystemPrompt(t *testing.T) {
	b := NewBuilder(Config{
		Cwd:                "/tmp",
		Model:              "test-model",
		AppendSystemPrompt: "Always respond in French.",
	})

	result := b.Build()

	// Standard sections should still be present
	if !strings.Contains(result, "# System") {
		t.Error("standard sections should appear with append prompt")
	}
	// Appended content should be at the end
	if !strings.Contains(result, "Always respond in French.") {
		t.Error("appended system prompt not found")
	}
	// Appended content should come after environment
	envIdx := strings.Index(result, "# Environment")
	appendIdx := strings.Index(result, "Always respond in French.")
	if appendIdx < envIdx {
		t.Error("appended prompt should come after environment section")
	}
}

func TestBuilder_KnowledgeCutoff(t *testing.T) {
	tests := []struct {
		model  string
		expect string
	}{
		{"claude-opus-4-6-20250514", "May 2025"},
		{"claude-sonnet-4-6-20250514", "August 2025"},
		{"claude-haiku-4-5-20251001", "February 2025"},
	}

	for _, tt := range tests {
		b := NewBuilder(Config{Cwd: "/tmp", Model: tt.model})
		result := b.Build()
		if !strings.Contains(result, tt.expect) {
			t.Errorf("model %s: expected knowledge cutoff %q in prompt", tt.model, tt.expect)
		}
	}
}
