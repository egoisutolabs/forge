package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltInAgents(t *testing.T) {
	agents := BuiltInAgents()
	if len(agents) < 2 {
		t.Fatalf("expected at least 2 built-in agents, got %d", len(agents))
	}

	names := make(map[string]bool)
	for _, a := range agents {
		names[a.Name] = true
		if a.Description == "" {
			t.Errorf("agent %q has empty description", a.Name)
		}
		if a.SystemPrompt == "" {
			t.Errorf("agent %q has empty system prompt", a.Name)
		}
	}

	for _, want := range []string{"Explore", "Plan"} {
		if !names[want] {
			t.Errorf("missing built-in agent: %s", want)
		}
	}
}

func TestFindAgent_CaseInsensitive(t *testing.T) {
	agents := BuiltInAgents()

	for _, name := range []string{"Explore", "explore", "EXPLORE"} {
		got := FindAgent(agents, name)
		if got == nil {
			t.Errorf("FindAgent(%q) = nil, want Explore", name)
		} else if got.Name != "Explore" {
			t.Errorf("FindAgent(%q).Name = %q, want Explore", name, got.Name)
		}
	}
}

func TestFindAgent_NotFound(t *testing.T) {
	agents := BuiltInAgents()
	got := FindAgent(agents, "nonexistent-agent-xyz")
	if got != nil {
		t.Errorf("expected nil for unknown agent, got %+v", got)
	}
}

func TestLoadAgentsDir_NonExistent(t *testing.T) {
	agents, err := LoadAgentsDir("/no/such/directory/forge-agents-xyz")
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	// Should still return built-ins
	if len(agents) < 2 {
		t.Errorf("expected built-in agents even for missing dir, got %d", len(agents))
	}
}

func TestLoadAgentsDir_Empty(t *testing.T) {
	dir := t.TempDir()
	agents, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only built-ins
	if len(agents) != len(BuiltInAgents()) {
		t.Errorf("expected only built-ins (%d), got %d", len(BuiltInAgents()), len(agents))
	}
}

func TestLoadAgentsDir_WithFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: TestAgent
description: A test agent
model: haiku
max_turns: 10
tools:
  - Read
  - Glob
---
You are a test agent. Do test things.
`
	if err := os.WriteFile(filepath.Join(dir, "test-agent.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found *AgentDefinition
	for i := range agents {
		if agents[i].Name == "TestAgent" {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("TestAgent not found in %v", agents)
	}
	if found.Description != "A test agent" {
		t.Errorf("description = %q, want %q", found.Description, "A test agent")
	}
	if found.Model != "haiku" {
		t.Errorf("model = %q, want haiku", found.Model)
	}
	if found.MaxTurns != 10 {
		t.Errorf("max_turns = %d, want 10", found.MaxTurns)
	}
	if len(found.Tools) != 2 || found.Tools[0] != "Read" || found.Tools[1] != "Glob" {
		t.Errorf("tools = %v, want [Read Glob]", found.Tools)
	}
	if found.SystemPrompt != "You are a test agent. Do test things." {
		t.Errorf("system_prompt = %q", found.SystemPrompt)
	}
}

func TestLoadAgentsDir_InlineTools(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: InlineAgent
description: Agent with inline tools
tools: Read, Glob, Grep
---
System prompt here.
`
	if err := os.WriteFile(filepath.Join(dir, "inline.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found *AgentDefinition
	for i := range agents {
		if agents[i].Name == "InlineAgent" {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		t.Fatal("InlineAgent not found")
	}
	if len(found.Tools) != 3 {
		t.Errorf("tools = %v, want 3 tools", found.Tools)
	}
}

func TestLoadAgentsDir_NameFallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	content := `---
description: Unnamed agent
---
System prompt.
`
	if err := os.WriteFile(filepath.Join(dir, "my-custom-agent.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found *AgentDefinition
	for i := range agents {
		if agents[i].Name == "my-custom-agent" {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		t.Error("agent name should fall back to filename without .md extension")
	}
}

func TestLoadAgentsDir_IgnoresNonMd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-an-agent.txt"), []byte("---\nname: TxtAgent\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	agents, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range agents {
		if a.Name == "TxtAgent" {
			t.Error("non-.md files should be ignored")
		}
	}
}

func TestParseFrontmatter_BackgroundFlag(t *testing.T) {
	lines := []string{
		"name: BgAgent",
		"background: true",
	}
	def := parseFrontmatter(lines)
	if !def.Background {
		t.Error("background should be true")
	}
}
