package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// mockTool is a minimal tools.Tool implementation for testing.
type mockTool struct{ name string }

func (m *mockTool) Name() string                 { return m.name }
func (m *mockTool) Description() string          { return "mock" }
func (m *mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "ok"}, nil
}
func (m *mockTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (m *mockTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (m *mockTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (m *mockTool) IsReadOnly(_ json.RawMessage) bool        { return false }

func mt(name string) tools.Tool { return &mockTool{name: name} }

func TestFilterToolsForAgent_RemovesAlwaysDisallowed(t *testing.T) {
	input := []tools.Tool{
		mt("Read"),
		mt("Bash"),
		mt("Agent"),           // must be removed
		mt("AskUserQuestion"), // must be removed
		mt("TaskStop"),        // must be removed
		mt("Glob"),
	}

	got := FilterToolsForAgent(input, false)

	names := toolNames(got)
	for _, banned := range []string{"Agent", "AskUserQuestion", "TaskStop"} {
		if names[banned] {
			t.Errorf("banned tool %q should not be in filtered set", banned)
		}
	}
	for _, allowed := range []string{"Read", "Bash", "Glob"} {
		if !names[allowed] {
			t.Errorf("allowed tool %q should be present in filtered set", allowed)
		}
	}
}

func TestFilterToolsForAgent_SyncAllowsNonAsyncTools(t *testing.T) {
	input := []tools.Tool{
		mt("TaskCreate"),
		mt("TaskList"),
		mt("WebFetch"),
	}
	got := FilterToolsForAgent(input, false)
	names := toolNames(got)
	for _, tool := range []string{"TaskCreate", "TaskList", "WebFetch"} {
		if !names[tool] {
			t.Errorf("sync agent should allow %q", tool)
		}
	}
}

func TestFilterToolsForAgent_AsyncRestriction(t *testing.T) {
	input := []tools.Tool{
		mt("Read"),
		mt("Write"),
		mt("Bash"),
		mt("Glob"),
		mt("Grep"),
		mt("Skill"),
		mt("ToolSearch"),
		mt("TaskCreate"), // not in async allowed
		mt("TaskList"),   // not in async allowed
		mt("WebFetch"),   // not in async allowed
	}

	got := FilterToolsForAgent(input, true)
	names := toolNames(got)

	for _, allowed := range []string{"Read", "Write", "Bash", "Glob", "Grep", "Skill", "ToolSearch"} {
		if !names[allowed] {
			t.Errorf("async agent should allow %q", allowed)
		}
	}
	for _, blocked := range []string{"TaskCreate", "TaskList", "WebFetch"} {
		if names[blocked] {
			t.Errorf("async agent should NOT allow %q", blocked)
		}
	}
}

func TestFilterToolsForAgent_EmptyInput(t *testing.T) {
	got := FilterToolsForAgent(nil, false)
	if len(got) != 0 {
		t.Errorf("empty input → empty output, got %d tools", len(got))
	}
}

func TestFilterToolsByNames_NilAllowed(t *testing.T) {
	input := []tools.Tool{mt("Read"), mt("Bash")}
	got := FilterToolsByNames(input, nil)
	if len(got) != 2 {
		t.Errorf("nil allowed → all returned, got %d", len(got))
	}
}

func TestFilterToolsByNames_EmptyAllowed(t *testing.T) {
	input := []tools.Tool{mt("Read"), mt("Bash")}
	got := FilterToolsByNames(input, []string{})
	if len(got) != 2 {
		t.Errorf("empty allowed → all returned, got %d", len(got))
	}
}

func TestFilterToolsByNames_Subset(t *testing.T) {
	input := []tools.Tool{mt("Read"), mt("Bash"), mt("Glob")}
	got := FilterToolsByNames(input, []string{"Read", "Glob"})
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}
	names := toolNames(got)
	if !names["Read"] || !names["Glob"] || names["Bash"] {
		t.Errorf("unexpected tools: %v", toolNameList(got))
	}
}

func TestFilterToolsByNames_NonexistentAllowed(t *testing.T) {
	input := []tools.Tool{mt("Read")}
	got := FilterToolsByNames(input, []string{"DoesNotExist"})
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

// helpers

func toolNames(tt []tools.Tool) map[string]bool {
	m := make(map[string]bool, len(tt))
	for _, t := range tt {
		m[t.Name()] = true
	}
	return m
}

func toolNameList(tt []tools.Tool) []string {
	names := make([]string, len(tt))
	for i, t := range tt {
		names[i] = t.Name()
	}
	return names
}
