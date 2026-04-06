package planmode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/permissions"
	"github.com/egoisutolabs/forge/tools"
)

// ─── interface compliance ─────────────────────────────────────────────────────

func TestExitTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &ExitTool{}
}

func TestExitTool_Name(t *testing.T) {
	if got := (&ExitTool{}).Name(); got != "ExitPlanMode" {
		t.Errorf("Name() = %q, want %q", got, "ExitPlanMode")
	}
}

func TestExitTool_IsConcurrencySafe(t *testing.T) {
	if !(&ExitTool{}).IsConcurrencySafe(nil) {
		t.Error("ExitTool should be concurrency-safe")
	}
}

func TestExitTool_IsReadOnly(t *testing.T) {
	if (&ExitTool{}).IsReadOnly(nil) {
		t.Error("ExitTool should NOT be read-only (writes plan to disk)")
	}
}

// ─── CheckPermissions ─────────────────────────────────────────────────────────

func TestExitTool_CheckPermissions_PermAskForInteractiveCaller(t *testing.T) {
	// nil tctx → non-agent (interactive) caller → PermAsk
	decision, err := (&ExitTool{}).CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("Behavior = %q, want PermAsk (interactive callers must confirm)", decision.Behavior)
	}
}

func TestExitTool_CheckPermissions_PermAllowForAgent(t *testing.T) {
	// tctx with AgentID → sub-agent caller → PermAllow (bypass prompt)
	tctx := &tools.ToolContext{AgentID: "agent-123"}
	decision, err := (&ExitTool{}).CheckPermissions(json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("Behavior = %q, want PermAllow (sub-agents bypass prompt)", decision.Behavior)
	}
}

// ─── Execute — mode restoration ───────────────────────────────────────────────

func TestExitTool_Execute_RestoresPreviousMode(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModePlan
	pctx.PrePlanMode = models.ModeAcceptEdits
	tctx := &tools.ToolContext{Permissions: pctx}

	et := &ExitTool{PlansDir: t.TempDir()}
	et.Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	if pctx.Mode != models.ModeAcceptEdits {
		t.Errorf("Mode = %q, want %q", pctx.Mode, models.ModeAcceptEdits)
	}
	if pctx.PrePlanMode != "" {
		t.Errorf("PrePlanMode should be cleared, got %q", pctx.PrePlanMode)
	}
}

func TestExitTool_Execute_RestoresDefaultWhenPrePlanModeEmpty(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModePlan
	pctx.PrePlanMode = "" // not set
	tctx := &tools.ToolContext{Permissions: pctx}

	et := &ExitTool{PlansDir: t.TempDir()}
	et.Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	if pctx.Mode != models.ModeDefault {
		t.Errorf("Mode = %q, want %q (default fallback)", pctx.Mode, models.ModeDefault)
	}
}

func TestExitTool_Execute_DoesNotChangeMode_WhenNotInPlanMode(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeAcceptEdits // not in plan mode
	tctx := &tools.ToolContext{Permissions: pctx}

	et := &ExitTool{PlansDir: t.TempDir()}
	et.Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	if pctx.Mode != models.ModeAcceptEdits {
		t.Errorf("Mode should be unchanged, got %q", pctx.Mode)
	}
}

// ─── Execute — plan writing ───────────────────────────────────────────────────

func TestExitTool_Execute_WritesPlanToDisk(t *testing.T) {
	dir := t.TempDir()
	et := &ExitTool{PlansDir: dir}

	in := mustExitJSON(t, map[string]any{"plan": "# My Plan\n\nStep 1."})
	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck
	fp, _ := out["filePath"].(string)
	if fp == "" {
		t.Fatal("filePath should be non-empty when plan content is provided")
	}

	content, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("could not read plan file %s: %v", fp, err)
	}
	if string(content) != "# My Plan\n\nStep 1." {
		t.Errorf("plan file content = %q, want %q", string(content), "# My Plan\n\nStep 1.")
	}
}

func TestExitTool_Execute_NoPlanNoDiskWrite(t *testing.T) {
	dir := t.TempDir()
	et := &ExitTool{PlansDir: dir}

	result, err := et.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck
	if out["filePath"] != nil && out["filePath"] != "" {
		t.Errorf("filePath should be empty when no plan provided, got %v", out["filePath"])
	}

	// Directory should exist but be empty.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("expected no files written, found %d", len(entries))
	}
}

func TestExitTool_Execute_CreatesPlansDirIfMissing(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "deep", "plans")
	et := &ExitTool{PlansDir: dir}

	in := mustExitJSON(t, map[string]any{"plan": "content"})
	result, _ := et.Execute(context.Background(), in, nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("plans directory was not created")
	}
}

func TestExitTool_Execute_UseCwdForDefaultPlansDir(t *testing.T) {
	cwd := t.TempDir()
	tctx := &tools.ToolContext{Cwd: cwd}

	et := &ExitTool{} // no PlansDir override
	in := mustExitJSON(t, map[string]any{"plan": "content"})
	result, _ := et.Execute(context.Background(), in, tctx)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	expected := filepath.Join(cwd, ".forge", "plans")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected plans dir at %s, not found", expected)
	}
}

