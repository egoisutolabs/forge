package api

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/egoisutolabs/forge/internal/log"
	"github.com/egoisutolabs/forge/models"
)

// --- OpenAI SSE response types ---

// openaiChunk is a single SSE chunk from OpenAI streaming.
type openaiChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   *openaiUsage   `json:"usage,omitempty"`
}

// openaiChoice is one choice in a streaming chunk.
type openaiChoice struct {
	Index        int         `json:"index"`
	Delta        openaiDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// openaiDelta holds incremental content/tool_call fragments.
type openaiDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   *string         `json:"content,omitempty"`
	ToolCalls []openaiDeltaTC `json:"tool_calls,omitempty"`
}

// openaiDeltaTC is a tool call fragment in a streaming delta.
type openaiDeltaTC struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function *openaiDeltaFn `json:"function,omitempty"`
}

// openaiDeltaFn is a function fragment in a streaming tool call delta.
type openaiDeltaFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// openaiUsage is the token usage reported in the final streaming chunk.
type openaiUsage struct {
	PromptTokens        int                 `json:"prompt_tokens"`
	CompletionTokens    int                 `json:"completion_tokens"`
	TotalTokens         int                 `json:"total_tokens"`
	PromptTokensDetails *openaiPromptDetail `json:"prompt_tokens_details,omitempty"`
}

// openaiPromptDetail holds optional cached-token info from some providers.
type openaiPromptDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

// --- Stream state ---

// openaiStreamState accumulates content across SSE chunks.
type openaiStreamState struct {
	contentBuf   strings.Builder
	toolCalls    map[int]*toolCallAccum
	toolOrder    []int // track insertion order of tool call indices
	finishReason string
	usage        *openaiUsage
	msgID        string
	model        string
}

// toolCallAccum accumulates fragments for a single tool call.
type toolCallAccum struct {
	id      string
	name    string
	argsBuf strings.Builder
}

// parseOpenAISSEStream reads an OpenAI SSE stream and emits StreamEvents.
func parseOpenAISSEStream(body io.Reader, ch chan<- StreamEvent) error {
	state := &openaiStreamState{
		toolCalls: make(map[int]*toolCallAccum),
	}

	scanner := bufio.NewScanner(body)
	buf := make([]byte, 1<<20)
	scanner.Buffer(buf, 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") && !strings.HasPrefix(line, "data:") {
			continue
		}
		// Extract data after "data: " or "data:"
		data := strings.TrimPrefix(line, "data: ")
		data = strings.TrimPrefix(data, "data:")
		data = strings.TrimSpace(data)

		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}

		if err := processOpenAIChunk(data, state, ch); err != nil {
			return err
		}
	}

	// If we got a finish reason but no message_done was emitted yet
	// (can happen if [DONE] arrived after finish_reason chunk),
	// build and emit the final message.
	if state.finishReason != "" {
		msg := buildOpenAIFinalMessage(state)
		select {
		case ch <- StreamEvent{Type: "message_done", Message: msg}:
		default:
			ch <- StreamEvent{Type: "message_done", Message: msg}
		}
	}

	return scanner.Err()
}

func processOpenAIChunk(data string, state *openaiStreamState, ch chan<- StreamEvent) error {
	var chunk openaiChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		log.Debug("OpenAI SSE: ignoring malformed chunk: %v", err)
		return nil // ignore malformed
	}

	if chunk.ID != "" {
		state.msgID = chunk.ID
	}
	if chunk.Model != "" {
		state.model = chunk.Model
	}

	// Extract usage from final chunk (stream_options.include_usage).
	if chunk.Usage != nil {
		state.usage = chunk.Usage
	}

	for _, choice := range chunk.Choices {
		delta := choice.Delta

		// Text content delta.
		if delta.Content != nil && *delta.Content != "" {
			state.contentBuf.WriteString(*delta.Content)
			select {
			case ch <- StreamEvent{Type: "text_delta", Text: *delta.Content}:
			default:
				ch <- StreamEvent{Type: "text_delta", Text: *delta.Content}
			}
		}

		// Tool call deltas — accumulate fragments.
		for _, tc := range delta.ToolCalls {
			accum, ok := state.toolCalls[tc.Index]
			if !ok {
				accum = &toolCallAccum{}
				state.toolCalls[tc.Index] = accum
				state.toolOrder = append(state.toolOrder, tc.Index)
			}
			if tc.ID != "" {
				accum.id = tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != "" {
					accum.name = tc.Function.Name
				}
				accum.argsBuf.WriteString(tc.Function.Arguments)
			}
		}

		// Finish reason.
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			state.finishReason = *choice.FinishReason
		}
	}

	return nil
}

// mapFinishReason converts an OpenAI finish_reason to an internal StopReason.
func mapFinishReason(reason string) models.StopReason {
	switch reason {
	case "stop":
		return models.StopEndTurn
	case "tool_calls":
		return models.StopToolUse
	case "length":
		return models.StopMaxTokens
	case "content_filter":
		log.Debug("OpenAI: content_filter finish_reason, mapping to end_turn")
		return models.StopEndTurn
	default:
		return models.StopEndTurn
	}
}

// fromOpenAIUsage converts OpenAI usage to internal Usage.
func fromOpenAIUsage(ou *openaiUsage) *models.Usage {
	if ou == nil {
		return nil
	}
	u := &models.Usage{
		InputTokens:  ou.PromptTokens,
		OutputTokens: ou.CompletionTokens,
		ServiceTier:  "standard",
		Speed:        "standard",
	}
	// Best-effort: map cached tokens if the provider reports them.
	if ou.PromptTokensDetails != nil {
		u.CacheRead = ou.PromptTokensDetails.CachedTokens
	}
	return u
}

// buildOpenAIFinalMessage assembles a Message from accumulated stream state.
func buildOpenAIFinalMessage(state *openaiStreamState) *models.Message {
	var content []models.Block

	// Add text block if we accumulated any text.
	if text := state.contentBuf.String(); text != "" {
		content = append(content, models.Block{
			Type: models.BlockText,
			Text: text,
		})
	}

	// Add tool call blocks in order.
	for _, idx := range state.toolOrder {
		accum, ok := state.toolCalls[idx]
		if !ok {
			continue
		}
		args := accum.argsBuf.String()
		if args == "" {
			args = "{}"
		}
		content = append(content, models.Block{
			Type:  models.BlockToolUse,
			ID:    accum.id,
			Name:  accum.name,
			Input: json.RawMessage(args),
		})
	}

	msgID := state.msgID
	if msgID == "" {
		msgID = uuid.NewString()
	}

	return &models.Message{
		ID:         msgID,
		Role:       models.RoleAssistant,
		Content:    content,
		Model:      state.model,
		StopReason: mapFinishReason(state.finishReason),
		Usage:      fromOpenAIUsage(state.usage),
		Timestamp:  time.Now(),
	}
}
