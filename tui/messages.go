package tui

import (
	"time"

	"github.com/egoisutolabs/forge/models"
)

// ActiveToolInfo tracks an in-progress tool with timing and detail info.
type ActiveToolInfo struct {
	Name      string
	ID        string
	Detail    string // human-readable: file path, command, pattern
	StartTime time.Time
}

// StreamTextMsg is sent when a text delta arrives from the API stream.
type StreamTextMsg struct {
	Text string
}

// ToolStartMsg is sent when a tool call begins execution.
type ToolStartMsg struct {
	Name   string
	ID     string
	Detail string // human-readable detail: file path, command, pattern, etc.
}

// ToolDoneMsg is sent when a tool call completes.
type ToolDoneMsg struct {
	ID      string
	Name    string
	Result  string
	IsError bool
}

// PromptDoneMsg is sent when the engine finishes processing a prompt.
type PromptDoneMsg struct {
	Result *models.LoopResult
}

// ErrorMsg is sent when an unrecoverable error occurs.
type ErrorMsg struct {
	Err error
}

// CostUpdateMsg is sent at the end of each turn with token/cost info.
type CostUpdateMsg struct {
	Usage   models.Usage
	CostUSD float64
}

// PermissionRequestMsg is sent when a tool requires interactive approval.
type PermissionRequestMsg struct {
	ToolName   string
	Action     string // human-readable action description (e.g. "Execute command")
	Detail     string // specific command/path/content
	Risk       RiskLevel
	Message    string
	ResponseCh chan bool
}

// RiskLevel classifies the danger level of a tool operation.
type RiskLevel int

const (
	RiskLow      RiskLevel = iota // read-only tools
	RiskModerate                  // file modifications
	RiskHigh                      // destructive/system commands
)

// String returns a human-readable risk label.
func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskModerate:
		return "moderate"
	case RiskHigh:
		return "high"
	default:
		return "unknown"
	}
}

// AgentSpawnMsg is sent when a sub-agent is launched.
type AgentSpawnMsg struct {
	Name       string
	Background bool
}

// AgentDoneMsg is sent when a background sub-agent completes.
type AgentDoneMsg struct {
	Name string
}

// FileAccessMsg is sent when a file read/edit/write is in progress.
type FileAccessMsg struct {
	Path   string
	Action string // "Reading", "Editing", "Writing"
}
