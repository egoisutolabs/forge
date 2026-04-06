package e2e

import (
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/permissions"
)

// TestPermission_ReadOnlyToolGetsAllow verifies read-only tools get PermAllow
// in default mode without any rules.
func TestPermission_ReadOnlyToolGetsAllow(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	decision := ctx.Check("Read", true)

	if decision.Behavior != models.PermAllow {
		t.Errorf("read-only tool: got %q, want %q", decision.Behavior, models.PermAllow)
	}
}

// TestPermission_WriteToolGetsAsk verifies non-read-only tools get PermAsk
// in default mode.
func TestPermission_WriteToolGetsAsk(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	decision := ctx.Check("Edit", false)

	if decision.Behavior != models.PermAsk {
		t.Errorf("write tool: got %q, want %q", decision.Behavior, models.PermAsk)
	}
	if decision.Message == "" {
		t.Error("PermAsk should include a message")
	}
}

// TestPermission_PlanMode_ReadOnlyAllowed verifies read-only tools are allowed
// in plan mode.
func TestPermission_PlanMode_ReadOnlyAllowed(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.Mode = models.ModePlan

	decision := ctx.Check("Read", true)
	if decision.Behavior != models.PermAllow {
		t.Errorf("plan mode read-only: got %q, want %q", decision.Behavior, models.PermAllow)
	}
}

// TestPermission_PlanMode_WriteDenied verifies write tools are denied in plan mode
// with a clear message.
func TestPermission_PlanMode_WriteDenied(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.Mode = models.ModePlan

	decision := ctx.Check("Edit", false)
	if decision.Behavior != models.PermDeny {
		t.Errorf("plan mode write: got %q, want %q", decision.Behavior, models.PermDeny)
	}
	if decision.Message == "" {
		t.Error("plan mode deny should include a message")
	}
}

// TestPermission_BypassMode_AllowsEverything verifies BypassPermissions mode
// allows all tools regardless of read-only status.
func TestPermission_BypassMode_AllowsEverything(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.Mode = models.ModeBypassPermissions

	for _, tc := range []struct {
		name     string
		readOnly bool
	}{
		{"read-only tool", true},
		{"write tool", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := ctx.Check("AnyTool", tc.readOnly)
			if decision.Behavior != models.PermAllow {
				t.Errorf("bypass mode %s: got %q, want %q", tc.name, decision.Behavior, models.PermAllow)
			}
		})
	}
}

// TestPermission_AcceptEditsMode_AllowsWriteTools verifies AcceptEdits mode
// allows write tools without asking.
func TestPermission_AcceptEditsMode_AllowsWriteTools(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.Mode = models.ModeAcceptEdits

	decision := ctx.Check("Edit", false)
	if decision.Behavior != models.PermAllow {
		t.Errorf("acceptEdits mode write: got %q, want %q", decision.Behavior, models.PermAllow)
	}
}

// TestPermission_DenyRule verifies that deny rules take precedence.
func TestPermission_DenyRule(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.DenyRules = []models.PermissionRule{
		{ToolName: "Bash", Behavior: models.PermDeny, Source: "test"},
	}

	decision := ctx.Check("Bash", false)
	if decision.Behavior != models.PermDeny {
		t.Errorf("deny rule: got %q, want %q", decision.Behavior, models.PermDeny)
	}
}

// TestPermission_AllowRule verifies that allow rules grant access.
func TestPermission_AllowRule(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.AllowRules = []models.PermissionRule{
		{ToolName: "Bash", Behavior: models.PermAllow, Source: "test"},
	}

	decision := ctx.Check("Bash", false)
	if decision.Behavior != models.PermAllow {
		t.Errorf("allow rule: got %q, want %q", decision.Behavior, models.PermAllow)
	}
}

// TestPermission_DenyRuleTakesPrecedenceOverAllow verifies deny rules are
// checked before allow rules.
func TestPermission_DenyRuleTakesPrecedenceOverAllow(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.DenyRules = []models.PermissionRule{
		{ToolName: "Bash", Behavior: models.PermDeny, Source: "test"},
	}
	ctx.AllowRules = []models.PermissionRule{
		{ToolName: "Bash", Behavior: models.PermAllow, Source: "test"},
	}

	decision := ctx.Check("Bash", false)
	if decision.Behavior != models.PermDeny {
		t.Errorf("deny should beat allow: got %q, want %q", decision.Behavior, models.PermDeny)
	}
}

// TestPermission_AddSessionAllowRule verifies AddSessionAllowRule works.
func TestPermission_AddSessionAllowRule(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")

	// Before: write tool gets PermAsk.
	before := ctx.Check("Bash", false)
	if before.Behavior != models.PermAsk {
		t.Fatalf("before rule: got %q, want %q", before.Behavior, models.PermAsk)
	}

	// Add session allow rule.
	ctx.AddSessionAllowRule("Bash")

	// After: write tool gets PermAllow.
	after := ctx.Check("Bash", false)
	if after.Behavior != models.PermAllow {
		t.Errorf("after rule: got %q, want %q", after.Behavior, models.PermAllow)
	}
}

// TestPermission_PlanMode_PreservesPrePlanMode verifies the PrePlanMode field
// stores the previous mode correctly.
func TestPermission_PlanMode_PreservesPrePlanMode(t *testing.T) {
	ctx := permissions.NewDefaultContext("/tmp")
	ctx.Mode = models.ModeAcceptEdits

	// Simulate entering plan mode.
	ctx.PrePlanMode = ctx.Mode
	ctx.Mode = models.ModePlan

	if ctx.PrePlanMode != models.ModeAcceptEdits {
		t.Errorf("PrePlanMode = %q, want %q", ctx.PrePlanMode, models.ModeAcceptEdits)
	}

	// Simulate exiting plan mode.
	ctx.Mode = ctx.PrePlanMode
	if ctx.Mode != models.ModeAcceptEdits {
		t.Errorf("Mode after exit = %q, want %q", ctx.Mode, models.ModeAcceptEdits)
	}
}
