package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

func TestResolveModel(t *testing.T) {
	r := &PhaseRunner{Model: "claude-sonnet-4-6"}

	cases := []struct {
		hint string
		want string
	}{
		{"", "claude-sonnet-4-6"},
		{"inherit", "claude-sonnet-4-6"},
		{"Inherit", "claude-sonnet-4-6"},
		{"sonnet", "claude-sonnet-4-6"},
		{"opus", "claude-opus-4-6"},
		{"haiku", "claude-haiku-4-5-20251001"},
		{"claude-custom-model-id", "claude-custom-model-id"},
	}
	for _, c := range cases {
		t.Run(c.hint, func(t *testing.T) {
			got := r.resolveModel(c.hint)
			if got != c.want {
				t.Errorf("resolveModel(%q) = %q, want %q", c.hint, got, c.want)
			}
		})
	}
}

func TestFilterToolsForPhase_ReturnsSubset(t *testing.T) {
	allTools := []tools.Tool{
		&namedTool{"Read"},
		&namedTool{"Write"},
		&namedTool{"Edit"},
		&namedTool{"Glob"},
		&namedTool{"Grep"},
		&namedTool{"Bash"},
		&namedTool{"Agent"},
		&namedTool{"AskUserQuestion"},
	}

	r := &PhaseRunner{Tools: allTools}

	// plan should include Read, Glob, Grep, Bash but not Write, Edit, Agent, AskUserQuestion
	planTools := r.filterToolsForPhase("plan")
	names := make(map[string]bool)
	for _, tool := range planTools {
		names[tool.Name()] = true
	}
	if names["Write"] || names["Edit"] || names["Agent"] || names["AskUserQuestion"] {
		t.Error("plan phase should not include Write, Edit, Agent, or AskUserQuestion")
	}
	if !names["Read"] || !names["Glob"] || !names["Grep"] || !names["Bash"] {
		t.Error("plan phase should include Read, Glob, Grep, Bash")
	}
}

func TestFilterToolsForPhase_UnknownPhaseReturnsAll(t *testing.T) {
	allTools := []tools.Tool{
		&namedTool{"Read"},
		&namedTool{"Write"},
	}
	r := &PhaseRunner{Tools: allTools}

	result := r.filterToolsForPhase("unknown-phase")
	if len(result) != len(allTools) {
		t.Errorf("unknown phase: got %d tools, want %d (all)", len(result), len(allTools))
	}
}

// namedTool is a minimal Tool implementation for filtering tests.
type namedTool struct{ n string }

func (t *namedTool) Name() string                 { return t.n }
func (t *namedTool) Description() string          { return "stub" }
func (t *namedTool) InputSchema() json.RawMessage { return nil }
func (t *namedTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "ok"}, nil
}
func (t *namedTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *namedTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *namedTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *namedTool) IsReadOnly(_ json.RawMessage) bool        { return true }
