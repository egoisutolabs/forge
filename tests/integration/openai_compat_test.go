package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/config"
	"github.com/egoisutolabs/forge/internal/models"
)

// =============================================================================
// Config Loading & Env Expansion
// =============================================================================

func TestConfigLoadEnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-test-key-12345")
	t.Setenv("TEST_BASE_URL", "https://api.example.com/v1")

	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yaml := `
default_model: gpt-4
providers:
  - name: openai
    api_key: "${TEST_OPENAI_KEY}"
    base_url: "${TEST_BASE_URL}"
    models:
      - gpt-4
      - gpt-3.5-turbo
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4")
	}
	if len(cfg.Providers) == 0 {
		t.Fatal("expected at least one provider")
	}
	p := cfg.Providers[0]
	if p.APIKey != "sk-test-key-12345" {
		t.Errorf("APIKey = %q, want %q", p.APIKey, "sk-test-key-12345")
	}
	if p.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q, want %q", p.BaseURL, "https://api.example.com/v1")
	}
}

func TestConfigLoadUnsetEnvVar(t *testing.T) {
	os.Unsetenv("UNSET_KEY_THAT_SHOULD_NOT_EXIST")

	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yaml := `
default_model: test
providers:
  - name: provider
    api_key: "${UNSET_KEY_THAT_SHOULD_NOT_EXIST}"
    models:
      - test
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Providers[0].APIKey != "" {
		t.Errorf("expected empty APIKey for unset env var, got %q", cfg.Providers[0].APIKey)
	}
}

// =============================================================================
// Config Merging (Global + Project)
// =============================================================================

func TestConfigMergeGlobalAndProject(t *testing.T) {
	// Create a global config in a temporary home directory.
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	globalForge := filepath.Join(homeDir, ".forge")
	if err := os.MkdirAll(globalForge, 0o755); err != nil {
		t.Fatal(err)
	}

	globalYAML := `
default_model: claude-sonnet-4-6
providers:
  - name: anthropic
    api_key: "global-key"
    models:
      - claude-sonnet-4-6
  - name: openai
    api_key: "global-openai-key"
    base_url: "https://api.openai.com/v1"
    models:
      - gpt-4
model_costs:
  gpt-4:
    input: 10.0
    output: 30.0
`
	if err := os.WriteFile(filepath.Join(globalForge, "config.yaml"), []byte(globalYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a project config that overrides specific values.
	projDir := t.TempDir()
	projForge := filepath.Join(projDir, ".forge")
	if err := os.MkdirAll(projForge, 0o755); err != nil {
		t.Fatal(err)
	}

	projYAML := `
default_model: gpt-4
providers:
  - name: openai
    api_key: "project-openai-key"
    base_url: "https://custom.openai.com/v1"
    models:
      - gpt-4
      - gpt-4-turbo
model_costs:
  gpt-4:
    input: 5.0
    output: 15.0
  custom-model:
    input: 1.0
    output: 2.0
`
	if err := os.WriteFile(filepath.Join(projForge, "config.yaml"), []byte(projYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(projDir)
	if err != nil {
		t.Fatal(err)
	}

	// Project overrides default model.
	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4")
	}

	// Project overrides OpenAI provider.
	var openai *config.Provider
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "openai" {
			openai = &cfg.Providers[i]
			break
		}
	}
	if openai == nil {
		t.Fatal("openai provider not found in merged config")
	}
	if openai.APIKey != "project-openai-key" {
		t.Errorf("OpenAI APIKey = %q, want %q", openai.APIKey, "project-openai-key")
	}
	if openai.BaseURL != "https://custom.openai.com/v1" {
		t.Errorf("OpenAI BaseURL = %q, want %q", openai.BaseURL, "https://custom.openai.com/v1")
	}
	if len(openai.Models) != 2 {
		t.Errorf("OpenAI Models count = %d, want 2", len(openai.Models))
	}

	// Global anthropic provider still present.
	var anthropic *config.Provider
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "anthropic" {
			anthropic = &cfg.Providers[i]
			break
		}
	}
	if anthropic == nil {
		t.Fatal("anthropic provider should be preserved from global config")
	}

	// Model costs: project overrides gpt-4, adds custom-model.
	if cost, ok := cfg.ModelCosts["gpt-4"]; !ok || cost.Input != 5.0 || cost.Output != 15.0 {
		t.Errorf("gpt-4 cost = %+v, want Input=5.0, Output=15.0", cfg.ModelCosts["gpt-4"])
	}
	if cost, ok := cfg.ModelCosts["custom-model"]; !ok || cost.Input != 1.0 {
		t.Errorf("custom-model cost = %+v, want Input=1.0", cost)
	}
}

func TestConfigLoadEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Should return a zero config, not an error.
	if cfg.DefaultModel != "" {
		t.Errorf("expected empty DefaultModel, got %q", cfg.DefaultModel)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected no providers, got %d", len(cfg.Providers))
	}
}

