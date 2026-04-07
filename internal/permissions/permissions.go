package permissions

import "github.com/egoisutolabs/forge/internal/models"

// Context holds the active permission configuration.
type Context struct {
	Mode       models.PermissionMode
	Cwd        string
	AllowRules []models.PermissionRule
	DenyRules  []models.PermissionRule

	// PrePlanMode stores the mode that was active before entering plan mode.
	// Restored by ExitPlanModeTool when leaving plan mode.
	PrePlanMode models.PermissionMode
}

// NewDefaultContext creates a permission context with default mode.
func NewDefaultContext(cwd string) *Context {
	return &Context{
		Mode: models.ModeDefault,
		Cwd:  cwd,
	}
}

// Check evaluates permission rules for a given tool and returns a decision.
func (c *Context) Check(toolName string, isReadOnly bool) *models.PermissionDecision {
	if c.Mode == models.ModeBypassPermissions {
		return &models.PermissionDecision{Behavior: models.PermAllow}
	}

	if c.Mode == models.ModePlan {
		if isReadOnly {
			return &models.PermissionDecision{Behavior: models.PermAllow}
		}
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "write operations not allowed in plan mode"}
	}

	for _, rule := range c.DenyRules {
		if rule.ToolName == toolName {
			return &models.PermissionDecision{Behavior: models.PermDeny, Message: "denied by rule: " + rule.Source}
		}
	}

	for _, rule := range c.AllowRules {
		if rule.ToolName == toolName {
			return &models.PermissionDecision{Behavior: models.PermAllow}
		}
	}

	if isReadOnly {
		return &models.PermissionDecision{Behavior: models.PermAllow}
	}

	if c.Mode == models.ModeAcceptEdits {
		return &models.PermissionDecision{Behavior: models.PermAllow}
	}

	return &models.PermissionDecision{Behavior: models.PermAsk, Message: "allow " + toolName + "?"}
}

// AddSessionAllowRule adds a rule that allows a tool for the rest of the session.
func (c *Context) AddSessionAllowRule(toolName string) {
	c.AllowRules = append(c.AllowRules, models.PermissionRule{
		ToolName: toolName,
		Behavior: models.PermAllow,
		Source:   "session",
	})
}
