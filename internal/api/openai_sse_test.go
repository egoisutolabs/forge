package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
)

// parseSSELines is a helper that feeds SSE lines into the OpenAI parser.
func parseSSELines(lines []string) []StreamEvent {
	input := strings.Join(lines, "\n")
	ch := make(chan StreamEvent, 32)
	go func() {
		defer close(ch)
		parseOpenAISSEStream(strings.NewReader(input), ch)
	}()
	return collectEvents(ch)
}

func TestOpenAISSE_TextDelta(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var textDeltas []string
	var done *StreamEvent
	for i, e := range events {
		switch e.Type {
		case "text_delta":
			textDeltas = append(textDeltas, e.Text)
		case "message_done":
			done = &events[i]
		case "error":
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	if len(textDeltas) != 2 {
		t.Errorf("want 2 text_delta events, got %d", len(textDeltas))
	}
	if textDeltas[0] != "Hello" || textDeltas[1] != " world" {
		t.Errorf("unexpected text deltas: %v", textDeltas)
	}

	if done == nil || done.Message == nil {
		t.Fatal("no message_done event")
	}
	msg := done.Message
	if msg.TextContent() != "Hello world" {
		t.Errorf("want 'Hello world', got %q", msg.TextContent())
	}
	if msg.StopReason != models.StopEndTurn {
		t.Errorf("want stop=end_turn, got %q", msg.StopReason)
	}
	if msg.ID != "chatcmpl-1" {
		t.Errorf("want id=chatcmpl-1, got %q", msg.ID)
	}
	if msg.Usage == nil {
		t.Fatal("want usage, got nil")
	}
	if msg.Usage.InputTokens != 10 {
		t.Errorf("want input_tokens=10, got %d", msg.Usage.InputTokens)
	}
	if msg.Usage.OutputTokens != 5 {
		t.Errorf("want output_tokens=5, got %d", msg.Usage.OutputTokens)
	}
}

func TestOpenAISSE_ToolCallFragments(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-2","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-2","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-2","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\"ls\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-2","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl-2","choices":[],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "error" {
			t.Fatalf("unexpected error: %v", e.Err)
		}
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil || done.Message == nil {
		t.Fatal("no message_done event")
	}
	msg := done.Message

	if msg.StopReason != models.StopToolUse {
		t.Errorf("want stop=tool_use, got %q", msg.StopReason)
	}

	toolBlocks := msg.ToolUseBlocks()
	if len(toolBlocks) != 1 {
		t.Fatalf("want 1 tool_use block, got %d", len(toolBlocks))
	}

	blk := toolBlocks[0]
	if blk.ID != "call_1" {
		t.Errorf("want id=call_1, got %q", blk.ID)
	}
	if blk.Name != "bash" {
		t.Errorf("want name=bash, got %q", blk.Name)
	}

	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(blk.Input, &input); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}
	if input.Command != "ls" {
		t.Errorf("want command=ls, got %q", input.Command)
	}
}

func TestOpenAISSE_TextPlusToolCall(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-3","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Let me check."},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-3","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"read","arguments":"{\"path\":\"/tmp\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-3","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	var gotTextDelta bool
	for i, e := range events {
		if e.Type == "text_delta" {
			gotTextDelta = true
		}
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if !gotTextDelta {
		t.Error("expected at least one text_delta event")
	}
	if done == nil || done.Message == nil {
		t.Fatal("no message_done")
	}
	msg := done.Message
	if msg.TextContent() != "Let me check." {
		t.Errorf("want 'Let me check.', got %q", msg.TextContent())
	}
	if !msg.HasToolUse() {
		t.Error("expected tool use")
	}
}

func TestOpenAISSE_MultipleToolCalls(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-4","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-4","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"read","arguments":"{\"path\":\"/tmp\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-4","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil || done.Message == nil {
		t.Fatal("no message_done")
	}
	toolBlocks := done.Message.ToolUseBlocks()
	if len(toolBlocks) != 2 {
		t.Fatalf("want 2 tool_use blocks, got %d", len(toolBlocks))
	}
	if toolBlocks[0].Name != "bash" || toolBlocks[1].Name != "read" {
		t.Errorf("tool names: %q, %q", toolBlocks[0].Name, toolBlocks[1].Name)
	}
}

func TestOpenAISSE_FinishReasonLength(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-5","model":"gpt-4","choices":[{"index":0,"delta":{"content":"truncated"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-5","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil || done.Message == nil {
		t.Fatal("no message_done")
	}
	if done.Message.StopReason != models.StopMaxTokens {
		t.Errorf("want stop=max_tokens, got %q", done.Message.StopReason)
	}
}

func TestOpenAISSE_UsageWithCachedTokens(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-6","model":"gpt-4","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-6","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-6","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":50}}}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil || done.Message == nil || done.Message.Usage == nil {
		t.Fatal("no message with usage")
	}
	u := done.Message.Usage
	if u.InputTokens != 100 {
		t.Errorf("want input_tokens=100, got %d", u.InputTokens)
	}
	if u.CacheRead != 50 {
		t.Errorf("want cache_read=50 (from cached_tokens), got %d", u.CacheRead)
	}
}

func TestOpenAISSE_MalformedChunkIgnored(t *testing.T) {
	lines := []string{
		`data: not-valid-json`,
		`data: {"id":"chatcmpl-7","model":"gpt-4","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-7","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil || done.Message == nil {
		t.Fatal("no message_done")
	}
	if done.Message.TextContent() != "ok" {
		t.Errorf("want 'ok', got %q", done.Message.TextContent())
	}
}

func TestOpenAISSE_EmptyStream(t *testing.T) {
	lines := []string{
		`data: [DONE]`,
	}

	events := parseSSELines(lines)

	// No finish_reason means no message_done should be emitted.
	for _, e := range events {
		if e.Type == "message_done" {
			t.Error("did not expect message_done for empty stream")
		}
	}
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		input string
		want  models.StopReason
	}{
		{"stop", models.StopEndTurn},
		{"tool_calls", models.StopToolUse},
		{"length", models.StopMaxTokens},
		{"content_filter", models.StopEndTurn},
		{"unknown", models.StopEndTurn},
	}

	for _, tt := range tests {
		got := mapFinishReason(tt.input)
		if got != tt.want {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFromOpenAIUsage(t *testing.T) {
	ou := &openaiUsage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}

	u := fromOpenAIUsage(ou)

	if u.InputTokens != 1000 {
		t.Errorf("want input=1000, got %d", u.InputTokens)
	}
	if u.OutputTokens != 500 {
		t.Errorf("want output=500, got %d", u.OutputTokens)
	}
	if u.CacheRead != 0 {
		t.Errorf("want cache_read=0, got %d", u.CacheRead)
	}
}

func TestFromOpenAIUsage_Nil(t *testing.T) {
	if fromOpenAIUsage(nil) != nil {
		t.Error("nil usage should return nil")
	}
}