// =============================================================================
// Model Resolution
// =============================================================================

func TestModelResolutionExact(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4", "gpt-3.5-turbo"}},
			{Name: "anthropic", Models: []string{"claude-sonnet-4-6"}},
		},
	}

	p, err := cfg.ResolveModel("gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "openai" {
		t.Errorf("resolved to provider %q, want %q", p.Name, "openai")
	}

	p, err = cfg.ResolveModel("claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "anthropic" {
		t.Errorf("resolved to provider %q, want %q", p.Name, "anthropic")
	}
}

func TestModelResolutionSuffix(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "deepseek", BaseURL: "https://api.deepseek.com/v1", Models: []string{"deepseek-r1"}},
		},
	}

	// Suffix match: "deepseek/deepseek-r1" should match "deepseek-r1".
	p, err := cfg.ResolveModel("deepseek/deepseek-r1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "deepseek" {
		t.Errorf("resolved to provider %q, want %q", p.Name, "deepseek")
	}
}

func TestModelResolutionFirstMatchWins(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "provider-a", BaseURL: "https://a.com/v1", Models: []string{"shared-model"}},
			{Name: "provider-b", BaseURL: "https://b.com/v1", Models: []string{"shared-model"}},
		},
	}

	p, err := cfg.ResolveModel("shared-model")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "provider-a" {
		t.Errorf("expected first match 'provider-a', got %q", p.Name)
	}
}

