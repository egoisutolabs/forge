package engine

import (
	"context"
	"time"

	"github.com/egoisutolabs/forge/internal/api"
	log "github.com/egoisutolabs/forge/internal/logger"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/observe"
	"github.com/egoisutolabs/forge/internal/services/compact"
	"github.com/egoisutolabs/forge/internal/tools"
)

// LoopParams is everything the loop needs to run.
type LoopParams struct {
	Caller         api.Caller
	Messages       []*models.Message
	SystemPrompt   string
	Tools          []tools.Tool
	Model          string
	MaxTurns       int
	OnEvent        func(api.StreamEvent)                       // optional callback for streaming events
	OnToolComplete func(name, id, result string, isError bool) // optional callback when a tool finishes
	ToolCtx        *tools.ToolContext                          // shared tool context (created if nil)
	MaxBudgetUSD   *float64                                    // nil = unlimited
	Notifications  <-chan string                               // background agent notifications (nil = none)
}

// maxTokensEscalated is the token limit used after the first max_tokens hit.
const maxTokensEscalated = 64000

// maxTokensMaxRetries is the max number of subsequent "Resume directly" retries.
const maxTokensMaxRetries = 3

// RunLoop is the core agent loop. This is the Go equivalent of
// Claude Code's queryLoop() in query.ts.
//
// The loop:
//  1. Calls the Claude API with the current messages + tools
//  2. If Claude responds with no tool calls → done
//  3. If Claude responds with tool calls → execute them, append results, repeat
//  4. If maxTurns reached → stop
//  5. If context cancelled → abort
//  6. On max_tokens: escalate token limit, then inject resume message (up to 3x)
//  7. After each turn: auto-compact if context is near the limit
func RunLoop(ctx context.Context, params LoopParams) (*models.LoopResult, []*models.Message, error) {
	// Pre-allocate with headroom for a typical session (assistant + tool_result +
	// user per turn) to avoid repeated slice growth and copying.
	const estimatedTurns = 200
	initCap := len(params.Messages) + estimatedTurns*3
	messages := make([]*models.Message, len(params.Messages), initCap)
	copy(messages, params.Messages)

	toolSchemas := buildToolSchemas(params.Tools)
	tctx := params.ToolCtx
	totalUsage := models.EmptyUsage()
	turns := 0
	maxTokens := 8192

	// max_tokens recovery state
	maxTokensRetries := 0

	// auto-compact circuit breaker
	compactFailures := 0

	// Incremental token estimate — avoid rescanning full history each turn.
	tokenEstimate := compact.EstimateTokens(messages)

	for {
		// Check context before each turn
		if ctx.Err() != nil {
			return &models.LoopResult{
				Reason:       models.StopAborted,
				Messages:     len(messages),
				Turns:        turns,
				TotalUsage:   totalUsage,
				TotalCostUSD: models.CostForModel(params.Model, totalUsage),
			}, messages, nil
		}

		turns++
		observe.SetTurn(turns)
		log.Debug("Loop turn %d: %d messages", turns, len(messages))

		// ---- MICROCOMPACT (before full compact) ----
		// Clear old tool_result content to reduce token usage cheaply
		// before the more expensive summarization compact.
		mcResult := compact.MicroCompact(messages, compact.DefaultKeepRecent)
		messages = mcResult.Messages

		// ---- PROACTIVE AUTO-COMPACT (before API call) ----
		// Use the incremental token estimate (maintained across turns) rather
		// than rescanning the full message history each iteration.
		if compactFailures < compact.MaxCircuitBreakFailures &&
			compact.ShouldCompact(tokenEstimate) {
			compacted, err := compact.CompactConversation(ctx, params.Caller, messages, params.SystemPrompt)
			if err != nil {
				compactFailures++
			} else {
				messages = compacted
				tokenEstimate = compact.EstimateTokens(messages) // reset after compaction
				compactFailures = 0
			}
		}

		// ---- CALL API ----
		apiCallStart := time.Now()
		events := params.Caller.Stream(ctx, api.StreamParams{
			Messages:     models.NormalizeForAPI(messages),
			SystemPrompt: params.SystemPrompt,
			Tools:        toolSchemas,
			Model:        params.Model,
			MaxTokens:    maxTokens,
		})

		// Drain the stream, collect the final assistant message
		var assistantMsg *models.Message
		var streamErr error

		for event := range events {
			// Forward events to callback
			if params.OnEvent != nil {
				params.OnEvent(event)
			}

			switch event.Type {
			case "message_done":
				assistantMsg = event.Message
			case "error":
				streamErr = event.Err
			}
		}

		// ---- EMIT API CALL EVENT ----
		if observe.Enabled() {
			apiDuration := time.Since(apiCallStart)
			apiPayload := observe.APICallPayload{
				Model:      params.Model,
				MaxTokens:  maxTokens,
				DurationMs: apiDuration.Milliseconds(),
			}
			if assistantMsg != nil {
				apiPayload.StopReason = string(assistantMsg.StopReason)
				if assistantMsg.Usage != nil {
					u := assistantMsg.Usage
					apiPayload.InputTokens = u.InputTokens
					apiPayload.OutputTokens = u.OutputTokens
					apiPayload.CacheRead = u.CacheRead
					apiPayload.CacheCreate = u.CacheCreate
					apiPayload.CostUSD = models.CostForModel(params.Model, *u)
				}
			}
			if streamErr != nil {
				apiPayload.StopReason = "error"
				observe.EmitError("api", "", streamErr.Error())
			}
			observe.EmitAPICall(apiPayload)
		}

		// Handle API errors
		if streamErr != nil {
			log.Debug("Stream error: %v", streamErr)
		}
		if assistantMsg != nil {
			log.Debug("Assistant message: stop=%s blocks=%d", assistantMsg.StopReason, len(assistantMsg.Content))
		}
		if streamErr != nil || assistantMsg == nil {
			return &models.LoopResult{
				Reason:       models.StopModelError,
				Messages:     len(messages),
				Turns:        turns,
				TotalUsage:   totalUsage,
				TotalCostUSD: models.CostForModel(params.Model, totalUsage),
			}, messages, nil
		}

		// ---- MAX TOKENS RECOVERY (Step 11) ----
		// Detect truncated output and recover: first escalate token limit,
		// then inject "Resume" messages. Mirror Claude Code's query.ts behavior.
		if assistantMsg.StopReason == models.StopMaxTokens {
			if maxTokensRetries == 0 {
				// First hit: escalate MaxTokens, retry without appending truncated msg
				maxTokens = maxTokensEscalated
				maxTokensRetries++
				turns-- // don't count the failed attempt as a real turn
				continue
			} else if maxTokensRetries <= maxTokensMaxRetries {
				// Subsequent hits: append truncated message + resume instruction
				resumeMsg := models.NewUserMessage("Resume directly — no apology, no recap")
				messages = append(messages, assistantMsg)
				if assistantMsg.Usage != nil {
					totalUsage = models.AccumulateUsage(totalUsage, *assistantMsg.Usage)
				}
				messages = append(messages, resumeMsg)
				tokenEstimate = compact.IncrementalEstimate(tokenEstimate, []*models.Message{assistantMsg, resumeMsg})
				maxTokensRetries++
				continue
			}
			// Too many retries — surface as error
			if assistantMsg.Usage != nil {
				totalUsage = models.AccumulateUsage(totalUsage, *assistantMsg.Usage)
			}
			return &models.LoopResult{
				Reason:       models.StopOutputTruncated,
				Messages:     len(messages),
				Turns:        turns,
				TotalUsage:   totalUsage,
				TotalCostUSD: models.CostForModel(params.Model, totalUsage),
			}, messages, nil
		}
		// Reset retries on a successful (non-truncated) response
		maxTokensRetries = 0

		// Append assistant message to history and update incremental estimate.
		messages = append(messages, assistantMsg)
		tokenEstimate = compact.IncrementalEstimate(tokenEstimate, []*models.Message{assistantMsg})

		// ---- ACCUMULATE USAGE ----
		if assistantMsg.Usage != nil {
			totalUsage = models.AccumulateUsage(totalUsage, *assistantMsg.Usage)
		}

		// ---- CHECK BUDGET ----
		if params.MaxBudgetUSD != nil {
			cost := models.CostForModel(params.Model, totalUsage)
			if cost >= *params.MaxBudgetUSD {
				return &models.LoopResult{
					Reason:       models.StopBudgetExceeded,
					Messages:     len(messages),
					Turns:        turns,
					TotalUsage:   totalUsage,
					TotalCostUSD: cost,
				}, messages, nil
			}
		}

		// ---- AUTO-COMPACT (Step 12) ----
		// After each turn, check if we're near the context window limit.
		// If so, side-query Claude to summarize the conversation.
		// Circuit breaker: stop trying after 3 consecutive failures.
		if assistantMsg.Usage != nil && compactFailures < compact.MaxCircuitBreakFailures &&
			compact.ShouldCompact(assistantMsg.Usage.InputTokens) {
			compacted, err := compact.CompactConversation(ctx, params.Caller, messages, params.SystemPrompt)
			if err != nil {
				compactFailures++
			} else {
				messages = compacted
				tokenEstimate = compact.EstimateTokens(messages) // reset after compaction
				compactFailures = 0
			}
		}

		// ---- DECIDE ----
		toolUseBlocks := assistantMsg.ToolUseBlocks()

		if len(toolUseBlocks) == 0 {
			// No tool calls — we're done
			return &models.LoopResult{
				Reason:       models.StopCompleted,
				Messages:     len(messages),
				Turns:        turns,
				TotalUsage:   totalUsage,
				TotalCostUSD: models.CostForModel(params.Model, totalUsage),
			}, messages, nil
		}

		// ---- EXECUTE TOOLS via StreamingExecutor (Step 10) ----
		// Update ToolContext with current messages before tool execution.
		if tctx != nil {
			tctx.Messages = messages
		}

		// Use StreamingExecutor for concurrent-safe parallel execution.
		// It handles: concurrent tools in parallel, non-concurrent tools serially,
		// and yields results in insertion order.
		se := tools.NewStreamingExecutor(ctx, params.Tools, tctx)
		for _, block := range toolUseBlocks {
			se.AddTool(block)
		}
		se.Done()

		// Build a lookup from tool-use ID → tool name so we can include the
		// name in the OnToolComplete callback.
		toolNameByID := make(map[string]string, len(toolUseBlocks))
		for _, b := range toolUseBlocks {
			toolNameByID[b.ID] = b.Name
		}

		var resultBlocks []models.Block
		for result := range se.Results() {
			resultBlocks = append(resultBlocks, result)
			if params.OnToolComplete != nil {
				params.OnToolComplete(toolNameByID[result.ToolUseID], result.ToolUseID, result.Content, result.IsError)
			}
		}

		// Build a single user message with all tool_result blocks
		toolResultMsg := models.NewToolResultMessage(resultBlocks)
		messages = append(messages, toolResultMsg)
		tokenEstimate = compact.IncrementalEstimate(tokenEstimate, []*models.Message{toolResultMsg})

		// ---- DRAIN BACKGROUND AGENT NOTIFICATIONS ----
		// Inject any completed/failed background agent results as user messages.
		// NormalizeForAPI will merge these with the preceding tool_result message.
		messages = drainNotifications(params.Notifications, messages)

		// ---- CHECK TURN LIMIT ----
		if turns >= params.MaxTurns {
			return &models.LoopResult{
				Reason:       models.StopBlockingLimit,
				Messages:     len(messages),
				Turns:        turns,
				TotalUsage:   totalUsage,
				TotalCostUSD: models.CostForModel(params.Model, totalUsage),
			}, messages, nil
		}
	}
}

// drainNotifications reads all available notification strings from ch
// (non-blocking) and appends each as a user message.
func drainNotifications(ch <-chan string, messages []*models.Message) []*models.Message {
	if ch == nil {
		return messages
	}
	for {
		select {
		case msg := <-ch:
			messages = append(messages, models.NewUserMessage(msg))
		default:
			return messages
		}
	}
}

func buildToolSchemas(tt []tools.Tool) []api.ToolSchema {
	schemas := make([]api.ToolSchema, 0, len(tt))
	for _, t := range tt {
		schemas = append(schemas, api.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return schemas
}
