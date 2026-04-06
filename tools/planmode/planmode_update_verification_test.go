// Package planmode — supplemental verification tests after task #27 completed
// (ExitPlanMode: add teammate plan-approval workflow).
//
// This file documents gaps closed and remaining gaps after task #27.
//
// GAPS CLOSED (as of 2026-04-04):
//
//	GAP 2 (was: output missing isAgent, hasTaskTool, planWasEdited,
//	        awaitingLeaderApproval) → ALL FOUR NOW PRESENT in exitOutput.
//
//	GAP 3 (was: CheckPermissions always PermAllow) → CLOSED.
//	    Non-agents now receive PermAsk; sub-agents (AgentID set) receive PermAllow.
//
//	GAP 3b (was: allowedPrompts not echoed) → CLOSED.
//	    AllowedPrompts field now populated in exitOutput.
//
// GAPS STILL OPEN:
//
//	A. requestId is set only for agent callers, not for all callers.
//	   TypeScript generates requestId for both agent and interactive paths.
//
//	B. MISSING: Leader mailbox notification.
//	   TypeScript: writeToMailbox() sends a plan_approval_request message to
//	   the team lead's in-process mailbox. Go sets awaitingLeaderApproval=true
//	   and requestId but does not dispatch a mailbox message — so the leader
//	   never receives the notification and the approval workflow is a stub.
//
//	C. MISSING: setAwaitingPlanApproval() side-effect.
//	   TypeScript: updates in-process teammate state so teammates block further
//	   execution until the leader approves. Go: no equivalent state flag.
//
//	D. MISSING: hasTaskTool description mismatch.
//	   TypeScript outputSchema: hasTaskTool = "Whether the Agent tool is available".
//	   Go: checks for TaskCreate/TaskGet/etc. names (i.e. Task tools, not Agent tool).
package planmode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// ─── CLOSED GAP 2: output fields now present ─────────────────────────────────

// TestVerification_ExitPlanMode_AgentOutputFields verifies that the four new
// output fields (isAgent, hasTaskTool, planWasEdited, awaitingLeaderApproval)
// are now present in exitOutput for both agent and non-agent paths.
func TestVerification_ExitPlanMode_AgentOutputFields(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "# Plan\n\nStep 1."})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}

	// All four new fields must be present.
	required := []string{"isAgent", "hasTaskTool", "planWasEdited", "awaitingLeaderApproval"}
	for _, field := range required {
		if _, ok := out[field]; !ok {
			t.Errorf("REGRESSION: output missing field %q (task #27 should have added it)", field)
		}
	}
	t.Log("CLOSED GAP: isAgent, hasTaskTool, planWasEdited, awaitingLeaderApproval all in exitOutput")
}

// TestVerification_ExitPlanMode_NonAgent_AwaitingLeaderApprovalFalse verifies
// that non-agent callers get awaitingLeaderApproval=false.
func TestVerification_ExitPlanMode_NonAgent_AwaitingLeaderApprovalFalse(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "step 1"})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["awaitingLeaderApproval"] != false {
		t.Errorf("non-agent: awaitingLeaderApproval = %v, want false", out["awaitingLeaderApproval"])
	}
	if out["isAgent"] != false {
		t.Errorf("non-agent: isAgent = %v, want false", out["isAgent"])
	}
	t.Log("CORRECT: non-agent path sets awaitingLeaderApproval=false, isAgent=false")
}

// TestVerification_ExitPlanMode_Agent_AwaitingLeaderApprovalTrue verifies that
// sub-agents get awaitingLeaderApproval=true and a requestId.
func TestVerification_ExitPlanMode_Agent_AwaitingLeaderApprovalTrue(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "step 1"})
	agentTctx := &tools.ToolContext{AgentID: "agent-abc"}

	result, err := et.Execute(context.Background(), in, agentTctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["awaitingLeaderApproval"] != true {
		t.Errorf("agent: awaitingLeaderApproval = %v, want true", out["awaitingLeaderApproval"])
	}
	if out["isAgent"] != true {
		t.Errorf("agent: isAgent = %v, want true", out["isAgent"])
	}
	if requestID, _ := out["requestId"].(string); requestID == "" {
		t.Error("agent: requestId should be non-empty")
	}
	t.Log("CORRECT: agent path sets awaitingLeaderApproval=true, isAgent=true, requestId set")
}

// TestVerification_ExitPlanMode_PlanWasEdited_TrueWhenPlanProvided verifies
// planWasEdited=true when plan content is provided in input.
func TestVerification_ExitPlanMode_PlanWasEdited_TrueWhenPlanProvided(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "# My plan"})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["planWasEdited"] != true {
		t.Errorf("planWasEdited = %v, want true when plan content provided", out["planWasEdited"])
	}
	t.Log("CORRECT: planWasEdited=true when plan content non-empty")
}