func TestModelResolutionNotFound(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.Provider{
			{Name: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4"}},
		},
	}

	_, err := cfg.ResolveModel("nonexistent-model")
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	if !strings.Contains(err.Error(), "no provider found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModelResolutionEmptyName(t *testing.T) {
	cfg := &config.Config{}
	_, err := cfg.ResolveModel("")
	if err == nil {
		t.Fatal("expected error for empty model name")
	}
	if !strings.Contains(err.Error(), "empty model name") {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// OpenAI Message Translation
// =============================================================================

func TestOpenAIMessageTranslationUserText(t *testing.T) {
	// toOpenAIMessages is unexported — test via roundtrip through the caller.
	// Instead, we test the user-visible behavior: a mock OpenAI server receiving
	// translated messages.

	var receivedBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body

		// Return a minimal SSE stream.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		delta := `{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`
		done := `{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
		fmt.Fprintf(w, "data: %s\n\n", delta)
		fmt.Fprintf(w, "data: %s\n\n", done)
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-openai",
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)

	msgs := []*models.Message{
		models.NewUserMessage("What is 2+2?"),
	}

	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages:     msgs,
		SystemPrompt: "You are a helpful assistant.",
		Model:        "gpt-4",
	})

	var events []api.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Verify the request body structure.
	var reqBody struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
		Stream   bool              `json:"stream"`
	}
	if err := json.Unmarshal(receivedBody, &reqBody); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if reqBody.Model != "gpt-4" {
		t.Errorf("model = %q, want %q", reqBody.Model, "gpt-4")
	}
	if !reqBody.Stream {
		t.Error("expected stream=true")
	}
	// Should have system + user = 2 messages.
	if len(reqBody.Messages) != 2 {
		t.Errorf("message count = %d, want 2", len(reqBody.Messages))
	}

	// Verify SSE events came through.
	var gotTextDelta, gotMessageDone bool
	for _, ev := range events {
		switch ev.Type {
		case "text_delta":
			gotTextDelta = true
			if ev.Text != "Hello" {
				t.Errorf("text_delta = %q, want %q", ev.Text, "Hello")
			}
		case "message_done":
			gotMessageDone = true
			if ev.Message == nil {
				t.Error("message_done should have a Message")
			} else {
				if ev.Message.TextContent() != "Hello" {
					t.Errorf("final message text = %q, want %q", ev.Message.TextContent(), "Hello")
				}
				if ev.Message.StopReason != models.StopEndTurn {
					t.Errorf("stop reason = %q, want %q", ev.Message.StopReason, models.StopEndTurn)
				}
			}
		case "error":
			t.Errorf("unexpected error event: %v", ev.Err)
		}
	}
	if !gotTextDelta {
		t.Error("no text_delta event received")
	}
	if !gotMessageDone {
		t.Error("no message_done event received")
	}
}

func TestOpenAIMessageTranslationAssistantToolUse(t *testing.T) {
	var receivedBody json.RawMessage
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		callCount++

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		// Return simple text response.
		delta := `{"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"},"finish_reason":null}]}`
		done := `{"id":"chatcmpl-2","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":1,"total_tokens":21}}`
		fmt.Fprintf(w, "data: %s\n\n", delta)
		fmt.Fprintf(w, "data: %s\n\n", done)
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-openai",
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)

	// Build a conversation with assistant tool_use + user tool_result.
	msgs := []*models.Message{
		models.NewUserMessage("Read the file"),
		{
			Role: models.RoleAssistant,
			Content: []models.Block{
				{Type: models.BlockText, Text: "Let me read that file."},
				{
					Type:  models.BlockToolUse,
					ID:    "tool-123",
					Name:  "Read",
					Input: json.RawMessage(`{"path": "/tmp/test.go"}`),
				},
			},
		},
		models.NewToolResultMessage([]models.Block{
			models.NewToolResultBlock("tool-123", "file contents here", false),
		}),
	}

	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages:     msgs,
		SystemPrompt: "You are a coding assistant.",
		Model:        "gpt-4",
	})

	for range ch {
		// drain
	}

	// Parse the sent request to verify translation.
	var reqBody struct {
		Messages []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
			ToolCallID string `json:"tool_call_id,omitempty"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(receivedBody, &reqBody); err != nil {
		t.Fatalf("parse request body: %v", err)
	}

	// Expect: system, user, assistant (with tool_calls), tool, in that order.
	if len(reqBody.Messages) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(reqBody.Messages))
	}

	if reqBody.Messages[0].Role != "system" {
		t.Errorf("msg[0] role = %q, want system", reqBody.Messages[0].Role)
	}
	if reqBody.Messages[1].Role != "user" {
		t.Errorf("msg[1] role = %q, want user", reqBody.Messages[1].Role)
	}
	if reqBody.Messages[2].Role != "assistant" {
		t.Errorf("msg[2] role = %q, want assistant", reqBody.Messages[2].Role)
	}
	if len(reqBody.Messages[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(reqBody.Messages[2].ToolCalls))
	}
	tc := reqBody.Messages[2].ToolCalls[0]
	if tc.ID != "tool-123" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "tool-123")
	}
	if tc.Type != "function" {
		t.Errorf("tool call type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "Read" {
		t.Errorf("tool call name = %q, want %q", tc.Function.Name, "Read")
	}

	// The tool result becomes a role:tool message.
	if reqBody.Messages[3].Role != "tool" {
		t.Errorf("msg[3] role = %q, want tool", reqBody.Messages[3].Role)
	}
	if reqBody.Messages[3].ToolCallID != "tool-123" {
		t.Errorf("tool_call_id = %q, want %q", reqBody.Messages[3].ToolCallID, "tool-123")
	}
	if reqBody.Messages[3].Content != "file contents here" {
		t.Errorf("tool result content = %q, want %q", reqBody.Messages[3].Content, "file contents here")
	}
}

// =============================================================================
// Tool Schema Wrapping
// =============================================================================

