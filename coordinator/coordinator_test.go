package coordinator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// fakeTool is a minimal tools.Tool for filter tests.
type fakeTool struct{ name string }

func (f *fakeTool) Name() string                             { return f.name }
func (f *fakeTool) Description() string                      { return "" }
func (f *fakeTool) InputSchema() json.RawMessage             { return nil }
func (f *fakeTool) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (f *fakeTool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (f *fakeTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (f *fakeTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return nil, nil
}

// makeTools creates a slice of fake tools with the given names.
func makeTools(names ...string) []tools.Tool {
	tt := make([]tools.Tool, len(names))
	for i, n := range names {
		tt[i] = &fakeTool{name: n}
	}
	return tt
}

// --- IsCoordinatorMode ---

func TestIsCoordinatorMode_False(t *testing.T) {
	t.Setenv("FORGE_COORDINATOR_MODE", "")
	if IsCoordinatorMode() {
		t.Error("expected false when FORGE_COORDINATOR_MODE is empty")
	}
}

func TestIsCoordinatorMode_True(t *testing.T) {
	t.Setenv("FORGE_COORDINATOR_MODE", "1")
	if !IsCoordinatorMode() {
		t.Error("expected true when FORGE_COORDINATOR_MODE=1")
	}
}

func TestIsCoordinatorMode_OtherValueFalse(t *testing.T) {
	t.Setenv("FORGE_COORDINATOR_MODE", "true")
	if IsCoordinatorMode() {
		t.Error("expected false for value 'true' (only '1' is recognized)")
	}
}

// --- CoordinatorSystemPrompt ---

func TestCoordinatorSystemPrompt_NotEmpty(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if strings.TrimSpace(p) == "" {
		t.Error("expected non-empty system prompt")
	}
}

func TestCoordinatorSystemPrompt_MentionsAgent(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if !strings.Contains(p, "Agent") {
		t.Error("expected system prompt to mention Agent tool")
	}
}

func TestCoordinatorSystemPrompt_MentionsSendMessage(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if !strings.Contains(p, "SendMessage") {
		t.Error("expected system prompt to mention SendMessage tool")
	}
}

func TestCoordinatorSystemPrompt_MentionsTaskStop(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if !strings.Contains(p, "TaskStop") {
		t.Error("expected system prompt to mention TaskStop tool")
	}
}

func TestCoordinatorSystemPrompt_NoFabricationWarning(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if !strings.Contains(p, "fabricate") {
		t.Error("expected system prompt to warn against fabricating results")
	}
}

// --- CoordinatorTools ---

func TestCoordinatorTools_KeepsAllowedTools(t *testing.T) {
	all := makeTools("Agent", "SendMessage", "TaskStop", "Bash", "Read", "Write")
	got := CoordinatorTools(all)
	if len(got) != 3 {
		t.Fatalf("expected 3 coordinator tools, got %d", len(got))
	}
	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name()] = true
	}
	for _, want := range []string{"Agent", "SendMessage", "TaskStop"} {
		if !names[want] {
			t.Errorf("expected %s in coordinator tools", want)
		}
	}
}

func TestCoordinatorTools_RemovesNonCoordinatorTools(t *testing.T) {
	all := makeTools("Bash", "Read", "Write", "Glob", "Grep")
	got := CoordinatorTools(all)
	if len(got) != 0 {
		t.Errorf("expected 0 tools when none are coordinator tools, got %d", len(got))
	}
}

func TestCoordinatorTools_EmptyInput(t *testing.T) {
	got := CoordinatorTools(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 tools for nil input, got %d", len(got))
	}
}

func TestCoordinatorTools_OnlyAgentPresent(t *testing.T) {
	all := makeTools("Agent", "Bash")
	got := CoordinatorTools(all)
	if len(got) != 1 || got[0].Name() != "Agent" {
		t.Errorf("expected only Agent, got %v", got)
	}
}

func TestCoordinatorTools_AllThreeAllowed(t *testing.T) {
	all := makeTools("Agent", "SendMessage", "TaskStop")
	got := CoordinatorTools(all)
	if len(got) != 3 {
		t.Errorf("expected 3 tools when all coordinator tools present, got %d", len(got))
	}
}

// --- Wiring integration tests ---

// TestCoordinatorTools_FindsSendMessageAmongAllTools simulates the real tool
// list (which now includes SendMessage) and verifies the coordinator filter
// selects it.
func TestCoordinatorTools_FindsSendMessageAmongAllTools(t *testing.T) {
	// Simulate the full tool list from main.go buildTools
	all := makeTools(
		"Bash", "Read", "Edit", "Write", "Glob", "Grep",
		"AskUserQuestion", "EnterPlanMode", "ExitPlanMode",
		"TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TaskStop", "TaskOutput",
		"Skill", "ToolSearch", "Agent", "SendMessage",
		"Browser", "AstGrep", "WebFetch", "WebSearch",
	)
	got := CoordinatorTools(all)
	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name()] = true
	}
	if !names["SendMessage"] {
		t.Error("expected SendMessage in coordinator tools when present in allTools")
	}
	if !names["Agent"] {
		t.Error("expected Agent in coordinator tools")
	}
	if !names["TaskStop"] {
		t.Error("expected TaskStop in coordinator tools")
	}
	if len(got) != 3 {
		t.Errorf("expected exactly 3 coordinator tools, got %d", len(got))
	}
}

func TestCoordinatorSystemPrompt_IsDistinctFromEmpty(t *testing.T) {
	p := CoordinatorSystemPrompt()
	if len(p) < 50 {
		t.Errorf("coordinator system prompt is suspiciously short (%d chars)", len(p))
	}
}
