package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
)

// sseServer returns a test server that streams the given SSE lines.
func sseServer(lines []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range lines {
			fmt.Fprintf(w, "%s\n", line)
		}
	}))
}

// newCallerAgainst creates an AnthropicCaller pointed at the given test server URL.
func newCallerAgainst(serverURL string) *AnthropicCaller {
	c := NewAnthropicCaller("test-key", "claude-sonnet-4-6")
	// Override the API URL via a custom RoundTripper that rewrites the host.
	c.httpClient = &http.Client{
		Transport: &hostRewriter{base: serverURL, inner: http.DefaultTransport},
	}
	return c
}

// hostRewriter rewrites the request URL host to the test server.
type hostRewriter struct {
	base  string
	inner http.RoundTripper
}

func (h *hostRewriter) RoundTrip(r *http.Request) (*http.Response, error) {
	// Parse the base URL and replace the scheme+host+path prefix.
	// The test server handles all paths, so we just rewrite the host.
	r2 := r.Clone(r.Context())
	base := h.base
	// Strip trailing slash from base
	base = strings.TrimRight(base, "/")
	r2.URL, _ = r2.URL.Parse(base + r.URL.Path)
	return h.inner.RoundTrip(r2)
}

// collectEvents drains the stream channel and returns all events.
func collectEvents(ch <-chan StreamEvent) []StreamEvent {
	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestStream_TextResponse(t *testing.T) {
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}

	srv := sseServer(lines)
	defer srv.Close()

	caller := newCallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "claude-sonnet-4-6",
	}

	events := collectEvents(caller.Stream(context.Background(), params))

	// Should have: text_delta x2 + message_done
	var textDeltas []string
	var done *StreamEvent
	for i, e := range events {
		switch e.Type {
		case "text_delta":
			textDeltas = append(textDeltas, e.Text)
		case "message_done":
			done = &events[i]
		case "error":
			t.Fatalf("unexpected error event: %v", e.Err)
		}
	}

	if len(textDeltas) != 2 {
		t.Errorf("want 2 text_delta events, got %d", len(textDeltas))
	}
	if textDeltas[0] != "Hello" || textDeltas[1] != " world" {
		t.Errorf("unexpected text deltas: %v", textDeltas)
	}
	if done == nil {
		t.Fatal("no message_done event")
	}
	if done.Message == nil {
		t.Fatal("message_done has nil Message")
	}
	if done.Message.TextContent() != "Hello world" {
		t.Errorf("want 'Hello world', got %q", done.Message.TextContent())
	}
	if done.Message.StopReason != models.StopEndTurn {
		t.Errorf("want stop_reason=end_turn, got %q", done.Message.StopReason)
	}
	if done.Message.Usage == nil {
		t.Fatal("message has nil Usage")
	}
	if done.Message.Usage.InputTokens != 10 {
		t.Errorf("want input_tokens=10, got %d", done.Message.Usage.InputTokens)
	}
	if done.Message.Usage.OutputTokens != 5 {
		t.Errorf("want output_tokens=5, got %d", done.Message.Usage.OutputTokens)
	}
}

func TestStream_ToolUse(t *testing.T) {
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_02","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":20,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"Bash","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":" -la\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}

	srv := sseServer(lines)
	defer srv.Close()

	caller := newCallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("list files")},
		Model:    "claude-sonnet-4-6",
	}

	events := collectEvents(caller.Stream(context.Background(), params))

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "error" {
			t.Fatalf("unexpected error: %v", e.Err)
		}
		if e.Type == "message_done" {
			done = &events[i]
		}
	}

	if done == nil {
		t.Fatal("no message_done event")
	}
	if done.Message == nil {
		t.Fatal("message_done has nil Message")
	}

	toolBlocks := done.Message.ToolUseBlocks()
	if len(toolBlocks) != 1 {
		t.Fatalf("want 1 tool_use block, got %d", len(toolBlocks))
	}
	blk := toolBlocks[0]
	if blk.Name != "Bash" {
		t.Errorf("want tool name 'Bash', got %q", blk.Name)
	}
	if blk.ID != "toolu_01" {
		t.Errorf("want tool id 'toolu_01', got %q", blk.ID)
	}

	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(blk.Input, &input); err != nil {
		t.Fatalf("unmarshal tool input: %v", err)
	}
	if input.Command != "ls -la" {
		t.Errorf("want command 'ls -la', got %q", input.Command)
	}
	if done.Message.StopReason != models.StopToolUse {
		t.Errorf("want stop_reason=tool_use, got %q", done.Message.StopReason)
	}
}

func TestStream_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid api key"}}`))
	}))
	defer srv.Close()

	caller := newCallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
	}

	events := collectEvents(caller.Stream(context.Background(), params))

	if len(events) == 0 {
		t.Fatal("expected at least one event (error)")
	}
	last := events[len(events)-1]
	if last.Type != "error" {
		t.Errorf("want error event, got %q", last.Type)
	}
	if last.Err == nil {
		t.Error("error event has nil Err")
	}
	if !strings.Contains(last.Err.Error(), "invalid api key") {
		t.Errorf("want 'invalid api key' in error, got %q", last.Err.Error())
	}
}

func TestStream_MixedTextAndTool(t *testing.T) {
	lines := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_03","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"usage":{"input_tokens":30,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check."}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_02","name":"Read","input":{}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":\"/tmp/test\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}

	srv := sseServer(lines)
	defer srv.Close()

	caller := newCallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("read the file")},
		Model:    "claude-sonnet-4-6",
	}

	events := collectEvents(caller.Stream(context.Background(), params))

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
		t.Fatal("no message_done")
	}

	msg := done.Message
	if msg.TextContent() != "Let me check." {
		t.Errorf("want 'Let me check.', got %q", msg.TextContent())
	}
	if !msg.HasToolUse() {
		t.Error("expected tool use block")
	}
	toolBlocks := msg.ToolUseBlocks()
	if toolBlocks[0].Name != "Read" {
		t.Errorf("want 'Read', got %q", toolBlocks[0].Name)
	}
}

func TestParseSSEStream_IgnoresMalformed(t *testing.T) {
	lines := []string{
		`data: not-json`,
		`data: {"type":"message_start","message":{"id":"msg_04","model":"claude-sonnet-4-6","usage":{"input_tokens":5,"output_tokens":1}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`data: {"type":"message_stop"}`,
	}

	srv := sseServer(lines)
	defer srv.Close()

	caller := newCallerAgainst(srv.URL)
	events := collectEvents(caller.Stream(context.Background(), StreamParams{
		Messages: []*models.Message{models.NewUserMessage("test")},
	}))

	var done *StreamEvent
	for i, e := range events {
		if e.Type == "message_done" {
			done = &events[i]
		}
	}
	if done == nil {
		t.Fatal("no message_done")
	}
	if done.Message.TextContent() != "ok" {
		t.Errorf("want 'ok', got %q", done.Message.TextContent())
	}
}