func TestOpenAIToolSchemaWrapping(t *testing.T) {
	var receivedBody json.RawMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		done := `{"id":"chatcmpl-3","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", done)
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-openai",
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file path to read",
			},
		},
		"required": []string{"path"},
	}

	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("Hi")},
		Tools: []api.ToolSchema{
			{
				Name:        "Read",
				Description: "Read a file",
				InputSchema: schema,
			},
		},
		Model: "gpt-4",
	})
	for range ch {
		// drain
	}

	// Verify tools sent as OpenAI format: type=function, function.parameters.
	var reqBody struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string      `json:"name"`
				Description string      `json:"description"`
				Parameters  interface{} `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(receivedBody, &reqBody); err != nil {
		t.Fatalf("parse request body: %v", err)
	}

	if len(reqBody.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(reqBody.Tools))
	}
	tool := reqBody.Tools[0]
	if tool.Type != "function" {
		t.Errorf("tool type = %q, want %q", tool.Type, "function")
	}
	if tool.Function.Name != "Read" {
		t.Errorf("tool name = %q, want %q", tool.Function.Name, "Read")
	}
	if tool.Function.Description != "Read a file" {
		t.Errorf("tool description = %q, want %q", tool.Function.Description, "Read a file")
	}
	if tool.Function.Parameters == nil {
		t.Error("tool parameters should not be nil")
	}
}

// =============================================================================
// OpenAI SSE Parsing
// =============================================================================

func TestOpenAISSETextDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		// Stream multiple text deltas.
		for i, word := range []string{"Hello", " ", "World", "!"} {
			chunk := fmt.Sprintf(`{"id":"chatcmpl-sse","model":"gpt-4","choices":[{"index":0,"delta":{"content":"%s"},"finish_reason":null}]}`, word)
			if i == 3 {
				// Last chunk includes finish_reason.
				chunk = fmt.Sprintf(`{"id":"chatcmpl-sse","model":"gpt-4","choices":[{"index":0,"delta":{"content":"%s"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`, word)
			}
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-sse",
		BaseURL: srv.URL,
		APIKey:  "key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)
	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("Hi")},
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

	if len(textDeltas) != 4 {
		t.Errorf("expected 4 text deltas, got %d", len(textDeltas))
	}
	combined := strings.Join(textDeltas, "")
	if combined != "Hello World!" {
		t.Errorf("combined text = %q, want %q", combined, "Hello World!")
	}

	if finalMsg == nil {
		t.Fatal("no message_done event")
	}
	if finalMsg.TextContent() != "Hello World!" {
		t.Errorf("final text = %q, want %q", finalMsg.TextContent(), "Hello World!")
	}
	if finalMsg.Usage == nil {
		t.Fatal("expected usage in final message")
	}
	if finalMsg.Usage.InputTokens != 10 {
		t.Errorf("input tokens = %d, want 10", finalMsg.Usage.InputTokens)
	}
	if finalMsg.Usage.OutputTokens != 4 {
		t.Errorf("output tokens = %d, want 4", finalMsg.Usage.OutputTokens)
	}
}

func TestOpenAISSEToolCallFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		// Simulate streaming tool call fragments.
		chunks := []string{
			// First chunk: tool call start with ID and function name.
			`{"id":"chatcmpl-tc","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-abc","type":"function","function":{"name":"Read","arguments":""}}]},"finish_reason":null}]}`,
			// Argument fragment 1.
			`{"id":"chatcmpl-tc","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}`,
			// Argument fragment 2.
			`{"id":"chatcmpl-tc","model":"gpt-4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"/tmp/f.go\"}"}}]},"finish_reason":null}]}`,
			// Finish.
			`{"id":"chatcmpl-tc","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":15,"completion_tokens":20,"total_tokens":35}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-tc",
		BaseURL: srv.URL,
		APIKey:  "key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)
	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("Read file")},
		Model:    "gpt-4",
	})

	var finalMsg *models.Message
	for ev := range ch {
		switch ev.Type {
		case "message_done":
			finalMsg = ev.Message
		case "error":
			t.Fatalf("unexpected error: %v", ev.Err)
		}
	}

	if finalMsg == nil {
		t.Fatal("no message_done event")
	}

	// Should have a tool_use block with assembled arguments.
	toolBlocks := finalMsg.ToolUseBlocks()
	if len(toolBlocks) != 1 {
		t.Fatalf("expected 1 tool_use block, got %d", len(toolBlocks))
	}

	tb := toolBlocks[0]
	if tb.ID != "call-abc" {
		t.Errorf("tool ID = %q, want %q", tb.ID, "call-abc")
	}
	if tb.Name != "Read" {
		t.Errorf("tool Name = %q, want %q", tb.Name, "Read")
	}
	expectedArgs := `{"path":"/tmp/f.go"}`
	if string(tb.Input) != expectedArgs {
		t.Errorf("tool args = %q, want %q", string(tb.Input), expectedArgs)
	}
	if finalMsg.StopReason != models.StopToolUse {
		t.Errorf("stop reason = %q, want %q", finalMsg.StopReason, models.StopToolUse)
	}
}

func TestOpenAISSEFinishReasonLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		chunks := []string{
			`{"id":"chatcmpl-len","model":"gpt-4","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-len","model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":100,"total_tokens":110}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name: "test-len", BaseURL: srv.URL, APIKey: "key", Models: []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)
	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("Hi")},
		Model:    "gpt-4",
	})

	var finalMsg *models.Message
	for ev := range ch {
		if ev.Type == "message_done" {
			finalMsg = ev.Message
		}
	}

	if finalMsg == nil {
		t.Fatal("no message_done")
	}
	if finalMsg.StopReason != models.StopMaxTokens {
		t.Errorf("stop reason = %q, want %q", finalMsg.StopReason, models.StopMaxTokens)
	}
}

