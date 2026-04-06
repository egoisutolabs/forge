// Package observe provides structured event logging for Forge.
// Events are written as JSON lines to session log files for
// post-hoc analysis of tool calls, agent spawns, and API usage.
package observe

import (
	"encoding/json"
	"time"
)

// EventType identifies the kind of observability event.
type EventType string

const (
	EventToolCallStart EventType = "tool_call_start"
	EventToolCallEnd   EventType = "tool_call_end"
	EventAgentSpawn    EventType = "agent_spawn"
	EventAgentComplete EventType = "agent_complete"
	EventSkillInvoke   EventType = "skill_invoke"
	EventAPICall       EventType = "api_call"
	EventError         EventType = "error"
)

// Event is the top-level envelope written as one JSON line.
type Event struct {
	Timestamp time.Time       `json:"ts"`
	SessionID string          `json:"session_id"`
	EventType EventType       `json:"event_type"`
	TraceID   string          `json:"trace_id,omitempty"`
	ParentID  string          `json:"parent_id,omitempty"`
	Turn      int             `json:"turn"`
	Payload   json.RawMessage `json:"payload"`
}

// ToolCallStartPayload is emitted when a tool begins execution.
type ToolCallStartPayload struct {
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	Input      json.RawMessage `json:"input,omitempty"`
	IsConcSafe bool            `json:"is_conc_safe"`
}

// ToolCallEndPayload is emitted when a tool completes.
type ToolCallEndPayload struct {
	ToolName   string `json:"tool_name"`
	ToolUseID  string `json:"tool_use_id"`
	DurationMs int64  `json:"duration_ms"`
	IsError    bool   `json:"is_error"`
	Output     string `json:"output,omitempty"`
	ErrorMsg   string `json:"error_msg,omitempty"`
}

// AgentSpawnPayload is emitted when Agent tool spawns a child.
type AgentSpawnPayload struct {
	AgentID      string `json:"agent_id"`
	Description  string `json:"description"`
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
	IsBackground bool   `json:"is_background"`
	Prompt       string `json:"prompt,omitempty"`
}

// AgentCompletePayload is emitted when a child agent finishes.
type AgentCompletePayload struct {
	AgentID       string `json:"agent_id"`
	DurationMs    int64  `json:"duration_ms"`
	Turns         int    `json:"turns"`
	IsError       bool   `json:"is_error"`
	StopReason    string `json:"stop_reason"`
	OutputPreview string `json:"output_preview,omitempty"`
}

// SkillInvokePayload is emitted when a skill (slash command) is invoked.
type SkillInvokePayload struct {
	SkillName    string   `json:"skill_name"`
	Args         string   `json:"args,omitempty"`
	Source       string   `json:"source"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
	PromptLen    int      `json:"prompt_len"`
}

// APICallPayload is emitted after each Claude API call completes.
type APICallPayload struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CacheRead    int     `json:"cache_read_tokens"`
	CacheCreate  int     `json:"cache_create_tokens"`
	DurationMs   int64   `json:"duration_ms"`
	StopReason   string  `json:"stop_reason"`
	CostUSD      float64 `json:"cost_usd"`
	MaxTokens    int     `json:"max_tokens"`
}

// ErrorPayload captures any error event.
type ErrorPayload struct {
	Source   string `json:"source"`
	ToolName string `json:"tool_name,omitempty"`
	Message  string `json:"message"`
}
