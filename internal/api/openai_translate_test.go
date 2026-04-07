package api

import (
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
)

func TestToOpenAIMessages_SystemPrompt(t *testing.T) {
	msgs := []*models.Message{models.NewUserMessage("hello")}
	out := toOpenAIMessages("You are helpful.", msgs)

	if len(out) != 2 {
		t.Fatalf("want 2 messages, got %d", len(out))
	}
	if out[0].Role != "system" || out[0].Content != "You are helpful." {
		t.Errorf("system message = %+v", out[0])
	}
	if out[1].Role != "user" || out[1].Content != "hello" {
		t.Errorf("user message = %+v", out[1])
	}
}

func TestToOpenAIMessages_NoSystem(t *testing.T) {
	msgs := []*models.Message{models.NewUserMessage("hi")}
	out := toOpenAIMessages("", msgs)

	if len(out) != 1 {
		t.Fatalf("want 1 message (no system), got %d", len(out))
	}
	if out[0].Role != "user" {
		t.Errorf("want role=user, got %q", out[0].Role)
	}
}

func TestToOpenAIMessages_UserTextOnly(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleUser,
		Content: []models.Block{
			{Type: models.BlockText, Text: "part1 "},
			{Type: models.BlockText, Text: "part2"},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	if out[0].Content != "part1 part2" {
		t.Errorf("want 'part1 part2', got %q", out[0].Content)
	}
}

func TestToOpenAIMessages_ToolResult(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleUser,
		Content: []models.Block{
			{Type: models.BlockToolResult, ToolUseID: "call_1", Content: "file1.go\nfile2.go"},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	if out[0].Role != "tool" {
		t.Errorf("want role=tool, got %q", out[0].Role)
	}
	if out[0].ToolCallID != "call_1" {
		t.Errorf("want tool_call_id=call_1, got %q", out[0].ToolCallID)
	}
	if out[0].Content != "file1.go\nfile2.go" {
		t.Errorf("want tool content, got %q", out[0].Content)
	}
}

func TestToOpenAIMessages_MixedToolResultAndText(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleUser,
		Content: []models.Block{
			{Type: models.BlockToolResult, ToolUseID: "call_1", Content: "ok"},
			{Type: models.BlockText, Text: "please continue"},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	// Should emit: tool message first, then user text.
	if len(out) != 2 {
		t.Fatalf("want 2 messages, got %d", len(out))
	}
	if out[0].Role != "tool" || out[0].ToolCallID != "call_1" {
		t.Errorf("first message should be tool, got %+v", out[0])
	}
	if out[1].Role != "user" || out[1].Content != "please continue" {
		t.Errorf("second message should be user text, got %+v", out[1])
	}
}

func TestToOpenAIMessages_AssistantTextOnly(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockText, Text: "Hello, how can I help?"},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	if out[0].Role != "assistant" {
		t.Errorf("want role=assistant, got %q", out[0].Role)
	}
	if out[0].Content != "Hello, how can I help?" {
		t.Errorf("want content, got %q", out[0].Content)
	}
	if len(out[0].ToolCalls) != 0 {
		t.Errorf("want no tool_calls, got %d", len(out[0].ToolCalls))
	}
}

func TestToOpenAIMessages_AssistantToolUse(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockText, Text: "Let me check."},
			{
				Type:  models.BlockToolUse,
				ID:    "call_1",
				Name:  "bash",
				Input: json.RawMessage(`{"command":"ls"}`),
			},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	m := out[0]
	if m.Content != "Let me check." {
		t.Errorf("want content 'Let me check.', got %q", m.Content)
	}
	if len(m.ToolCalls) != 1 {
		t.Fatalf("want 1 tool_call, got %d", len(m.ToolCalls))
	}
	tc := m.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("want id=call_1, got %q", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("want type=function, got %q", tc.Type)
	}
	if tc.Function.Name != "bash" {
		t.Errorf("want name=bash, got %q", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"command":"ls"}` {
		t.Errorf("want arguments, got %q", tc.Function.Arguments)
	}
}

func TestToOpenAIMessages_AssistantToolUseEmptyInput(t *testing.T) {
	msg := &models.Message{
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: "call_2", Name: "status", Input: nil},
		},
	}
	out := toOpenAIMessages("", []*models.Message{msg})

	tc := out[0].ToolCalls[0]
	if tc.Function.Arguments != "{}" {
		t.Errorf("empty input should become '{}', got %q", tc.Function.Arguments)
	}
}

func TestToOpenAIMessages_FullConversation(t *testing.T) {
	msgs := []*models.Message{
		{Role: models.RoleUser, Content: []models.Block{
			{Type: models.BlockText, Text: "List files"},
		}},
		{Role: models.RoleAssistant, Content: []models.Block{
			{Type: models.BlockText, Text: "I'll list the files."},
			{Type: models.BlockToolUse, ID: "tu_1", Name: "bash",
				Input: json.RawMessage(`{"command":"ls"}`)},
		}},
		{Role: models.RoleUser, Content: []models.Block{
			{Type: models.BlockToolResult, ToolUseID: "tu_1", Content: "file1.go\nfile2.go"},
		}},
	}

	out := toOpenAIMessages("You are helpful.", msgs)

	// system + user + assistant + tool = 4
	if len(out) != 4 {
		t.Fatalf("want 4 messages, got %d", len(out))
	}
	if out[0].Role != "system" {
		t.Errorf("[0] want system, got %q", out[0].Role)
	}
	if out[1].Role != "user" {
		t.Errorf("[1] want user, got %q", out[1].Role)
	}
	if out[2].Role != "assistant" {
		t.Errorf("[2] want assistant, got %q", out[2].Role)
	}
	if out[3].Role != "tool" {
		t.Errorf("[3] want tool, got %q", out[3].Role)
	}
	if out[3].ToolCallID != "tu_1" {
		t.Errorf("[3] want tool_call_id=tu_1, got %q", out[3].ToolCallID)
	}
}

func TestToOpenAITools(t *testing.T) {
	tools := []ToolSchema{
		{
			Name:        "bash",
			Description: "Execute a shell command",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{"type": "string"},
				},
				"required": []string{"command"},
			},
		},
	}

	out := toOpenAITools(tools)

	if len(out) != 1 {
		t.Fatalf("want 1 tool, got %d", len(out))
	}
	if out[0].Type != "function" {
		t.Errorf("want type=function, got %q", out[0].Type)
	}
	if out[0].Function.Name != "bash" {
		t.Errorf("want name=bash, got %q", out[0].Function.Name)
	}
	if out[0].Function.Description != "Execute a shell command" {
		t.Errorf("want description, got %q", out[0].Function.Description)
	}
	// Parameters should be the same JSON Schema as input_schema.
	b, _ := json.Marshal(out[0].Function.Parameters)
	if !json.Valid(b) {
		t.Error("parameters should be valid JSON")
	}
}

func TestFromOpenAIToolCall(t *testing.T) {
	tc := openaiToolCall{
		ID:   "call_abc",
		Type: "function",
		Function: openaiCallFunc{
			Name:      "read_file",
			Arguments: `{"path":"/tmp/test"}`,
		},
	}

	block := fromOpenAIToolCall(tc)

	if block.Type != models.BlockToolUse {
		t.Errorf("want type=tool_use, got %q", block.Type)
	}
	if block.ID != "call_abc" {
		t.Errorf("want id=call_abc, got %q", block.ID)
	}
	if block.Name != "read_file" {
		t.Errorf("want name=read_file, got %q", block.Name)
	}

	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(block.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input.Path != "/tmp/test" {
		t.Errorf("want path=/tmp/test, got %q", input.Path)
	}
}

func TestFromOpenAIToolCall_EmptyArguments(t *testing.T) {
	tc := openaiToolCall{
		ID:       "call_xyz",
		Type:     "function",
		Function: openaiCallFunc{Name: "status", Arguments: ""},
	}

	block := fromOpenAIToolCall(tc)
	if string(block.Input) != "{}" {
		t.Errorf("empty arguments should become '{}', got %q", string(block.Input))
	}
}