// =============================================================================
// Factory: Anthropic vs. OpenAI Caller
// =============================================================================

func TestFactoryReturnsAnthropicCaller(t *testing.T) {
	tests := []struct {
		name     string
		provider config.Provider
	}{
		{
			name:     "name=anthropic",
			provider: config.Provider{Name: "anthropic", APIKey: "key", Models: []string{"claude-sonnet-4-6"}},
		},
		{
			name:     "empty BaseURL",
			provider: config.Provider{Name: "custom", BaseURL: "", APIKey: "key", Models: []string{"model"}},
		},
		{
			name:     "anthropic.com in URL",
			provider: config.Provider{Name: "custom", BaseURL: "https://api.anthropic.com/v1", APIKey: "key", Models: []string{"model"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := api.NewCaller(&tt.provider)
			// Type-check: if it's an OpenAICaller, the factory got it wrong.
			if _, ok := caller.(*api.OpenAICaller); ok {
				t.Error("expected AnthropicCaller, got OpenAICaller")
			}
		})
	}
}

func TestFactoryReturnsOpenAICaller(t *testing.T) {
	tests := []struct {
		name     string
		provider config.Provider
	}{
		{
			name:     "openrouter",
			provider: config.Provider{Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", APIKey: "key", Models: []string{"gpt-4"}},
		},
		{
			name:     "custom provider",
			provider: config.Provider{Name: "ollama", BaseURL: "http://localhost:11434/v1", APIKey: "", Models: []string{"llama3"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caller := api.NewCaller(&tt.provider)
			if _, ok := caller.(*api.OpenAICaller); !ok {
				t.Error("expected OpenAICaller")
			}
		})
	}
}

// =============================================================================
// CostForModelWithConfig
// =============================================================================

func TestCostForModelBuiltInPricing(t *testing.T) {
	u := models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}

	// Sonnet: $3 input, $15 output.
	cost := models.CostForModel("claude-sonnet-4-6", u)
	if cost != 18.0 {
		t.Errorf("sonnet cost = %.2f, want 18.00", cost)
	}

	// Opus 4.6 standard: $5 input, $25 output.
	cost = models.CostForModel("claude-opus-4-6", u)
	if cost != 30.0 {
		t.Errorf("opus 4.6 standard cost = %.2f, want 30.00", cost)
	}

	// Opus 4.6 fast: $30 input, $150 output.
	fastUsage := models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "fast"}
	cost = models.CostForModel("claude-opus-4-6", fastUsage)
	if cost != 180.0 {
		t.Errorf("opus 4.6 fast cost = %.2f, want 180.00", cost)
	}

	// Unknown model falls back to Sonnet tier.
	cost = models.CostForModel("unknown-model-xyz", u)
	if cost != 18.0 {
		t.Errorf("unknown model cost = %.2f, want 18.00 (sonnet fallback)", cost)
	}
}

func TestCostForModelWithCustomCosts(t *testing.T) {
	u := models.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}

	cc := &models.CostConfig{
		CustomCosts: map[string]models.ModelCosts{
			"custom-model": {InputTokens: 1.0, OutputTokens: 2.0},
		},
	}

	// Custom model uses custom pricing.
	cost := models.CostForModelWithConfig("custom-model", u, cc)
	if cost != 3.0 {
		t.Errorf("custom model cost = %.2f, want 3.00", cost)
	}

	// Non-custom model falls back to built-in pricing.
	cost = models.CostForModelWithConfig("claude-sonnet-4-6", u, cc)
	if cost != 18.0 {
		t.Errorf("sonnet with custom config fallback cost = %.2f, want 18.00", cost)
	}

	// Nil config falls back to built-in pricing.
	cost = models.CostForModelWithConfig("claude-sonnet-4-6", u, nil)
	if cost != 18.0 {
		t.Errorf("sonnet with nil config cost = %.2f, want 18.00", cost)
	}
}

