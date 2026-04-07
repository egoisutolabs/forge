package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/config"
	"github.com/egoisutolabs/forge/internal/models"
)

// TestOpenAI_ImplementsCaller is a compile-time check that OpenAICaller satisfies api.Caller.
func TestOpenAI_ImplementsCaller(t *testing.T) {
	p := &config.Provider{
		Name:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  "test-key",
		Models:  []string{"test-model"},
	}
	var _ api.Caller = api.NewOpenAICaller(p)
}

// TestOpenAI_RequestBodyShape verifies the JSON body sent to /chat/completions has the
// correct OpenAI shape: model, messages, tools, stream, stream_options.
func TestOpenAI_RequestBodyShape(t *testing.T) {
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body

		// Verify endpoint.
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}

		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Authorization = %q, want Bearer test-api-key", auth)
		}

		// Return a minimal SSE response.
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	caller := api.NewOpenAICaller(&config.Provider{
		Name:    "test",
		BaseURL: srv.URL,
		APIKey:  "test-api-key",
		Models:  []string{"test-model"},
	})

	toolSchema := api.ToolSchema{
		Name:        "read_file",
		Description: "Read a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{"type": "string"},
			},
		},
	}

	ch := caller.Stream(context.Background(), api.StreamParams{
		Messages:     []*models.Message{models.NewUserMessage("hello")},
		SystemPrompt: "You are helpful.",
		Tools:        []api.ToolSchema{toolSchema},
		Model:        "test-model",
		MaxTokens:    1024,
	})

	// Drain the channel.
	for range ch {
	}

	// Parse and verify the captured request body.
	var req map[string]json.RawMessage
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}

	// Required fields.
	requiredFields := []string{"model", "messages", "stream", "stream_options"}
	for _, field := range requiredFields {
		if _, ok := req[field]; !ok {
			t.Errorf("request body missing field %q", field)
		}
	}

	// Verify model.
	var model string
	json.Unmarshal(req["model"], &model)
	if model != "test-model" {
		t.Errorf("model = %q, want test-model", model)
	}

	// Verify stream = true.
	var stream bool
	json.Unmarshal(req["stream"], &stream)
	if !stream {
		t.Error("stream should be true")
	}

	// Verify stream_options has include_usage.
	var streamOpts map[string]bool
	json.Unmarshal(req["stream_options"], &streamOpts)
	if !streamOpts["include_usage"] {
		t.Error("stream_options.include_usage should be true")
	}

	// Verify tools present.
	if _, ok := req["tools"]; !ok {
		t.Error("tools should be present when provided")
	}

	// Verify messages includes system + user.
	var messages []map[string]any
	json.Unmarshal(req["messages"], &messages)
	if len(messages) < 2 {
		t.Fatalf("messages count = %d, want >= 2 (system + user)", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Errorf("messages[0].role = %v, want system", messages[0]["role"])
	}
	if messages[1]["role"] != "user" {
		t.Errorf("messages[1].role = %v, want user", messages[1]["role"])
	}
}

// TestOpenAI_SSEParsing verifies that SSE response parsing produces a valid models.Message
// with correct text content, role, and stop reason.
func TestOpenAI_SSEParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Multi-chunk text streaming.
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-abc\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-abc\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world!\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-abc\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	caller := api.NewOpenAICaller(&config.Provider{
		Name:    "test",
		BaseURL: srv.URL,
		APIKey:  "key",
		Models:  []string{"gpt-4"},
	})

	ch := caller.Stream(context.Background(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "gpt-4",
	})

	var textDeltas []string
	var finalMsg *models.Message
	for ev := range ch {
		switch ev.Type {
		case "text_delta":
			textDeltas = append(textDeltas, ev.Text)
		case "message_done":
			finalMsg = ev.Message
		case "error":
			t.Fatalf("unexpected error: %v", ev.Err)
		}
	}

	// Verify text deltas.
	combined := strings.Join(textDeltas, "")
	if combined != "Hello world!" {
		t.Errorf("text deltas = %q, want 'Hello world!'", combined)
	}

	// Verify final message.
	if finalMsg == nil {
		t.Fatal("no message_done event received")
	}
	if finalMsg.Role != models.RoleAssistant {
		t.Errorf("Role = %q, want assistant", finalMsg.Role)
	}
	if finalMsg.StopReason != models.StopEndTurn {
		t.Errorf("StopReason = %q, want end_turn", finalMsg.StopReason)
	}
	if finalMsg.TextContent() != "Hello world!" {
		t.Errorf("TextContent() = %q, want 'Hello world!'", finalMsg.TextContent())
	}

	// Verify usage was parsed.
	if finalMsg.Usage == nil {
		t.Fatal("Usage is nil")
	}
	if finalMsg.Usage.InputTokens != 12 {
		t.Errorf("InputTokens = %d, want 12", finalMsg.Usage.InputTokens)
	}
	if finalMsg.Usage.OutputTokens != 3 {
		t.Errorf("OutputTokens = %d, want 3", finalMsg.Usage.OutputTokens)
	}
}

// TestOpenAI_SSEToolCall verifies that tool_calls in SSE response are parsed correctly.
func TestOpenAI_SSEToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"msg1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"msg1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/test\"}"}}]},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"msg1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	caller := api.NewOpenAICaller(&config.Provider{
		BaseURL: srv.URL,
		APIKey:  "key",
		Models:  []string{"gpt-4"},
	})

	ch := caller.Stream(context.Background(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("read /tmp/test")},
		Model:    "gpt-4",
	})

	var finalMsg *models.Message
	for ev := range ch {
		if ev.Type == "message_done" {
			finalMsg = ev.Message
		}
		if ev.Type == "error" {
			t.Fatalf("error: %v", ev.Err)
		}
	}

	if finalMsg == nil {
		t.Fatal("no message_done")
	}
	if finalMsg.StopReason != models.StopToolUse {
		t.Errorf("StopReason = %q, want tool_use", finalMsg.StopReason)
	}
	if !finalMsg.HasToolUse() {
		t.Fatal("expected HasToolUse() = true")
	}

	blocks := finalMsg.ToolUseBlocks()
	if len(blocks) != 1 {
		t.Fatalf("ToolUseBlocks count = %d, want 1", len(blocks))
	}
	if blocks[0].Name != "read_file" {
		t.Errorf("tool name = %q, want read_file", blocks[0].Name)
	}
	if blocks[0].ID != "call_1" {
		t.Errorf("tool ID = %q, want call_1", blocks[0].ID)
	}
}

// TestOpenAI_FactoryRouting verifies NewCaller routes to OpenAICaller for non-Anthropic providers.
func TestOpenAI_FactoryRouting(t *testing.T) {
	// Non-Anthropic provider should get OpenAICaller.
	caller := api.NewCaller(&config.Provider{
		Name:    "openrouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Models:  []string{"test"},
	})
	if _, ok := caller.(*api.OpenAICaller); !ok {
		t.Errorf("NewCaller for openrouter returned %T, want *api.OpenAICaller", caller)
	}

	// Anthropic provider should get AnthropicCaller.
	anthCaller := api.NewCaller(&config.Provider{
		Name:   "anthropic",
		Models: []string{"claude-sonnet-4-20250514"},
	})
	if _, ok := anthCaller.(*api.AnthropicCaller); !ok {
		t.Errorf("NewCaller for anthropic returned %T, want *api.AnthropicCaller", anthCaller)
	}
}
