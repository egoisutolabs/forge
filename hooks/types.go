package hooks

import "encoding/json"

// HookEvent identifies when a hook fires.
type HookEvent string

const (
	HookEventPreToolUse         HookEvent = "PreToolUse"
	HookEventPostToolUse        HookEvent = "PostToolUse"
	HookEventSessionStart       HookEvent = "SessionStart"
	HookEventStop               HookEvent = "Stop"
	HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookEventSubagentStart      HookEvent = "SubagentStart"
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookEventPermissionRequest  HookEvent = "PermissionRequest"
	HookEventTeammateIdle       HookEvent = "TeammateIdle"
	HookEventTaskCreated        HookEvent = "TaskCreated"
	HookEventTaskCompleted      HookEvent = "TaskCompleted"
	HookEventSessionEnd         HookEvent = "SessionEnd"
	HookEventSetup              HookEvent = "Setup"
	HookEventPreCompact         HookEvent = "PreCompact"
	HookEventPostCompact        HookEvent = "PostCompact"
	HookEventNotification       HookEvent = "Notification"
	HookEventStopFailure        HookEvent = "StopFailure"
	HookEventPermissionDenied   HookEvent = "PermissionDenied"
	HookEventElicitation        HookEvent = "Elicitation"
	HookEventWorktreeCreate     HookEvent = "WorktreeCreate"
	HookEventWorktreeRemove     HookEvent = "WorktreeRemove"
)

// HookInput is the JSON payload sent to a hook process via stdin.
type HookInput struct {
	EventName  HookEvent       `json:"event_name"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	ToolOutput json.RawMessage `json:"tool_output,omitempty"`
}

// HookResult is the JSON payload read from a hook process's stdout.
// Continue defaults to true when not explicitly set in the hook output.
type HookResult struct {
	Continue      bool            `json:"continue"`
	Decision      string          `json:"decision,omitempty"` // "allow", "deny", or "ask"
	UpdatedInput  json.RawMessage `json:"updated_input,omitempty"`
	SystemMessage string          `json:"system_message,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	// Async, when true, signals that this hook's decision is advisory only —
	// ExecuteHooks treats it as Continue=true regardless of other fields.
	// Mirrors TypeScript's background-hook semantics.
	Async bool `json:"async,omitempty"`
}

// hookResultWire is used for custom JSON unmarshaling with proper defaults.
type hookResultWire struct {
	Continue      *bool           `json:"continue"`
	Decision      string          `json:"decision,omitempty"`
	UpdatedInput  json.RawMessage `json:"updated_input,omitempty"`
	SystemMessage string          `json:"system_message,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	Async         bool            `json:"async,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler so that Continue defaults to true
// when the field is absent from the hook's output JSON.
func (r *HookResult) UnmarshalJSON(data []byte) error {
	var wire hookResultWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Continue == nil {
		r.Continue = true
	} else {
		r.Continue = *wire.Continue
	}
	r.Decision = wire.Decision
	r.UpdatedInput = wire.UpdatedInput
	r.SystemMessage = wire.SystemMessage
	r.Reason = wire.Reason
	r.Async = wire.Async
	return nil
}

// HookConfig is the configuration for a single hook command.
type HookConfig struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // seconds; defaults to 10 when zero
	// If, when non-empty, is a regex pattern matched against the tool name.
	// The hook is skipped when the pattern does not match.
	// An empty If runs the hook unconditionally.
	If string `json:"if,omitempty"`
	// Source indicates where this hook originated: "user", "plugin", or "builtin".
	// Plugin-sourced hooks require explicit consent via TrustedSources to execute.
	// Empty source is treated as "user" (trusted by default).
	Source string `json:"source,omitempty"`
}

// HookMatcher pairs a regex pattern (matched against tool name) with hook commands.
type HookMatcher struct {
	Matcher string       `json:"matcher"` // regex pattern; empty matches everything
	Hooks   []HookConfig `json:"hooks"`
}

// HooksSettings maps hook events to their ordered list of matchers.
type HooksSettings map[HookEvent][]HookMatcher
