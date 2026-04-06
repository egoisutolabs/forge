package api

import (
	"context"

	"github.com/egoisutolabs/forge/models"
)

// StreamEvent is a single event yielded during API streaming.
type StreamEvent struct {
	// Type is "text_delta", "tool_use", "message_done", or "error".
	Type string

	// Text delta (when Type == "text_delta")
	Text string

	// Complete assistant message (when Type == "message_done")
	Message *models.Message

	// Error (when Type == "error")
	Err error
}

// Caller is the interface for calling the Claude API.
// The loop depends on this interface, not a concrete client.
// This is the equivalent of Claude Code's QueryDeps.callModel.
type Caller interface {
	// Stream sends messages to the Claude API and returns a channel of events.
	// The channel is closed when streaming is complete.
	// The final event will be Type=="message_done" with the full Message.
	Stream(ctx context.Context, params StreamParams) <-chan StreamEvent
}

// StreamParams is everything the API needs to make a request.
type StreamParams struct {
	Messages     []*models.Message
	SystemPrompt string
	Tools        []ToolSchema
	Model        string
	MaxTokens    int
}

// ToolSchema is the JSON format sent to the API for each tool.
type ToolSchema struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}
