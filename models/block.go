package models

import "encoding/json"

// BlockType identifies the kind of content block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockThinking   BlockType = "thinking"
)

// Block is a single content block within a message.
type Block struct {
	Type BlockType `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// NewToolResultBlock creates a single tool_result content block.
func NewToolResultBlock(toolUseID string, content string, isError bool) Block {
	return Block{
		Type:      BlockToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}