// TestVerification_ExitPlanMode_PlanWasEdited_FalseWhenNoPlan verifies
// planWasEdited=false when no plan content is provided.
func TestVerification_ExitPlanMode_PlanWasEdited_FalseWhenNoPlan(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["planWasEdited"] != false {
		t.Errorf("planWasEdited = %v, want false when no plan content", out["planWasEdited"])
	}
	t.Log("CORRECT: planWasEdited=false when no plan content in input")
}

// TestVerification_ExitPlanMode_AllowedPrompts_EchoedInOutput verifies that
// allowed_prompts input is now echoed in output (gap 3b closed).
func TestVerification_ExitPlanMode_AllowedPrompts_EchoedInOutput(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{
		"plan": "step 1",
		"allowed_prompts": []any{
			map[string]any{"tool": "Bash", "prompt": "run tests"},
			map[string]any{"tool": "Bash", "prompt": "install deps"},
		},
	})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]json.RawMessage
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if _, ok := out["allowedPrompts"]; !ok {
		t.Error("REGRESSION: allowedPrompts should be present in output (task #27)")
	} else {
		t.Log("CLOSED GAP: allowedPrompts now echoed from input to output")
	}
}

// ─── CLOSED GAP 3: CheckPermissions ──────────────────────────────────────────

// TestVerification_CheckPermissions_PermAsk_ForNonAgents confirms that
// CheckPermissions now returns PermAsk for non-agent (interactive) callers.
func TestVerification_CheckPermissions_PermAsk_ForNonAgents(t *testing.T) {
	decision, err := (&ExitTool{}).CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("Behavior = %q, want PermAsk for non-agent callers", decision.Behavior)
	}
	t.Log("CLOSED GAP: CheckPermissions returns PermAsk for non-agents (matches TypeScript requiresUserInteraction)")
}

// ─── REMAINING GAP B: mailbox notification not implemented ───────────────────

// TestVerification_ExitPlanMode_LeaderMailbox_NotImplemented documents that
// Go's ExitPlanMode does not actually send a notification to the team leader.
//
// TypeScript (ExitPlanModeV2Tool.call):
//
//	const taskId = await findInProcessTeammateTaskId()
//	await writeToMailbox({type:'plan_approval_request', requestId, plan, taskId})
//
// Go: sets awaitingLeaderApproval=true and requestId, but never calls any
// mailbox API. The team leader will not receive the plan approval request.
func TestVerification_ExitPlanMode_LeaderMailbox_NotImplemented(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "step 1"})
	agentTctx := &tools.ToolContext{AgentID: "agent-abc"}

	result, err := et.Execute(context.Background(), in, agentTctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The output says awaitingLeaderApproval=true, but no actual notification
	// was dispatched. This is a stub implementation.
	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["awaitingLeaderApproval"] == true {
		t.Log("GAP CONFIRMED: awaitingLeaderApproval=true set in output, but leader mailbox notification not implemented")
		t.Log("TypeScript: writeToMailbox({type:'plan_approval_request', requestId, plan, taskId})")
		t.Log("Go: no writeToMailbox equivalent — leader never receives the notification")
	}
}

// ─── REMAINING GAP D: hasTaskTool description mismatch ───────────────────────

// TestVerification_ExitPlanMode_HasTaskTool_Description_Mismatch documents
// that Go's hasTaskTool checks for Task tools (TaskCreate etc.) while
// TypeScript checks whether the "Agent" tool is available.
//
// TypeScript outputSchema: hasTaskTool = "Whether the Agent tool is available"
// Go hasTaskToolInContext(): checks for TaskCreate, TaskGet, TaskList, etc.
func TestVerification_ExitPlanMode_HasTaskTool_Description_Mismatch(t *testing.T) {
	// Set up a context with only the Agent tool (no Task tools).
	agentTool := &stubExitTool{name: "Agent"}
	tctxWithAgent := &tools.ToolContext{Tools: []tools.Tool{agentTool}}

	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{})

	result, err := et.Execute(context.Background(), in, tctxWithAgent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	// TypeScript would set hasTaskTool=true (Agent tool present).
	// Go sets hasTaskTool=false (Agent is not a "task tool").
	if out["hasTaskTool"] != false {
		t.Logf("NOTE: hasTaskTool = %v with Agent-only tool set — task #27 may have changed behaviour", out["hasTaskTool"])
	} else {
		t.Log("GAP CONFIRMED: hasTaskTool=false even with Agent tool present")
		t.Log("TypeScript: hasTaskTool = 'Whether the Agent tool is available'")
		t.Log("Go: hasTaskToolInContext checks for TaskCreate/TaskGet/TaskList/etc., not Agent")
	}
}

// stubExitTool implements tools.Tool minimally for context setup.
type stubExitTool struct{ name string }

func (s *stubExitTool) Name() string                          { return s.name }
func (s *stubExitTool) Description() string                   { return "" }
func (s *stubExitTool) InputSchema() json.RawMessage          { return json.RawMessage(`{}`) }
func (s *stubExitTool) ValidateInput(_ json.RawMessage) error { return nil }
func (s *stubExitTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *stubExitTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: ""}, nil
}
func (s *stubExitTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *stubExitTool) IsReadOnly(_ json.RawMessage) bool        { return true }
