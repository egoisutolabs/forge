package models

import (
	"encoding/json"
	"testing"
)

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("hello")

	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if msg.ID == "" {
		t.Error("expected non-empty ID")
	}
	if msg.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 block, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != BlockText {
		t.Errorf("expected block type %q, got %q", BlockText, msg.Content[0].Type)
	}
	if msg.Content[0].Text != "hello" {
		t.Errorf("expected text %q, got %q", "hello", msg.Content[0].Text)
	}
}

func TestNewToolResultMessage(t *testing.T) {
	results := []Block{
		NewToolResultBlock("tool_1", "output one", false),
		NewToolResultBlock("tool_2", "error happened", true),
	}
	msg := NewToolResultMessage(results)

	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].IsError {
		t.Error("expected first block not error")
	}
	if !msg.Content[1].IsError {
		t.Error("expected second block is error")
	}
}

func TestMessage_ToolUseBlocks(t *testing.T) {
	msg := &Message{
		Role: RoleAssistant,
		Content: []Block{
			{Type: BlockText, Text: "I'll check that"},
			{Type: BlockToolUse, ID: "t1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
			{Type: BlockToolUse, ID: "t2", Name: "Read", Input: json.RawMessage(`{"file_path":"main.go"}`)},
		},
	}

	blocks := msg.ToolUseBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected 2 tool_use blocks, got %d", len(blocks))
	}
	if blocks[0].Name != "Bash" {
		t.Errorf("expected first tool %q, got %q", "Bash", blocks[0].Name)
	}
}

func TestMessage_TextContent(t *testing.T) {
	msg := &Message{
		Content: []Block{
			{Type: BlockText, Text: "hello "},
			{Type: BlockToolUse, Name: "Bash"},
			{Type: BlockText, Text: "world"},
		},
	}
	if got := msg.TextContent(); got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

func TestMessage_TextContent_ManyBlocks(t *testing.T) {
	// Verify strings.Builder produces correct output with many text blocks.
	blocks := make([]Block, 100)
	for i := range blocks {
		blocks[i] = Block{Type: BlockText, Text: "x"}
	}
	msg := &Message{Content: blocks}
	got := msg.TextContent()
	if len(got) != 100 {
		t.Errorf("expected 100 chars, got %d", len(got))
	}
	for _, c := range got {
		if c != 'x' {
			t.Fatalf("unexpected char %q", c)
		}
	}
}

func TestMessage_TextContent_SingleBlock(t *testing.T) {
	msg := &Message{Content: []Block{{Type: BlockText, Text: "only"}}}
	if got := msg.TextContent(); got != "only" {
		t.Errorf("expected %q, got %q", "only", got)
	}
}

func TestMessage_TextContent_Empty(t *testing.T) {
	msg := &Message{}
	if got := msg.TextContent(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestMessage_HasToolUse(t *testing.T) {
	tests := []struct {
		name   string
		blocks []Block
		want   bool
	}{
		{"no blocks", nil, false},
		{"text only", []Block{{Type: BlockText}}, false},
		{"has tool_use", []Block{{Type: BlockText}, {Type: BlockToolUse}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Content: tt.blocks}
			if got := msg.HasToolUse(); got != tt.want {
				t.Errorf("HasToolUse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_JSON_RoundTrip(t *testing.T) {
	msg := NewUserMessage("test message")
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Role != msg.Role {
		t.Errorf("role: got %q, want %q", decoded.Role, msg.Role)
	}
}

func TestNormalizeForAPI_Empty(t *testing.T) {
	if result := NormalizeForAPI(nil); result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestNormalizeForAPI_MergesAdjacentSameRole(t *testing.T) {
	msgs := []*Message{
		{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "a"}}},
		{Role: RoleUser, Content: []Block{{Type: BlockText, Text: "b"}}},
		{Role: RoleAssistant, Content: []Block{{Type: BlockText, Text: "reply"}}},
	}
	result := NormalizeForAPI(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages after merge, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Errorf("expected merged message to have 2 blocks, got %d", len(result[0].Content))
	}
}

func TestEnsureToolResults_FindsOrphans(t *testing.T) {
	msgs := []*Message{
		{Role: RoleAssistant, Content: []Block{
			{Type: BlockToolUse, ID: "t1", Name: "Bash"},
			{Type: BlockToolUse, ID: "t2", Name: "Grep"},
		}},
		{Role: RoleUser, Content: []Block{
			{Type: BlockToolResult, ToolUseID: "t1", Content: "done"},
		}},
	}
	orphaned := EnsureToolResults(msgs)
	if len(orphaned) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphaned))
	}
	if orphaned[0].ToolUseID != "t2" {
		t.Errorf("expected orphan for t2, got %q", orphaned[0].ToolUseID)
	}
}

func TestUsage_TotalTokens(t *testing.T) {
	u := &Usage{InputTokens: 100, OutputTokens: 50}
	if u.TotalTokens() != 150 {
		t.Errorf("expected 150, got %d", u.TotalTokens())
	}
}

func TestNewToolResultBlock(t *testing.T) {
	b := NewToolResultBlock("id_123", "output", false)
	if b.Type != BlockToolResult {
		t.Errorf("expected type %q, got %q", BlockToolResult, b.Type)
	}
	if b.ToolUseID != "id_123" {
		t.Errorf("expected tool_use_id %q, got %q", "id_123", b.ToolUseID)
	}
	if b.IsError {
		t.Error("expected is_error=false")
	}
}
