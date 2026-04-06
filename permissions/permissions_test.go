package permissions

import (
	"testing"

	"github.com/egoisutolabs/forge/models"
)

func TestCheck_DefaultMode_ReadOnly(t *testing.T) {
	ctx := NewDefaultContext("/tmp")
	d := ctx.Check("Grep", true)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow for read-only tool, got %v", d.Behavior)
	}
}

func TestCheck_DefaultMode_Write(t *testing.T) {
	ctx := NewDefaultContext("/tmp")
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermAsk {
		t.Errorf("expected Ask for write tool in default mode, got %v", d.Behavior)
	}
}

func TestCheck_BypassMode(t *testing.T) {
	ctx := &Context{Mode: models.ModeBypassPermissions}
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow in bypass mode, got %v", d.Behavior)
	}
}

func TestCheck_PlanMode_ReadOnly(t *testing.T) {
	ctx := &Context{Mode: models.ModePlan}
	d := ctx.Check("Read", true)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow for read-only in plan mode, got %v", d.Behavior)
	}
}

func TestCheck_PlanMode_Write(t *testing.T) {
	ctx := &Context{Mode: models.ModePlan}
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermDeny {
		t.Errorf("expected Deny for write in plan mode, got %v", d.Behavior)
	}
}

func TestCheck_DenyRule(t *testing.T) {
	ctx := &Context{
		Mode: models.ModeDefault,
		DenyRules: []models.PermissionRule{
			{ToolName: "Bash", Behavior: models.PermDeny, Source: "project"},
		},
	}
	d := ctx.Check("Bash", true)
	if d.Behavior != models.PermDeny {
		t.Errorf("expected Deny from deny rule, got %v", d.Behavior)
	}
}

func TestCheck_AllowRule(t *testing.T) {
	ctx := &Context{
		Mode: models.ModeDefault,
		AllowRules: []models.PermissionRule{
			{ToolName: "Edit", Behavior: models.PermAllow, Source: "session"},
		},
	}
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow from allow rule, got %v", d.Behavior)
	}
}

func TestCheck_DenyTakesPrecedence(t *testing.T) {
	ctx := &Context{
		Mode: models.ModeDefault,
		AllowRules: []models.PermissionRule{
			{ToolName: "Bash", Behavior: models.PermAllow, Source: "session"},
		},
		DenyRules: []models.PermissionRule{
			{ToolName: "Bash", Behavior: models.PermDeny, Source: "project"},
		},
	}
	d := ctx.Check("Bash", true)
	if d.Behavior != models.PermDeny {
		t.Errorf("expected deny to take precedence, got %v", d.Behavior)
	}
}

func TestCheck_AcceptEditsMode(t *testing.T) {
	ctx := &Context{Mode: models.ModeAcceptEdits}
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow for write in acceptEdits mode, got %v", d.Behavior)
	}
}

func TestAddSessionAllowRule(t *testing.T) {
	ctx := NewDefaultContext("/tmp")
	d := ctx.Check("Edit", false)
	if d.Behavior != models.PermAsk {
		t.Fatalf("expected Ask before rule, got %v", d.Behavior)
	}

	ctx.AddSessionAllowRule("Edit")

	d = ctx.Check("Edit", false)
	if d.Behavior != models.PermAllow {
		t.Errorf("expected Allow after session rule, got %v", d.Behavior)
	}
}