func TestCostWithCacheAndWebSearch(t *testing.T) {
	// Sonnet pricing: CacheRead=$0.3/M, CacheWrite=$3.75/M, WebSearch=$0.01/req.
	u := models.Usage{
		InputTokens:   500_000,
		OutputTokens:  500_000,
		CacheRead:     200_000,
		CacheCreate:   100_000,
		WebSearchReqs: 3,
	}
	cost := models.CostForModel("claude-sonnet-4-6", u)

	expected := float64(500_000)/1_000_000*3 + // input
		float64(500_000)/1_000_000*15 + // output
		float64(200_000)/1_000_000*0.3 + // cache read
		float64(100_000)/1_000_000*3.75 + // cache write
		float64(3)*0.01 // web search

	if abs(cost-expected) > 0.001 {
		t.Errorf("cost = %.6f, want %.6f", cost, expected)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// =============================================================================
// Usage Accumulation
// =============================================================================

func TestUsageAccumulation(t *testing.T) {
	a := models.Usage{
		InputTokens:  100,
		OutputTokens: 50,
		CacheRead:    10,
		ServiceTier:  "standard",
		Speed:        "standard",
	}
	b := models.Usage{
		InputTokens:  200,
		OutputTokens: 100,
		CacheRead:    20,
		CacheCreate:  5,
		ServiceTier:  "enterprise",
		Speed:        "fast",
	}

	result := models.AccumulateUsage(a, b)

	if result.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", result.InputTokens)
	}
	if result.OutputTokens != 150 {
		t.Errorf("OutputTokens = %d, want 150", result.OutputTokens)
	}
	if result.CacheRead != 30 {
		t.Errorf("CacheRead = %d, want 30", result.CacheRead)
	}
	if result.CacheCreate != 5 {
		t.Errorf("CacheCreate = %d, want 5", result.CacheCreate)
	}
	// Metadata: last-write-wins.
	if result.ServiceTier != "enterprise" {
		t.Errorf("ServiceTier = %q, want %q", result.ServiceTier, "enterprise")
	}
	if result.Speed != "fast" {
		t.Errorf("Speed = %q, want %q", result.Speed, "fast")
	}
}

// =============================================================================
// OpenAI Error Handling
// =============================================================================

func TestOpenAICallerErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprintf(w, `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`)
	}))
	defer srv.Close()

	provider := &config.Provider{
		Name:    "test-error",
		BaseURL: srv.URL,
		APIKey:  "key",
		Models:  []string{"gpt-4"},
	}

	caller := api.NewOpenAICaller(provider)
	ch := caller.Stream(t.Context(), api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("Hi")},
		Model:    "gpt-4",
	})

	var gotError bool
	for ev := range ch {
		if ev.Type == "error" {
			gotError = true
			if ev.Err == nil {
				t.Error("expected non-nil error")
			}
		}
	}
	if !gotError {
		t.Error("expected error event from 429 response")
	}
}

// =============================================================================
// Env Overrides
// =============================================================================

func TestEnvOverrideFORGE_MODEL(t *testing.T) {
	t.Setenv("FORGE_MODEL", "overridden-model")

	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yaml := `
default_model: original-model
providers:
  - name: test
    models:
      - original-model
      - overridden-model
`
	if err := os.WriteFile(filepath.Join(forgeDir, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultModel != "overridden-model" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "overridden-model")
	}
}
