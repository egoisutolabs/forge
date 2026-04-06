package models

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Role represents the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
)

// Message is a single entry in the conversation history.
type Message struct {
	ID        string    `json:"id"`
	Role      Role      `json:"role"`
	Content   []Block   `json:"content"`
	Timestamp time.Time `json:"timestamp"`

	// Assistant-only fields
	Model      string     `json:"model,omitempty"`
	StopReason StopReason `json:"stop_reason,omitempty"`
	Usage      *Usage     `json:"usage,omitempty"`
}

// NewUserMessage creates a user message with text content.
func NewUserMessage(text string) *Message {
	return &Message{
		ID:        uuid.NewString(),
		Role:      RoleUser,
		Content:   []Block{{Type: BlockText, Text: text}},
		Timestamp: time.Now(),
	}
}

// NewToolResultMessage creates a user message containing tool results.
func NewToolResultMessage(results []Block) *Message {
	return &Message{
		ID:        uuid.NewString(),
		Role:      RoleUser,
		Content:   results,
		Timestamp: time.Now(),
	}
}

// ToolUseBlocks returns all tool_use blocks from the message.
func (m *Message) ToolUseBlocks() []Block {
	var blocks []Block
	for _, b := range m.Content {
		if b.Type == BlockToolUse {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// TextContent returns the concatenated text from all text blocks.
func (m *Message) TextContent() string {
	var sb strings.Builder
	for _, b := range m.Content {
		if b.Type == BlockText {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// HasToolUse returns true if the message contains at least one tool_use block.
func (m *Message) HasToolUse() bool {
	for _, b := range m.Content {
		if b.Type == BlockToolUse {
			return true
		}
	}
	return false
}

// NormalizeForAPI prepares the internal message history for the Claude API.
// It enforces: strict role alternation by merging adjacent same-role messages.
func NormalizeForAPI(messages []*Message) []*Message {
	if len(messages) == 0 {
		return nil
	}

	var result []*Message
	for _, msg := range messages {
		if len(result) == 0 {
			result = append(result, msg)
			continue
		}

		last := result[len(result)-1]
		if last.Role == msg.Role {
			last.Content = append(last.Content, msg.Content...)
		} else {
			result = append(result, msg)
		}
	}

	return result
}

// EnsureToolResults checks that every tool_use block in assistant messages
// has a corresponding tool_result. Returns error tool_result blocks for orphans.
func EnsureToolResults(messages []*Message) []Block {
	toolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == RoleAssistant {
			for _, b := range msg.Content {
				if b.Type == BlockToolUse {
					toolUseIDs[b.ID] = true
				}
			}
		}
	}

	for _, msg := range messages {
		if msg.Role == RoleUser {
			for _, b := range msg.Content {
				if b.Type == BlockToolResult {
					delete(toolUseIDs, b.ToolUseID)
				}
			}
		}
	}

	var orphaned []Block
	for id := range toolUseIDs {
		orphaned = append(orphaned, NewToolResultBlock(id, "Error: tool execution was interrupted", true))
	}
	return orphaned
}
