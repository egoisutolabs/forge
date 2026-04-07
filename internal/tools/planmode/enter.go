// Package planmode implements EnterPlanModeTool and ExitPlanModeTool — Go ports
// of Claude Code's EnterPlanModeTool and ExitPlanModeV2Tool.
package planmode

import (
	"context"
	"encoding/json"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// EnterTool implements EnterPlanModeTool — switch to plan mode.
//
// Switches the permission context mode to "plan", storing the previous mode in
// PrePlanMode so ExitPlanModeTool can restore it. In plan mode only read-only
// tools are permitted; write operations are denied by the permission system.
//
// Permission: PermAllow (always allowed)
// Concurrency: safe
type EnterTool struct{}

func (t *EnterTool) Name() string { return "EnterPlanMode" }
func (t *EnterTool) Description() string {
	return "Switch to plan mode for read-only codebase exploration and implementation planning."
}

func (t *EnterTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *EnterTool) ValidateInput(_ json.RawMessage) error { return nil }

func (t *EnterTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *EnterTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *EnterTool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *EnterTool) Execute(_ context.Context, _ json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	if tctx != nil && tctx.Permissions != nil {
		tctx.Permissions.PrePlanMode = tctx.Permissions.Mode
		tctx.Permissions.Mode = models.ModePlan
	}

	out := map[string]string{
		"message": "Entered plan mode. You should now focus on exploring the codebase and designing an implementation approach.\n\nIn plan mode, you should:\n1. Thoroughly explore the codebase to understand existing patterns\n2. Identify similar features and architectural approaches\n3. Consider multiple approaches and their trade-offs\n4. Use AskUserQuestion if you need to clarify the approach\n5. Design a concrete implementation strategy\n6. When ready, use ExitPlanMode to present your plan for approval\n\nRemember: DO NOT write or edit any files yet. This is a read-only exploration and planning phase.",
	}
	data, _ := json.Marshal(out)
	return &models.ToolResult{Content: string(data)}, nil
}
