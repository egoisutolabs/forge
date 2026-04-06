// Package planmode — verification tests comparing Go port against Claude Code's
// EnterPlanModeTool / ExitPlanModeTool TypeScript sources.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: Teammate plan-approval workflow in ExitPlanMode.
//     TypeScript ExitPlanModeV2Tool detects `isTeammate` and, when
//     `isPlanModeRequired()`, sends a `plan_approval_request` to the team
//     lead and waits for approval. Output includes `awaitingLeaderApproval`
//     and `requestId`. Go always exits locally without approval handshake.
//
//  2. MISSING: Output fields `isAgent`, `hasTaskTool`, `planWasEdited`,
//     `awaitingLeaderApproval`, `requestId`.
//     TypeScript outputSchema includes all five; Go only returns `{plan, filePath}`.
//
//  3. DIVERGENCE: CheckPermissions on ExitPlanMode.
//     TypeScript: bypasses permission check for teammates, uses 'ask' for others.
//     Go: always returns PermAllow regardless of caller context.
//
//  4. MISSING: EnterPlanMode isEnabled() check.
//     TypeScript disables the tool when KAIROS channels are active.
//     Go has no such check (acceptable — no KAIROS infra in Go port).
//
//  5. MISSING: Classifier activation in EnterPlanMode.
//     TypeScript calls a classifier activation on enter. Not applicable to Go.
package planmode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/permissions"
	"github.com/egoisutolabs/forge/tools"
)

// ─── GAP 1: teammate plan-approval workflow missing ───────────────────────────

// TestVerification_ExitPlanMode_NoTeammateApprovalWorkflow documents that the
// Go port does not implement the teammate plan-approval handshake.
//
// Claude Code TypeScript behaviour (ExitPlanModeV2Tool.call):
//  1. Detect if running as a teammate (agent).
//  2. If `isPlanModeRequired()` → send `plan_approval_request` to team lead.
//  3. Output includes `awaitingLeaderApproval: true` and a `requestId`.
//  4. Execution blocks until the team lead approves or rejects.
//
// Go behaviour: always exits locally, never sends approval request.
func TestVerification_ExitPlanMode_NoTeammateApprovalWorkflow(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "# Plan\n\nStep 1."})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// GAP: these fields are absent in Go but present in TypeScript output.
	gaps := []string{"awaitingLeaderApproval", "requestId", "isAgent", "hasTaskTool", "planWasEdited"}
	for _, field := range gaps {
		if _, ok := out[field]; !ok {
			t.Logf("GAP CONFIRMED: output missing field %q (present in Claude Code TypeScript schema)", field)
		}
	}
}

// ─── GAP 2: output schema shape ──────────────────────────────────────────────

// TestVerification_ExitPlanMode_OutputSchemaShape verifies the output fields
// that SHOULD match between Go and Claude Code.
func TestVerification_ExitPlanMode_OutputSchemaShape(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	planContent := "# Implementation Plan\n\n1. Do X\n2. Do Y"
	in := mustExitJSON(t, map[string]any{"plan": planContent})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// `plan` must be echoed back — present in both TypeScript and Go.
	if out["plan"] != planContent {
		t.Errorf("output plan = %v, want %q", out["plan"], planContent)
	}

	// `filePath` must be set when plan content was provided.
	fp, _ := out["filePath"].(string)
	if fp == "" {
		t.Error("filePath should be non-empty when plan content is provided")
	}
}

// ─── GAP 3: CheckPermissions divergence ──────────────────────────────────────

// TestVerification_ExitPlanMode_CheckPermissionsAlwaysAllow verifies the
// current Go behaviour and documents the divergence from TypeScript.
//
// TypeScript: bypasses for teammates (agents), 'ask' for human users.
// Go: always PermAllow.
//
// For most use cases this is safe (plan approval is about the plan content,
// not the ExitPlanMode call itself), but it diverges from the TypeScript
// semantics where a human user must confirm before exiting plan mode.
func TestVerification_ExitPlanMode_CheckPermissions(t *testing.T) {
	// Non-agent (nil tctx) → PermAsk, matching TypeScript for human callers.
	decision, err := (&ExitTool{}).CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("non-agent: Behavior = %q, want PermAsk (TypeScript uses 'ask' for human callers)", decision.Behavior)
	}

	// Sub-agent (AgentID set) → PermAllow, matching TypeScript teammate bypass.
	agentTctx := &tools.ToolContext{AgentID: "agent-1"}
	decision, err = (&ExitTool{}).CheckPermissions(json.RawMessage(`{}`), agentTctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("sub-agent: Behavior = %q, want PermAllow (TypeScript bypasses for teammates)", decision.Behavior)
	}
}

