package models

// PermissionBehavior is the outcome of a permission check.
type PermissionBehavior string

const (
	PermAllow PermissionBehavior = "allow"
	PermDeny  PermissionBehavior = "deny"
	PermAsk   PermissionBehavior = "ask"
)

// PermissionDecision is the result of checking whether a tool is permitted.
type PermissionDecision struct {
	Behavior PermissionBehavior
	Message  string // reason for deny, or prompt for ask
}

// PermissionMode controls the overall permission policy.
type PermissionMode string

const (
	ModeDefault           PermissionMode = "default"
	ModeAcceptEdits       PermissionMode = "acceptEdits"
	ModeBypassPermissions PermissionMode = "bypassPermissions"
	ModePlan              PermissionMode = "plan"
)

// PermissionRule is a single permission rule entry.
type PermissionRule struct {
	ToolName string             // e.g., "Bash"
	Pattern  string             // e.g., "git *" for Bash(git *)
	Behavior PermissionBehavior // allow, deny, ask
	Source   string             // "user", "project", "session"
}
