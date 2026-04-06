package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/skills"
	"github.com/egoisutolabs/forge/tools"
)

// captureTool is a mock tool that captures the ToolContext passed during Execute.
type captureTool struct {
	captured *tools.ToolContext
}

func (c *captureTool) Name() string                 { return "capture" }
func (c *captureTool) Description() string          { return "captures tool context" }
func (c *captureTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (c *captureTool) Execute(_ context.Context, _ json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	c.captured = tctx
	return &models.ToolResult{Content: "ok"}, nil
}
func (c *captureTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (c *captureTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (c *captureTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (c *captureTool) IsReadOnly(_ json.RawMessage) bool        { return true }

func TestSubmitMessage_AddsUserMessageAndCallsLoop(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("Hi there!"),
		},
	}

	qe := New(Config{
		Model:        "test-model",
		SystemPrompt: "You are helpful.",
		MaxTurns:     10,
		Cwd:          "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	msgs := qe.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in history, got %d", len(msgs))
	}
	if msgs[0].Role != models.RoleUser {
		t.Errorf("expected first message to be user, got %v", msgs[0].Role)
	}
	if msgs[1].Role != models.RoleAssistant {
		t.Errorf("expected second message to be assistant, got %v", msgs[1].Role)
	}
}

func TestSubmitMessage_PersistsAcrossTurns(t *testing.T) {
	// First turn: simple response
	// Second turn: simple response
	// History should accumulate across both calls
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("First reply"),
			assistantText("Second reply"),
		},
	}

	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "first")
	if err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	if len(qe.Messages()) != 2 {
		t.Fatalf("after turn 1: expected 2 messages, got %d", len(qe.Messages()))
	}

	_, err = qe.SubmitMessage(context.Background(), caller, "second")
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	if len(qe.Messages()) != 4 {
		t.Fatalf("after turn 2: expected 4 messages, got %d", len(qe.Messages()))
	}
}

func TestSubmitMessage_WithEventCallback(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("streaming text"),
		},
	}

	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
	})

	var events []api.StreamEvent
	qe.OnEvent = func(event api.StreamEvent) {
		events = append(events, event)
	}

	_, err := qe.SubmitMessage(context.Background(), caller, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have received at least the text_delta event
	hasTextDelta := false
	for _, e := range events {
		if e.Type == "text_delta" {
			hasTextDelta = true
		}
	}
	if !hasTextDelta {
		t.Error("expected to receive text_delta event via OnEvent callback")
	}
}

func TestSubmitMessage_ToolContext_SkillsWired(t *testing.T) {
	ct := &captureTool{}
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "capture", `{}`),
			assistantText("done"),
		},
	}

	reg := skills.NewRegistry()
	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
		Tools:    []tools.Tool{ct},
		Skills:   reg,
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "test skills")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.captured == nil {
		t.Fatal("captureTool was not invoked")
	}
	if ct.captured.Skills != reg {
		t.Error("expected ToolContext.Skills to match the configured registry")
	}
}

func TestSubmitMessage_ToolContext_GlobMaxResults_Default(t *testing.T) {
	ct := &captureTool{}
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "capture", `{}`),
			assistantText("done"),
		},
	}

	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
		Tools:    []tools.Tool{ct},
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "test glob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.captured == nil {
		t.Fatal("captureTool was not invoked")
	}
	if ct.captured.GlobMaxResults != 100 {
		t.Errorf("expected GlobMaxResults=100 (default), got %d", ct.captured.GlobMaxResults)
	}
}

func TestSubmitMessage_ToolContext_GlobMaxResults_Custom(t *testing.T) {
	ct := &captureTool{}
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "capture", `{}`),
			assistantText("done"),
		},
	}

	qe := New(Config{
		Model:          "test-model",
		MaxTurns:       10,
		Cwd:            "/tmp",
		Tools:          []tools.Tool{ct},
		GlobMaxResults: 250,
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "test glob custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.captured == nil {
		t.Fatal("captureTool was not invoked")
	}
	if ct.captured.GlobMaxResults != 250 {
		t.Errorf("expected GlobMaxResults=250, got %d", ct.captured.GlobMaxResults)
	}
}

func TestSubmitMessage_ToolContext_UserPromptWired(t *testing.T) {
	ct := &captureTool{}
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "capture", `{}`),
			assistantText("done"),
		},
	}

	promptFn := func(questions []tools.AskQuestion) (map[string]string, error) {
		return nil, nil
	}
	qe := New(Config{
		Model:      "test-model",
		MaxTurns:   10,
		Cwd:        "/tmp",
		Tools:      []tools.Tool{ct},
		UserPrompt: promptFn,
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "test user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.captured == nil {
		t.Fatal("captureTool was not invoked")
	}
	if ct.captured.UserPrompt == nil {
		t.Error("expected ToolContext.UserPrompt to be non-nil")
	}
}

func TestSubmitMessage_ToolContext_HooksWired(t *testing.T) {
	ct := &captureTool{}
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "capture", `{}`),
			assistantText("done"),
		},
	}

	// Use SessionStart event (not PreToolUse) to avoid hook execution
	// interfering with the tool call. We just need to verify the map
	// is passed through to ToolContext.
	hs := hooks.HooksSettings{
		hooks.HookEventSessionStart: {
			{Matcher: "", Hooks: []hooks.HookConfig{{Command: "echo test"}}},
		},
	}
	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
		Tools:    []tools.Tool{ct},
		Hooks:    hs,
	})

	_, err := qe.SubmitMessage(context.Background(), caller, "test hooks")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.captured == nil {
		t.Fatal("captureTool was not invoked")
	}
	if len(ct.captured.Hooks) == 0 {
		t.Error("expected ToolContext.Hooks to be non-empty")
	}
	if _, ok := ct.captured.Hooks[hooks.HookEventSessionStart]; !ok {
		t.Error("expected ToolContext.Hooks to contain SessionStart event")
	}
}