func TestExitTool_Execute_OutputContainsPlan(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "my plan content"})

	result, _ := et.Execute(context.Background(), in, nil)
	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if out["plan"] != "my plan content" {
		t.Errorf("output plan = %v, want %q", out["plan"], "my plan content")
	}
}

// ─── Execute — output fields ──────────────────────────────────────────────────

func TestExitTool_Execute_OutputFields_NonAgent(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "step 1"})

	result, err := et.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	// Non-agent: awaitingLeaderApproval must be false, no requestId.
	if awaiting, _ := out["awaitingLeaderApproval"].(bool); awaiting {
		t.Error("awaitingLeaderApproval should be false for non-agent callers")
	}
	if rid, _ := out["requestId"].(string); rid != "" {
		t.Errorf("requestId should be empty for non-agent callers, got %q", rid)
	}
	if isAgent, _ := out["isAgent"].(bool); isAgent {
		t.Error("isAgent should be false when tctx is nil")
	}
	// planWasEdited must be true because plan content was provided.
	if edited, _ := out["planWasEdited"].(bool); !edited {
		t.Error("planWasEdited should be true when plan content was provided")
	}
}

func TestExitTool_Execute_AgentPath_AwaitingApproval(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModePlan
	pctx.PrePlanMode = models.ModeDefault
	tctx := &tools.ToolContext{AgentID: "agent-abc", Permissions: pctx}

	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "# Agent Plan"})

	result, err := et.Execute(context.Background(), in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if awaiting, _ := out["awaitingLeaderApproval"].(bool); !awaiting {
		t.Error("awaitingLeaderApproval should be true for sub-agent callers")
	}
	if rid, _ := out["requestId"].(string); rid == "" {
		t.Error("requestId should be non-empty for sub-agent callers")
	}
	if isAgent, _ := out["isAgent"].(bool); !isAgent {
		t.Error("isAgent should be true when AgentID is set")
	}

	// Permissions must NOT be restored — agent waits for leader approval.
	if pctx.Mode != models.ModePlan {
		t.Errorf("Mode should still be ModePlan (awaiting approval), got %q", pctx.Mode)
	}
	if pctx.PrePlanMode != models.ModeDefault {
		t.Errorf("PrePlanMode should still be set while awaiting approval, got %q", pctx.PrePlanMode)
	}
}

func TestExitTool_Execute_AgentPath_RequestIDIsUUID(t *testing.T) {
	tctx := &tools.ToolContext{AgentID: "agent-xyz"}
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{"plan": "plan"})

	result, _ := et.Execute(context.Background(), in, tctx)

	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	rid, _ := out["requestId"].(string)
	if len(rid) == 0 {
		t.Fatal("requestId should not be empty")
	}
	// UUID format: 8-4-4-4-12 hex chars = 36 chars with dashes.
	if len(rid) != 36 {
		t.Errorf("requestId = %q, expected UUID format (36 chars), got %d chars", rid, len(rid))
	}
}

func TestExitTool_Execute_HasTaskTool_False_WhenEmpty(t *testing.T) {
	tctx := &tools.ToolContext{AgentID: "a"}
	if hasTaskToolInContext(tctx) {
		t.Error("hasTaskToolInContext should be false when Tools slice is empty")
	}
}

func TestExitTool_Execute_AllowedPromptsEchoedInOutput(t *testing.T) {
	et := &ExitTool{PlansDir: t.TempDir()}
	in := mustExitJSON(t, map[string]any{
		"plan": "step 1",
		"allowed_prompts": []any{
			map[string]any{"tool": "Bash", "prompt": "run tests"},
		},
	})

	result, _ := et.Execute(context.Background(), in, nil)
	var out map[string]json.RawMessage
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck

	if _, ok := out["allowedPrompts"]; !ok {
		t.Error("allowedPrompts should be present in output when provided in input")
	}
}

// ─── Execute — round trip (enter → exit) ─────────────────────────────────────

func TestPlanModeRoundTrip(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeDefault
	tctx := &tools.ToolContext{Permissions: pctx}

	// Enter plan mode.
	(&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck
	if pctx.Mode != models.ModePlan {
		t.Fatalf("expected plan mode after Enter, got %q", pctx.Mode)
	}
	if pctx.PrePlanMode != models.ModeDefault {
		t.Fatalf("expected PrePlanMode=default, got %q", pctx.PrePlanMode)
	}

	// While in plan mode, writes should be denied.
	if pctx.Check("FileWrite", false).Behavior != models.PermDeny {
		t.Error("writes should be denied in plan mode")
	}

	// Exit plan mode.
	et := &ExitTool{PlansDir: t.TempDir()}
	et.Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck
	if pctx.Mode != models.ModeDefault {
		t.Errorf("expected default mode after Exit, got %q", pctx.Mode)
	}

	// After exit, writes should be asked (default mode).
	if pctx.Check("FileWrite", false).Behavior != models.PermAsk {
		t.Errorf("expected PermAsk after exiting plan mode, got %q", pctx.Check("FileWrite", false).Behavior)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustExitJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}
