package tools

import (
	"context"
	"encoding/json"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/hooks"
	"github.com/egoisutolabs/forge/internal/lsp"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
	"github.com/egoisutolabs/forge/internal/skills"
)

// Tool is the interface every tool must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage, tctx *ToolContext) (*models.ToolResult, error)
	CheckPermissions(input json.RawMessage, tctx *ToolContext) (*models.PermissionDecision, error)
	ValidateInput(input json.RawMessage) error
	IsConcurrencySafe(input json.RawMessage) bool
	IsReadOnly(input json.RawMessage) bool
}

// AskQuestionOption is a single selectable choice within an AskQuestion.
type AskQuestionOption struct {
	Label       string
	Description string
}

// AskQuestion is one question posed by AskUserQuestionTool.
type AskQuestion struct {
	Question    string
	Header      string
	Options     []AskQuestionOption
	MultiSelect bool
}

// ToolContext holds shared state available to all tools during execution.
//
// This is the Go equivalent of Claude Code's ToolUseContext. Claude Code has 50+
// fields on this type, but research shows only ~7 are used by the core 6 tools.
// We carry the ones that matter.
type ToolContext struct {
	// Cwd is the working directory for this session.
	Cwd string

	// Messages is the current conversation history.
	Messages []*models.Message

	// Model is the current model being used (e.g. "claude-sonnet-4-20250514").
	Model string

	// Tools is the set of tools available in this session.
	Tools []Tool

	// Permissions is the active permission context (mode, rules).
	Permissions *permissions.Context

	// FileState is the LRU cache tracking file read state.
	// Used by FileReadTool (populate on read), FileEditTool (guard: must read first),
	// and FileWriteTool (guard: must read first for updates).
	FileState *FileStateCache

	// AbortCtx is the context for cancellation propagation.
	// Tools should check this for cancellation during long-running operations.
	AbortCtx context.Context

	// IsNonInteractive is true when running in auto/non-interactive mode (no user prompts).
	// Affects BashTool permission behavior.
	IsNonInteractive bool

	// AgentID, when non-empty, identifies the sub-agent this context belongs to.
	// Tools can use this to detect teammate/sub-agent execution and adjust behavior
	// (e.g. ExitPlanMode sends a plan_approval_request instead of exiting locally).
	AgentID string

	// GlobMaxResults is the maximum number of results GlobTool should return.
	// Default 100 if zero.
	GlobMaxResults int

	// Hooks contains the hook settings for this session.
	// When non-nil, PreToolUse and PostToolUse hooks are run around each tool call.
	Hooks hooks.HooksSettings

	// TrustedSources lists hook sources that are allowed to execute (e.g.
	// "user", "builtin"). Plugin-sourced hooks not in this list are skipped.
	// When nil, all sources are trusted (backward-compatible default).
	TrustedSources []string

	// UserPrompt is called by AskUserQuestionTool to collect answers from the user.
	// The callback receives the questions and returns a map of question text → answer.
	// Multi-select answers are comma-separated.
	// If nil, AskUserQuestionTool returns an error ("user interaction not available").
	UserPrompt func(questions []AskQuestion) (map[string]string, error)

	// Skills is the registry of available skills (slash commands).
	// Used by SkillTool to look up skill definitions.
	// If nil, SkillTool returns "skill registry not available".
	Skills *skills.SkillRegistry

	// PermissionPrompt, when non-nil, is called whenever CheckPermissions
	// returns PermAsk. It presents message to the user and returns true if the
	// user approves, false if they deny.
	//
	// If nil (the default), PermAsk is treated as PermDeny so that sub-agents
	// and non-interactive invocations fail safe rather than silently proceeding.
	PermissionPrompt func(message string) bool

	// LSPManager is the language server manager for this session.
	// Used by file tools (DidOpen/DidChange/DidSave) and the LSP tool.
	// Nil when no language servers are detected.
	LSPManager *lsp.Manager

	// Caller is the API caller for this session. Available to tools that need
	// to spawn sub-agents or make API calls directly (e.g. ForgeOrchestrator).
	Caller api.Caller
}

// SearchHinter is an optional interface that tools can implement to provide
// a short hint string for keyword-search scoring in ToolSearch.
// If a tool implements SearchHinter and its hint contains a query term, the
// tool receives a +4 score bonus on top of the normal name/description scoring.
type SearchHinter interface {
	SearchHint() string
}

// FindTool looks up a tool by name. Returns nil if not found.
func FindTool(tools []Tool, name string) Tool {
	for _, t := range tools {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

// ToAPISchema converts a tool to the JSON format expected by the Claude API.
func ToAPISchema(t Tool) map[string]any {
	return map[string]any{
		"name":         t.Name(),
		"description":  t.Description(),
		"input_schema": json.RawMessage(t.InputSchema()),
	}
}
