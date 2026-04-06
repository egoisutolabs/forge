package api

import (
	"encoding/json"
	"strings"

	"github.com/egoisutolabs/forge/models"
)

// --- OpenAI wire types ---

// openaiMessage is a single message in the OpenAI ChatCompletion format.
type openaiMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openaiToolCall is a tool invocation returned by the model.
type openaiToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // always "function"
	Function openaiCallFunc `json:"function"`
}

// openaiCallFunc is the function portion of a tool call.
type openaiCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openaiTool is a tool definition in the OpenAI ChatCompletion format.
type openaiTool struct {
	Type     string         `json:"type"` // always "function"
	Function openaiToolFunc `json:"function"`
}

// openaiToolFunc is the function portion of a tool definition.
type openaiToolFunc struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// --- Translation functions ---

// toOpenAIMessages converts internal messages to OpenAI ChatCompletion format.
// The system prompt becomes a leading system message; user text is flattened;
// tool_use becomes tool_calls; tool_result becomes role:tool messages.
func toOpenAIMessages(system string, msgs []*models.Message) []openaiMessage {
	var out []openaiMessage

	if system != "" {
		out = append(out, openaiMessage{Role: "system", Content: system})
	}

	for _, msg := range msgs {
		switch msg.Role {
		case models.RoleUser:
			out = append(out, translateUserMessage(msg)...)
		case models.RoleAssistant:
			out = append(out, translateAssistantMessage(msg))
		}
	}
	return out
}

// translateUserMessage converts a user message.
// - Pure text blocks become a single user message with concatenated text.
// - tool_result blocks each become a role:tool message.
// - Mixed: tool messages first, then user text.
func translateUserMessage(msg *models.Message) []openaiMessage {
	var toolMsgs []openaiMessage
	var textParts []string

	for _, b := range msg.Content {
		switch b.Type {
		case models.BlockToolResult:
			toolMsgs = append(toolMsgs, openaiMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    b.Content,
			})
		case models.BlockText:
			textParts = append(textParts, b.Text)
		}
	}

	var out []openaiMessage
	// Emit tool messages first.
	out = append(out, toolMsgs...)
	// Then emit user text if any.
	if len(textParts) > 0 {
		out = append(out, openaiMessage{
			Role:    "user",
			Content: strings.Join(textParts, ""),
		})
	}
	return out
}

// translateAssistantMessage converts an assistant message.
// Text blocks are concatenated into content; tool_use blocks become tool_calls.
func translateAssistantMessage(msg *models.Message) openaiMessage {
	var textParts []string
	var toolCalls []openaiToolCall

	for _, b := range msg.Content {
		switch b.Type {
		case models.BlockText:
			textParts = append(textParts, b.Text)
		case models.BlockToolUse:
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, openaiToolCall{
				ID:   b.ID,
				Type: "function",
				Function: openaiCallFunc{
					Name:      b.Name,
					Arguments: args,
				},
			})
		}
	}

	return openaiMessage{
		Role:      "assistant",
		Content:   strings.Join(textParts, ""),
		ToolCalls: toolCalls,
	}
}

// toOpenAITools converts internal tool schemas to OpenAI format.
// The JSON Schema is the same; only the wrapper differs
// (input_schema → parameters, wrapped in {"type":"function","function":{...}}).
func toOpenAITools(tools []ToolSchema) []openaiTool {
	out := make([]openaiTool, len(tools))
	for i, t := range tools {
		out[i] = openaiTool{
			Type: "function",
			Function: openaiToolFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return out
}

// fromOpenAIToolCall converts an OpenAI tool_call to an internal Block.
func fromOpenAIToolCall(tc openaiToolCall) models.Block {
	args := tc.Function.Arguments
	if args == "" {
		args = "{}"
	}
	return models.Block{
		Type:  models.BlockToolUse,
		ID:    tc.ID,
		Name:  tc.Function.Name,
		Input: json.RawMessage(args),
	}
}