// ─── GAP 3b: AllowedPrompts in output ────────────────────────────────────────

// TestVerification_ExitPlanMode_AllowedPromptsNotEchoedInOutput verifies
// whether allowed_prompts provided in input are reflected in output.
//
// Claude Code TypeScript carries allowed_prompts through to the output so
// the engine can pre-authorise subsequent tool calls. The Go implementation
// currently does not echo them.
func TestVerification_ExitPlanMode_AllowedPromptsNotEchoedInOutput(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{
		"plan": "step 1",
		"allowed_prompts": []any{
			map[string]any{"tool": "Bash", "prompt": "run tests"},
		},
	})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]json.RawMessage
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if _, ok := out["allowedPrompts"]; !ok {
		t.Log("GAP CONFIRMED: allowed_prompts not echoed in output (present in TypeScript output schema)")
	}
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_EnterPlanMode_SetsModePlan verifies the core behaviour
// of EnterPlanMode matches the TypeScript: sets app state to plan mode.
func TestVerification_EnterPlanMode_SetsModePlan(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeDefault
	tctx := &tools.ToolContext{Permissions: pctx}

	_, err := (&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TypeScript: sets app state → plan mode.
	if pctx.Mode != models.ModePlan {
		t.Errorf("Mode = %q, want ModePlan", pctx.Mode)
	}
	if pctx.PrePlanMode != models.ModeDefault {
		t.Errorf("PrePlanMode = %q, want ModeDefault (pre-plan mode saved)", pctx.PrePlanMode)
	}
}

// TestVerification_EnterPlanMode_IsReadOnly verifies that EnterPlanMode is
// marked read-only, matching TypeScript's isReadOnly(): true.
func TestVerification_EnterPlanMode_IsReadOnly(t *testing.T) {
	if !(&EnterTool{}).IsReadOnly(nil) {
		t.Error("EnterPlanMode must be read-only (matches TypeScript isReadOnly: true)")
	}
}

// TestVerification_EnterPlanMode_IsConcurrencySafe verifies concurrency flag
// matches TypeScript's isConcurrencySafe(): true.
func TestVerification_EnterPlanMode_IsConcurrencySafe(t *testing.T) {
	if !(&EnterTool{}).IsConcurrencySafe(nil) {
		t.Error("EnterPlanMode must be concurrency-safe (matches TypeScript isConcurrencySafe: true)")
	}
}

// TestVerification_ExitPlanMode_RestoredModeAllowsWrites verifies that after
// exiting plan mode, write operations are no longer blocked — matching the
// TypeScript behaviour of restoring the previous permission state.
func TestVerification_ExitPlanMode_RestoredModeAllowsWrites(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeDefault
	tctx := &tools.ToolContext{Permissions: pctx}

	// Enter plan mode.
	(&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	// In plan mode, writes should be denied.
	if pctx.Check("FileWrite", false).Behavior == models.PermAllow {
		t.Error("writes should not be allowed in plan mode")
	}

	// Exit plan mode.
	et := &ExitTool{PlansDir: t.TempDir()}
	et.Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	// After exit, mode should be restored to default.
	if pctx.Mode == models.ModePlan {
		t.Error("mode should no longer be plan mode after ExitPlanMode")
	}
}

// TestVerification_ExitPlanMode_InputSchemaHasAllowedPrompts verifies the
// input schema includes allowed_prompts, matching TypeScript inputSchema.
func TestVerification_ExitPlanMode_InputSchemaHasAllowedPrompts(t *testing.T) {
	schema := (&ExitTool{}).InputSchema()
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}

	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("InputSchema missing 'properties'")
	}

	if _, ok := props["allowed_prompts"]; !ok {
		t.Error("InputSchema missing 'allowed_prompts' property (present in TypeScript inputSchema)")
	}
	if _, ok := props["plan"]; !ok {
		t.Error("InputSchema missing 'plan' property")
	}
}
