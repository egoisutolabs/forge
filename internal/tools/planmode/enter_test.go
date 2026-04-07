package planmode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── interface compliance ─────────────────────────────────────────────────────

func TestEnterTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &EnterTool{}
}

func TestEnterTool_Name(t *testing.T) {
	if got := (&EnterTool{}).Name(); got != "EnterPlanMode" {
		t.Errorf("Name() = %q, want %q", got, "EnterPlanMode")
	}
}

func TestEnterTool_IsConcurrencySafe(t *testing.T) {
	if !(&EnterTool{}).IsConcurrencySafe(nil) {
		t.Error("EnterTool should be concurrency-safe")
	}
}

func TestEnterTool_IsReadOnly(t *testing.T) {
	if !(&EnterTool{}).IsReadOnly(nil) {
		t.Error("EnterTool should be read-only")
	}
}

// ─── CheckPermissions ─────────────────────────────────────────────────────────

func TestEnterTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	decision, err := (&EnterTool{}).CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("Behavior = %q, want %q", decision.Behavior, models.PermAllow)
	}
}

// ─── Execute ─────────────────────────────────────────────────────────────────

func TestEnterTool_Execute_SwitchesToPlanMode(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeDefault
	tctx := &tools.ToolContext{Permissions: pctx}

	_, err := (&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pctx.Mode != models.ModePlan {
		t.Errorf("Mode = %q, want %q", pctx.Mode, models.ModePlan)
	}
}

func TestEnterTool_Execute_StoresPreviousMode(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeAcceptEdits
	tctx := &tools.ToolContext{Permissions: pctx}

	(&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck
	if pctx.PrePlanMode != models.ModeAcceptEdits {
		t.Errorf("PrePlanMode = %q, want %q", pctx.PrePlanMode, models.ModeAcceptEdits)
	}
}

func TestEnterTool_Execute_NilPermissions(t *testing.T) {
	tctx := &tools.ToolContext{Permissions: nil}
	result, err := (&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
}

func TestEnterTool_Execute_NilToolContext(t *testing.T) {
	result, err := (&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
}

func TestEnterTool_Execute_ReturnsMessage(t *testing.T) {
	result, _ := (&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), nil)
	var out map[string]string
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if out["message"] == "" {
		t.Error("output 'message' field should not be empty")
	}
}

func TestEnterTool_Execute_PlanModeBlocksWrites(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	tctx := &tools.ToolContext{Permissions: pctx}

	(&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	decision := pctx.Check("FileWrite", false)
	if decision.Behavior != models.PermDeny {
		t.Errorf("write in plan mode should be denied, got %q", decision.Behavior)
	}
}

func TestEnterTool_Execute_PlanModeAllowsReads(t *testing.T) {
	pctx := permissions.NewDefaultContext("/tmp")
	tctx := &tools.ToolContext{Permissions: pctx}

	(&EnterTool{}).Execute(context.Background(), json.RawMessage(`{}`), tctx) //nolint:errcheck

	decision := pctx.Check("Read", true)
	if decision.Behavior != models.PermAllow {
		t.Errorf("read in plan mode should be allowed, got %q", decision.Behavior)
	}
}
