package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/engine"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/observe"
	"github.com/egoisutolabs/forge/internal/tools"
)

const toolName = "Agent"

// defaultGeneralPurposePrompt is used when no subagent_type is specified.
const defaultGeneralPurposePrompt = `You are a general-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. Complete the task given to you accurately and thoroughly.`

// toolInput is the JSON schema for AgentTool input.
type toolInput struct {
	Description     string `json:"description"`
	Prompt          string `json:"prompt"`
	SubagentType    string `json:"subagent_type,omitempty"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Name            string `json:"name,omitempty"`
	Mode            string `json:"mode,omitempty"`
}

// Tool implements the Agent tool — spawns a sub-agent to handle complex tasks.
//
// This is the Go equivalent of Claude Code's AgentTool.
// Key behaviors:
//   - Sync mode: creates a child QueryEngine with filtered tools, runs to completion
//   - Async mode: spawns a goroutine, returns {status:"async_launched", agentId, outputFile}
//   - Tool filtering: removes Agent/AskUserQuestion/TaskStop; async agents further restricted
//   - Agent definitions: built-in Explore + Plan, plus custom from LoadAgentsDir
type Tool struct {
	caller   api.Caller
	registry *AgentRegistry
	agents   []AgentDefinition
}

// New creates an AgentTool with the given API caller and agent definitions.
// If agents is nil, BuiltInAgents() is used.
func New(caller api.Caller, agentDefs []AgentDefinition) *Tool {
	if agentDefs == nil {
		agentDefs = BuiltInAgents()
	}
	return &Tool{
		caller:   caller,
		registry: DefaultRegistry,
		agents:   agentDefs,
	}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) Description() string {
	return `Launch a new agent to handle complex, multi-step tasks autonomously.

When to use:
- Complex research tasks that require multiple searches or reads
- Tasks that can be parallelized with run_in_background
- When a specialized agent type (Explore, Plan) fits the task

Available agent types and tools available to them:
- general-purpose: General-purpose agent (all tools except Agent/AskUserQuestion/TaskStop)
- Explore: Fast codebase exploration (Read, Glob, AstGrep, Grep, Bash)
- Plan: Architecture and planning agent (Read, Glob, AstGrep, Grep, Bash)`
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return false }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": {
				"type": "string",
				"description": "A short (3-5 word) description of the task"
			},
			"prompt": {
				"type": "string",
				"description": "The task for the agent to perform"
			},
			"subagent_type": {
				"type": "string",
				"description": "The type of specialized agent to use for this task"
			},
			"model": {
				"type": "string",
				"enum": ["sonnet", "opus", "haiku"],
				"description": "Optional model override for this agent. Takes precedence over the agent definition's model frontmatter. If omitted, uses the agent definition's model, or inherits from the parent."
			},
			"run_in_background": {
				"type": "boolean",
				"description": "Set to true to run this agent in the background. You will be notified when it completes."
			},
			"name": {
				"type": "string",
				"description": "Name for the spawned agent. Makes it addressable via SendMessage({to: name}) while running."
			},
			"mode": {
				"type": "string",
				"description": "Permission mode for spawned teammate (e.g., \"plan\" to require plan approval)."
			}
		},
		"required": ["description", "prompt"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.Description) == "" {
		return fmt.Errorf("description is required and cannot be empty")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return fmt.Errorf("prompt is required and cannot be empty")
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	// Find agent definition for the requested type
	agentDef := t.resolveAgentDef(in.SubagentType)

	// Determine model and system prompt
	model := resolveModel(in.Model, agentDef.Model, tctx)
	systemPrompt := agentDef.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultGeneralPurposePrompt
	}

	// Filter parent tools for the child agent
	parentTools := parentToolSet(tctx)
	childTools := FilterToolsForAgent(parentTools, in.RunInBackground)
	if len(agentDef.Tools) > 0 {
		childTools = FilterToolsByNames(childTools, agentDef.Tools)
	}

	maxTurns := agentDef.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	if in.RunInBackground {
		return t.spawnAsync(ctx, in, model, systemPrompt, childTools, maxTurns, tctx)
	}
	return t.runSync(ctx, in, model, systemPrompt, childTools, maxTurns, tctx)
}

// resolveAgentDef finds the agent definition for the given type name.
// Falls back to a generic general-purpose definition if not found.
func (t *Tool) resolveAgentDef(subagentType string) AgentDefinition {
	if subagentType != "" {
		if def := FindAgent(t.agents, subagentType); def != nil {
			return *def
		}
	}
	return AgentDefinition{
		Name:         "general-purpose",
		Description:  "General-purpose agent",
		SystemPrompt: defaultGeneralPurposePrompt,
	}
}

// runSync executes the child agent synchronously and returns its text output.
func (t *Tool) runSync(ctx context.Context, in toolInput, model, systemPrompt string, childTools []tools.Tool, maxTurns int, tctx *tools.ToolContext) (*models.ToolResult, error) {
	agentID := observe.NewTraceID()
	spawnStart := time.Now()
	observe.EmitAgentSpawn(agentID, in.Description, in.SubagentType, model, false, in.Prompt)

	cwd := cwdFromCtx(tctx)
	child := engine.New(engine.Config{
		Model:        model,
		SystemPrompt: systemPrompt,
		Tools:        childTools,
		MaxTurns:     maxTurns,
		Cwd:          cwd,
	})

	result, err := child.SubmitMessage(ctx, t.caller, in.Prompt)

	// Emit agent_complete regardless of outcome.
	isErr := err != nil
	var stopReason string
	var turns int
	if result != nil {
		stopReason = string(result.Reason)
		turns = result.Turns
	}
	if isErr {
		stopReason = "error"
	}
	observe.EmitAgentComplete(agentID, time.Since(spawnStart), turns, isErr, stopReason)

	if err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Agent error: %s", err),
			IsError: true,
		}, nil
	}

	output := lastAssistantText(child.Messages())
	if output == "" {
		output = fmt.Sprintf("Agent completed (reason: %s, turns: %d)", result.Reason, result.Turns)
	}
	return &models.ToolResult{Content: output}, nil
}

// spawnAsync launches the child agent in a goroutine and returns immediately
// with {status:"async_launched", agentId, outputFile}.
func (t *Tool) spawnAsync(ctx context.Context, in toolInput, model, systemPrompt string, childTools []tools.Tool, maxTurns int, tctx *tools.ToolContext) (*models.ToolResult, error) {
	agentID := uuid.NewString()
	outputFile, err := outputFilePath(agentID)
	if err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Failed to create output file: %s", err),
			IsError: true,
		}, nil
	}

	ba := &BackgroundAgent{
		AgentID:     agentID,
		Description: in.Description,
		Status:      AgentStatusRunning,
		OutputFile:  outputFile,
		StartedAt:   time.Now(),
	}
	t.registry.Register(ba)

	observe.EmitAgentSpawn(agentID, in.Description, in.SubagentType, model, true, in.Prompt)

	cwd := cwdFromCtx(tctx)
	caller := t.caller
	registry := t.registry
	spawnStart := time.Now()

	go func() {
		child := engine.New(engine.Config{
			Model:        model,
			SystemPrompt: systemPrompt,
			Tools:        childTools,
			MaxTurns:     maxTurns,
			Cwd:          cwd,
		})

		// Background agents run independently — don't inherit parent cancellation.
		result, err := child.SubmitMessage(context.Background(), caller, in.Prompt)

		isErr := err != nil
		var stopReason string
		var turns int
		if result != nil {
			stopReason = string(result.Reason)
			turns = result.Turns
		}
		if isErr {
			stopReason = "error"
		}
		observe.EmitAgentComplete(agentID, time.Since(spawnStart), turns, isErr, stopReason)

		if err != nil {
			registry.Fail(agentID, err.Error())
			return
		}

		output := lastAssistantText(child.Messages())
		if output == "" {
			output = fmt.Sprintf("Agent completed: %s", in.Description)
		}
		registry.Complete(agentID, output)
	}()

	// Suppress ctx unused warning — ctx is intentionally not forwarded to bg agent.
	_ = ctx

	resp, _ := json.Marshal(map[string]any{
		"status":      "async_launched",
		"agentId":     agentID,
		"description": in.Description,
		"outputFile":  outputFile,
	})
	return &models.ToolResult{Content: string(resp)}, nil
}

// resolveModel maps an alias to a full model ID, falling back through
// agentDefault then the parent model in tctx.
func resolveModel(alias, agentDefault string, tctx *tools.ToolContext) string {
	if alias == "" {
		alias = agentDefault
	}
	switch strings.ToLower(alias) {
	case "sonnet":
		return "claude-sonnet-4-6"
	case "opus":
		return "claude-opus-4-6"
	case "haiku":
		return "claude-haiku-4-5-20251001"
	default:
		if alias != "" {
			return alias // treat as full model ID
		}
	}
	if tctx != nil && tctx.Model != "" {
		return tctx.Model
	}
	return "claude-sonnet-4-6"
}

// parentToolSet returns the tool set from tctx, or nil if unavailable.
func parentToolSet(tctx *tools.ToolContext) []tools.Tool {
	if tctx != nil {
		return tctx.Tools
	}
	return nil
}

// cwdFromCtx returns the working directory from tctx, or empty string.
func cwdFromCtx(tctx *tools.ToolContext) string {
	if tctx != nil {
		return tctx.Cwd
	}
	return ""
}

// lastAssistantText returns the text content of the last assistant message.
func lastAssistantText(messages []*models.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == models.RoleAssistant {
			return messages[i].TextContent()
		}
	}
	return ""
}
