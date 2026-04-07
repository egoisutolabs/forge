package engine

import (
	"context"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/coordinator"
	"github.com/egoisutolabs/forge/internal/hooks"
	"github.com/egoisutolabs/forge/internal/lsp"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
	"github.com/egoisutolabs/forge/internal/services/session"
	"github.com/egoisutolabs/forge/internal/skills"
	"github.com/egoisutolabs/forge/internal/tools"
)

// Config holds the configuration for a QueryEngine instance.
type Config struct {
	Model        string
	SystemPrompt string
	Tools        []tools.Tool
	MaxTurns     int
	MaxBudgetUSD float64 // 0 = unlimited
	Cwd          string
	SessionID    string // if set, auto-save conversation after each turn

	// Skills is the skill registry. Passed through to ToolContext so that
	// SkillTool can look up skill definitions.
	Skills *skills.SkillRegistry

	// Hooks contains hook settings loaded from ~/.forge/settings.json or
	// merged from plugins. Passed through to ToolContext for pre/post
	// tool-use hook execution.
	Hooks hooks.HooksSettings

	// GlobMaxResults caps the number of results GlobTool returns.
	// Defaults to 100 when zero.
	GlobMaxResults int

	// UserPrompt, when non-nil, is called by AskUserQuestionTool to collect
	// answers from the user. If nil, AskUserQuestionTool returns an error.
	UserPrompt func(questions []tools.AskQuestion) (map[string]string, error)

	// PermissionPrompt, when non-nil, is called whenever a tool's
	// CheckPermissions returns PermAsk. Return true to approve, false to deny.
	// If nil, PermAsk is treated as PermDeny (safe default for non-interactive use).
	PermissionPrompt func(message string) bool

	// Notifications, when non-nil, is a channel of formatted task-notification
	// strings from background agents. The engine loop drains this between turns
	// and injects notifications as user messages.
	Notifications <-chan string

	// LSPManager is the language server manager for this session.
	// When non-nil, it is passed to ToolContext so file tools and the LSP tool
	// can interact with language servers.
	LSPManager *lsp.Manager
}

// QueryEngine manages conversation state and drives the agent loop.
type QueryEngine struct {
	config         Config
	messages       []*models.Message
	perms          *permissions.Context
	fileState      *tools.FileStateCache
	totalUsage     models.Usage
	session        *session.Session                            // non-nil when session persistence is enabled
	OnEvent        func(api.StreamEvent)                       // optional callback for streaming events
	OnToolComplete func(name, id, result string, isError bool) // optional callback when a tool finishes
}

// New creates a new QueryEngine with the given configuration.
func New(cfg Config) *QueryEngine {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 100
	}
	qe := &QueryEngine{
		config:     cfg,
		messages:   make([]*models.Message, 0),
		perms:      permissions.NewDefaultContext(cfg.Cwd),
		fileState:  tools.NewFileStateCache(100, 25*1024*1024),
		totalUsage: models.EmptyUsage(),
	}
	if cfg.SessionID != "" {
		qe.session = &session.Session{
			ID:    cfg.SessionID,
			Model: cfg.Model,
			Cwd:   cfg.Cwd,
		}
	}
	return qe
}

// Messages returns the current conversation history.
func (qe *QueryEngine) Messages() []*models.Message {
	return qe.messages
}

// Permissions returns the active permission context.
func (qe *QueryEngine) Permissions() *permissions.Context {
	return qe.perms
}

// Config returns the engine configuration.
func (qe *QueryEngine) Config() Config {
	return qe.config
}

// TotalUsage returns the accumulated token usage across all submissions.
func (qe *QueryEngine) TotalUsage() models.Usage {
	return qe.totalUsage
}

// SetModel changes the active model for subsequent API calls.
func (qe *QueryEngine) SetModel(model string) {
	qe.config.Model = model
}

// SetMaxBudget sets the maximum session budget in USD.
// Pass 0 to remove the budget limit.
func (qe *QueryEngine) SetMaxBudget(usd float64) {
	qe.config.MaxBudgetUSD = usd
}

// SystemPrompt returns the current system prompt.
func (qe *QueryEngine) SystemPrompt() string {
	return qe.config.SystemPrompt
}

// SetMessages replaces the conversation history (used by compact).
func (qe *QueryEngine) SetMessages(msgs []*models.Message) {
	qe.messages = msgs
}

// SetPermissionPrompt updates the permission prompt function used when a tool
// asks for user approval. Call this after construction to wire in a TUI prompt.
func (qe *QueryEngine) SetPermissionPrompt(fn func(string) bool) {
	qe.config.PermissionPrompt = fn
}

// SetUserPrompt updates the user prompt function used by AskUserQuestionTool
// to present choices to the user via the TUI.
func (qe *QueryEngine) SetUserPrompt(fn func(questions []tools.AskQuestion) (map[string]string, error)) {
	qe.config.UserPrompt = fn
}

// SubmitMessage adds a user message and runs the agent loop.
// This is the Go equivalent of Claude Code's QueryEngine.submitMessage().
// Messages persist across calls — each call is a new "turn" in the conversation.
//
// If FORGE_COORDINATOR_MODE=1, the system prompt and tool list are overridden
// to restrict the model to coordinator-only tools (Agent, SendMessage, TaskStop).
func (qe *QueryEngine) SubmitMessage(ctx context.Context, caller api.Caller, prompt string) (*models.LoopResult, error) {
	qe.messages = append(qe.messages, models.NewUserMessage(prompt))

	activeTools := qe.config.Tools
	systemPrompt := qe.config.SystemPrompt
	if coordinator.IsCoordinatorMode() {
		activeTools = coordinator.CoordinatorTools(activeTools)
		systemPrompt = coordinator.CoordinatorSystemPrompt()
	}

	globMax := qe.config.GlobMaxResults
	if globMax == 0 {
		globMax = 100
	}

	tctx := &tools.ToolContext{
		Cwd:              qe.config.Cwd,
		Messages:         qe.messages,
		Model:            qe.config.Model,
		Tools:            activeTools,
		Permissions:      qe.perms,
		FileState:        qe.fileState,
		AbortCtx:         ctx,
		PermissionPrompt: qe.config.PermissionPrompt,
		Caller:           caller,
		Skills:           qe.config.Skills,
		Hooks:            qe.config.Hooks,
		GlobMaxResults:   globMax,
		UserPrompt:       qe.config.UserPrompt,
		LSPManager:       qe.config.LSPManager,
	}

	var budgetPtr *float64
	if qe.config.MaxBudgetUSD > 0 {
		b := qe.config.MaxBudgetUSD
		budgetPtr = &b
	}

	result, updatedMessages, err := RunLoop(ctx, LoopParams{
		Caller:         caller,
		Messages:       qe.messages,
		SystemPrompt:   systemPrompt,
		Tools:          activeTools,
		Model:          qe.config.Model,
		MaxTurns:       qe.config.MaxTurns,
		OnEvent:        qe.OnEvent,
		OnToolComplete: qe.OnToolComplete,
		ToolCtx:        tctx,
		MaxBudgetUSD:   budgetPtr,
		Notifications:  qe.config.Notifications,
	})
	if err != nil {
		return nil, err
	}

	qe.messages = updatedMessages
	qe.totalUsage = models.AccumulateUsage(qe.totalUsage, result.TotalUsage)

	// Auto-save session after each turn if persistence is enabled.
	if qe.session != nil {
		_ = session.AutoSave(qe.session, qe.messages, qe.totalUsage, "")
	}

	return result, nil
}
