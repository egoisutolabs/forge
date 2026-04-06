package planmode

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// allowedPrompt is one semantic permission request carried in the exit input.
type allowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

// exitInput is the JSON schema for ExitPlanModeTool input.
type exitInput struct {
	Plan           string          `json:"plan,omitempty"`
	AllowedPrompts []allowedPrompt `json:"allowed_prompts,omitempty"`
}

// exitOutput is the JSON output of ExitPlanModeTool.
type exitOutput struct {
	Plan                   string          `json:"plan"`
	FilePath               string          `json:"filePath,omitempty"`
	IsAgent                bool            `json:"isAgent"`
	HasTaskTool            bool            `json:"hasTaskTool"`
	PlanWasEdited          bool            `json:"planWasEdited"`
	AwaitingLeaderApproval bool            `json:"awaitingLeaderApproval"`
	RequestID              string          `json:"requestId,omitempty"`
	AllowedPrompts         []allowedPrompt `json:"allowedPrompts,omitempty"`
}

// ExitTool implements ExitPlanModeTool — leave plan mode and persist the plan.
//
// When running as a sub-agent (tctx.AgentID is set), ExitTool does NOT restore
// permissions immediately. Instead it returns awaitingLeaderApproval=true with
// a requestId, allowing the team lead to approve or reject the plan first.
//
// For non-agent callers, plan mode is restored immediately (previous behavior).
//
// Permission: PermAllow for sub-agents, PermAsk for interactive callers.
// Concurrency: safe
type ExitTool struct {
	// PlansDir optionally overrides the directory where plans are written.
	// Defaults to "<cwd>/.forge/plans".
	PlansDir string
}

func (t *ExitTool) Name() string { return "ExitPlanMode" }
func (t *ExitTool) Description() string {
	return "Exit plan mode and present your implementation plan for approval."
}

func (t *ExitTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"plan": {
				"type": "string",
				"description": "The implementation plan content to save to disk"
			},
			"allowed_prompts": {
				"type": "array",
				"description": "Prompt-based permissions needed to implement the plan",
				"items": {
					"type": "object",
					"properties": {
						"tool":   {"type": "string", "description": "Tool this prompt applies to"},
						"prompt": {"type": "string", "description": "Semantic description of the action"}
					},
					"required": ["tool", "prompt"]
				}
			}
		}
	}`)
}

func (t *ExitTool) ValidateInput(_ json.RawMessage) error { return nil }

// CheckPermissions returns PermAllow for sub-agents (teammates) and PermAsk
// for interactive callers, matching TypeScript's ExitPlanModeV2Tool behavior.
func (t *ExitTool) CheckPermissions(_ json.RawMessage, tctx *tools.ToolContext) (*models.PermissionDecision, error) {
	if tctx != nil && tctx.AgentID != "" {
		// Sub-agents bypass the permission prompt — they have their own approval flow.
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  "Allow agent to exit plan mode and present its implementation plan?",
	}, nil
}

func (t *ExitTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *ExitTool) IsReadOnly(_ json.RawMessage) bool        { return false }

func (t *ExitTool) Execute(_ context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in exitInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	isAgent := tctx != nil && tctx.AgentID != ""
	hasTaskTool := tctx != nil && hasTaskToolInContext(tctx)
	planWasEdited := in.Plan != ""

	// Write plan to disk if content was provided.
	var filePath string
	if in.Plan != "" {
		dir := t.plansDir(tctx)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &models.ToolResult{
				Content: fmt.Sprintf("failed to create plans directory: %s", err),
				IsError: true,
			}, nil
		}
		ts := time.Now().UTC().Format("20060102-150405")
		filePath = filepath.Join(dir, ts+".md")
		if err := os.WriteFile(filePath, []byte(in.Plan), 0644); err != nil {
			return &models.ToolResult{
				Content: fmt.Sprintf("failed to write plan: %s", err),
				IsError: true,
			}, nil
		}
	}

	out := exitOutput{
		Plan:           in.Plan,
		FilePath:       filePath,
		IsAgent:        isAgent,
		HasTaskTool:    hasTaskTool,
		PlanWasEdited:  planWasEdited,
		AllowedPrompts: in.AllowedPrompts,
	}

	if isAgent {
		// Sub-agent path: request leader approval instead of restoring permissions.
		out.AwaitingLeaderApproval = true
		out.RequestID = generateRequestID()
		// Do NOT restore tctx.Permissions — stay in plan mode until leader approves.
	} else {
		// Interactive path: restore the pre-plan permission mode immediately.
		out.AwaitingLeaderApproval = false
		if tctx != nil && tctx.Permissions != nil && tctx.Permissions.Mode == models.ModePlan {
			prev := tctx.Permissions.PrePlanMode
			if prev == "" {
				prev = models.ModeDefault
			}
			tctx.Permissions.Mode = prev
			tctx.Permissions.PrePlanMode = ""
		}
	}

	data, _ := json.Marshal(out)
	return &models.ToolResult{Content: string(data)}, nil
}

// plansDir returns the directory where plan files are stored.
func (t *ExitTool) plansDir(tctx *tools.ToolContext) string {
	if t.PlansDir != "" {
		return t.PlansDir
	}
	base := "."
	if tctx != nil && tctx.Cwd != "" {
		base = tctx.Cwd
	}
	return filepath.Join(base, ".forge", "plans")
}

// hasTaskToolInContext reports whether any task tool is registered in tctx.
func hasTaskToolInContext(tctx *tools.ToolContext) bool {
	for _, tool := range tctx.Tools {
		switch tool.Name() {
		case "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TaskStop", "TaskOutput":
			return true
		}
	}
	return false
}

// generateRequestID returns a random UUID v4 string.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	// Set version 4 and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
