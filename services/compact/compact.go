// Package compact implements auto-compaction of conversation history.
//
// When a conversation grows near the model's context window limit,
// CompactConversation makes a side-query to Claude to summarize the
// older messages, replacing them with a compact boundary message.
// This mirrors Claude Code's microcompact behavior.
package compact

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/models"
)

const (
	// AutoCompactBufferTokens is the safety margin below the context window.
	// When inputTokens + AutoCompactBufferTokens >= contextWindowTokens, we compact.
	AutoCompactBufferTokens = 13000

	// ContextWindowTokens is the assumed context window for all current Claude models.
	ContextWindowTokens = 200_000

	// CompactModel is the model used for summarization side-queries (fast + cheap).
	CompactModel = "claude-haiku-4-5-20251001"

	// MaxCircuitBreakFailures is the number of consecutive compact failures before
	// the circuit breaker trips and compaction is disabled for the session.
	MaxCircuitBreakFailures = 3
)

// ShouldCompact returns true when the current turn's input token count is
// within AutoCompactBufferTokens of the context window limit.
func ShouldCompact(inputTokens int) bool {
	return inputTokens+AutoCompactBufferTokens >= ContextWindowTokens
}

// EstimateTokens returns a rough token count for a slice of messages.
// Heuristic: sum len(content)/4 across all text, tool-use inputs, and tool results.
// This is intentionally a fast approximation — use ShouldCompact with the actual
// API-reported token count when available; fall back to this for proactive checks.
func EstimateTokens(messages []*models.Message) int {
	total := 0
	for _, msg := range messages {
		text := msg.TextContent()
		total += len(text) / 4
		for _, b := range msg.Content {
			switch b.Type {
			case models.BlockToolUse:
				total += len(b.Input) / 4
			case models.BlockToolResult:
				total += len(b.Content) / 4
			}
		}
	}
	return total
}

// IncrementalEstimate returns the token estimate for newMessages only, added to
// an existing count. This avoids rescanning the full history every turn.
// The caller maintains a running total and passes it as existingCount.
func IncrementalEstimate(existingCount int, newMessages []*models.Message) int {
	return existingCount + EstimateTokens(newMessages)
}

// CompactConversation summarises messages into a compact boundary message by
// making a side-query to Claude. The system prompt is included for context.
//
// Returns a slice containing a single user message with the summary,
// suitable for replacing the full message history.
func CompactConversation(ctx context.Context, caller api.Caller, messages []*models.Message, systemPrompt string) ([]*models.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	summaryPrompt := buildSummaryPrompt(messages, systemPrompt)

	events := caller.Stream(ctx, api.StreamParams{
		Messages:  []*models.Message{models.NewUserMessage(summaryPrompt)},
		Model:     CompactModel,
		MaxTokens: 8192,
	})

	var summaryMsg *models.Message
	var streamErr error
	for event := range events {
		switch event.Type {
		case "message_done":
			summaryMsg = event.Message
		case "error":
			streamErr = event.Err
		}
	}

	if streamErr != nil {
		return nil, fmt.Errorf("compact: summarization stream error: %w", streamErr)
	}
	if summaryMsg == nil {
		return nil, fmt.Errorf("compact: no summary message received")
	}

	summary := summaryMsg.TextContent()
	if summary == "" {
		return nil, fmt.Errorf("compact: empty summary returned")
	}

	boundary := fmt.Sprintf(
		"[Compacted conversation — %d messages summarized]\n\n%s",
		len(messages),
		summary,
	)
	return []*models.Message{models.NewUserMessage(boundary)}, nil
}

// buildSummaryPrompt constructs the prompt sent to Claude for summarization.
func buildSummaryPrompt(messages []*models.Message, systemPrompt string) string {
	var sb strings.Builder
	sb.WriteString("Produce a detailed summary of the conversation below that preserves " +
		"all key facts, decisions, tool calls and their results, file changes, and any " +
		"other information needed to continue the work seamlessly.\n\n")

	if systemPrompt != "" {
		sb.WriteString("System context: ")
		sb.WriteString(systemPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Conversation to summarize:\n\n")
	for _, msg := range messages {
		role := string(msg.Role)
		text := msg.TextContent()
		if text != "" {
			sb.WriteString(fmt.Sprintf("[%s]: %s\n", role, text))
		}
		// Include tool calls / results as structured info
		for _, b := range msg.Content {
			switch b.Type {
			case models.BlockToolUse:
				sb.WriteString(fmt.Sprintf("[tool_use:%s id=%s input=%s]\n", b.Name, b.ID, string(b.Input)))
			case models.BlockToolResult:
				errTag := ""
				if b.IsError {
					errTag = " (error)"
				}
				sb.WriteString(fmt.Sprintf("[tool_result id=%s%s]: %s\n", b.ToolUseID, errTag, b.Content))
			}
		}
	}

	return sb.String()
}
