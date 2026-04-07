package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/config"
	"github.com/egoisutolabs/forge/internal/models"
)

// openaiSSEServer returns a test server that streams OpenAI-format SSE lines.
func openaiSSEServer(lines []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range lines {
			fmt.Fprintf(w, "%s\n", line)
		}
	}))
}

// newOpenAICallerAgainst creates an OpenAICaller pointed at a test server.
func newOpenAICallerAgainst(serverURL string) *OpenAICaller {
	p := &config.Provider{
		Name:    "test",
		BaseURL: serverURL,
		APIKey:  "test-key",
		Models:  []string{"test-model"},
	}
	return NewOpenAICaller(p)
}

func TestOpenAICaller_TextResponse(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-t1","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-t1","model":"test-model","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-t1","model":"test-model","choices":[{"index":0,"delta":{"content":" there"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-t1","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-t1","choices":[],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}`,
		`data: [DONE]`,
	}

	srv := openaiSSEServer(lines)
	defer srv.Close()

	caller := newOpenAICallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
	}

	events := collectEvents(caller.Stream(context.Background(), params))

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
	if done == nil || done.Message == nil {
		t.Fatal("no message_done")
	}
	if done.Message.TextContent() != "Hello there" {
		t.Errorf("want 'Hello there', got %q", done.Message.TextContent())
	}
	if done.Message.StopReason != models.StopEndTurn {
		t.Errorf("want stop=end_turn, got %q", done.Message.StopReason)
	}
	if done.Message.Usage == nil {
		t.Fatal("want usage")
	}
	if done.Message.Usage.InputTokens != 15 {
		t.Errorf("want input_tokens=15, got %d", done.Message.Usage.InputTokens)
	}
}

func TestOpenAICaller_ToolCall(t *testing.T) {
	lines := []string{
		`data: {"id":"chatcmpl-t2","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"bash","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-t2","model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"command\":\"ls\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-t2","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl-t2","choices":[],"usage":{"prompt_tokens":25,"completion_tokens":15,"total_tokens":40}}`,
		`data: [DONE]`,
	}

	srv := openaiSSEServer(lines)
	defer srv.Close()

	caller := newOpenAICallerAgainst(srv.URL)
	params := StreamParams{
		Messages: []*models.Message{models.NewUserMessage("list files")},
		Tools: []ToolSchema{{
			Name:        "bash",
			Description: "Execute a shell command",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
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
	if msg.StopReason != models.StopToolUse {
		t.Errorf("want stop=tool_use, got %q", msg.StopReason)
	}
	toolBlocks := msg.ToolUseBlocks()
	if len(toolBlocks) != 1 {
		t.Fatalf("want 1 tool_use block, got %d", len(toolBlocks))
	}
	if toolBlocks[0].Name != "bash" {
		t.Errorf("want tool name=bash, got %q", toolBlocks[0].Name)
	}

	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(toolBlocks[0].Input, &input); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if input.Command != "ls" {
		t.Errorf("want command=ls, got %q", input.Command)
	}
}

func TestOpenAICaller_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key","type":"auth_error"}}`))
	}))
	defer srv.Close()

	caller := newOpenAICallerAgainst(srv.URL)
	events := collectEvents(caller.Stream(context.Background(), StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
	}))

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	last := events[len(events)-1]
	if last.Type != "error" {
		t.Errorf("want error event, got %q", last.Type)
	}
	if !strings.Contains(last.Err.Error(), "invalid api key") {
		t.Errorf("want 'invalid api key' in error, got %q", last.Err.Error())
	}
}

func TestOpenAICaller_RequestFormat(t *testing.T) {
	// Verify the request body format sent to the server.
	var capturedBody []byte
	var capturedHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `data: {"id":"chatcmpl-x","model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`)
		fmt.Fprintln(w, `data: {"id":"chatcmpl-x","model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	p := &config.Provider{
		Name:    "test",
		BaseURL: srv.URL,
		APIKey:  "sk-test-123",
		Models:  []string{"my-model"},
		Headers: map[string]string{"X-Custom": "val"},
	}
	caller := NewOpenAICaller(p)

	params := StreamParams{
		Messages:     []*models.Message{models.NewUserMessage("hello")},
		SystemPrompt: "be helpful",
		Model:        "my-model",
		MaxTokens:    4096,
		Tools: []ToolSchema{{
			Name:        "bash",
			Description: "run shell",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
	}

	events := collectEvents(caller.Stream(context.Background(), params))
	// Verify we got a successful response.
	for _, e := range events {
		if e.Type == "error" {
			t.Fatalf("unexpected error: %v", e.Err)
		}
	}

	// Check headers.
	if auth := capturedHeaders.Get("Authorization"); auth != "Bearer sk-test-123" {
		t.Errorf("want Bearer auth, got %q", auth)
	}
	if ct := capturedHeaders.Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %q", ct)
	}
	if custom := capturedHeaders.Get("X-Custom"); custom != "val" {
		t.Errorf("want X-Custom=val, got %q", custom)
	}

	// Check body.
	var body openaiRequest
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body.Model != "my-model" {
		t.Errorf("want model=my-model, got %q", body.Model)
	}
	if !body.Stream {
		t.Error("want stream=true")
	}
	if body.StreamOptions == nil || !body.StreamOptions.IncludeUsage {
		t.Error("want stream_options.include_usage=true")
	}
	if body.MaxTokens != 4096 {
		t.Errorf("want max_tokens=4096, got %d", body.MaxTokens)
	}
	// Should have: system, user, = 2 messages.
	if len(body.Messages) != 2 {
		t.Errorf("want 2 messages, got %d", len(body.Messages))
	}
	if body.Messages[0].Role != "system" {
		t.Errorf("want system message first, got %q", body.Messages[0].Role)
	}
	if len(body.Tools) != 1 {
		t.Errorf("want 1 tool, got %d", len(body.Tools))
	}
}

func TestOpenAICaller_DefaultModel(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `data: {"id":"x","model":"m","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`)
		fmt.Fprintln(w, `data: [DONE]`)
	}))
	defer srv.Close()

	p := &config.Provider{
		Name:    "test",
		BaseURL: srv.URL,
		Models:  []string{"default-model"},
	}
	caller := NewOpenAICaller(p)

	// Don't set Model in params — should use the provider's default.
	collectEvents(caller.Stream(context.Background(), StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
	}))

	var body struct {
		Model string `json:"model"`
	}
	json.Unmarshal(capturedBody, &body)
	if body.Model != "default-model" {
		t.Errorf("want model=default-model, got %q", body.Model)
	}
}
