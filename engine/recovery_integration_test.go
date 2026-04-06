package engine

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
	"github.com/google/uuid"
)

// ============================================================
// Budget enforcement via SubmitMessage (Config → LoopParams plumbing)
// ============================================================

func TestIntegration_Budget_ViaSubmitMessage(t *testing.T) {
	// Verifies that MaxBudgetUSD set on engine.Config flows through
	// SubmitMessage → RunLoop → LoopParams.MaxBudgetUSD and triggers
	// StopBudgetExceeded.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("expensive response", models.Usage{
				InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "standard",
			}),
		},
	}

	qe := New(Config{
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: 1.0, // $1 budget — the response costs ~$18
		Cwd:          "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "do expensive work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBudgetExceeded {
		t.Errorf("expected StopBudgetExceeded, got %v", result.Reason)
	}
	if result.TotalCostUSD == 0 {
		t.Error("TotalCostUSD should be > 0")
	}
}

func TestIntegration_Budget_ZeroMeansUnlimited_ViaSubmitMessage(t *testing.T) {
	// MaxBudgetUSD=0 in Config means unlimited (nil *float64 in LoopParams).
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("done", models.Usage{
				InputTokens: 1_000_000, OutputTokens: 1_000_000, Speed: "standard",
			}),
		},
	}

	qe := New(Config{
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: 0, // 0 = unlimited
		Cwd:          "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted (unlimited budget), got %v", result.Reason)
	}
}

// ============================================================
// Budget accumulation across multiple tool-calling turns
// ============================================================

func TestIntegration_Budget_AccumulatesAcrossToolTurns(t *testing.T) {
	// Two tool-calling turns that each cost ~$1.80.
	// Budget is $3.00, so it should stop after the second turn ($3.60 > $3.00).
	// Token counts kept under 187K to avoid triggering auto-compact.
	echoTool := &mockTool{name: "Echo", result: "ok", safe: true}

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: tool call — costs ~$1.80 (under $3 budget)
			func() *models.Message {
				msg := assistantWithToolUse("working", "Echo", `{}`)
				msg.Usage = &models.Usage{InputTokens: 100_000, OutputTokens: 100_000, Speed: "standard"}
				return msg
			}(),
			// Turn 2: tool call — accumulated cost ~$3.60 (over $3 budget)
			func() *models.Message {
				msg := assistantWithToolUse("still working", "Echo", `{}`)
				msg.Usage = &models.Usage{InputTokens: 100_000, OutputTokens: 100_000, Speed: "standard"}
				return msg
			}(),
			// Turn 3: should never be reached
			assistantText("should not reach"),
		},
	}

	budget := 3.0
	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("work hard")},
		Tools:        []tools.Tool{echoTool},
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: &budget,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBudgetExceeded {
		t.Errorf("expected StopBudgetExceeded, got %v", result.Reason)
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns before budget exceeded, got %d", result.Turns)
	}
}

func TestIntegration_Budget_UnderLimit_Completes(t *testing.T) {
	// Budget is generous enough that the response completes normally.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantTextWithUsage("done", models.Usage{
				InputTokens: 1000, OutputTokens: 500, Speed: "standard",
			}),
		},
	}

	budget := 100.0
	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: &budget,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted (under budget), got %v", result.Reason)
	}
}

// ============================================================
// Max tokens recovery through SubmitMessage (end-to-end)
// ============================================================

func TestIntegration_MaxTokensRecovery_ViaSubmitMessage(t *testing.T) {
	// First call returns max_tokens, second call succeeds.
	// Verifies the full SubmitMessage → RunLoop → max_tokens recovery path.
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantMaxTokens("truncated"),
			assistantText("recovered"),
		},
	}

	qe := New(Config{
		Model:    "test-model",
		MaxTurns: 10,
		Cwd:      "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "tell me something long")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted (recovery succeeded), got %v", result.Reason)
	}

	// Verify token limit was escalated on the second call
	if len(caller.capturedParams) < 2 {
		t.Fatalf("expected at least 2 API calls, got %d", len(caller.capturedParams))
	}
	if caller.capturedParams[1].MaxTokens != maxTokensEscalated {
		t.Errorf("second call should use escalated tokens (%d), got %d",
			maxTokensEscalated, caller.capturedParams[1].MaxTokens)
	}

	// Verify truncated message is NOT in history
	for _, msg := range qe.Messages() {
		if msg.StopReason == models.StopMaxTokens {
			t.Error("truncated message should not be in message history after recovery")
		}
	}
}

// ============================================================
// Streaming executor: hooks fire during tool execution
// ============================================================

func TestIntegration_StreamingExecutor_PreToolUseHook_Denies(t *testing.T) {
	// A PreToolUse hook that denies tool execution.
	// The tool should NOT execute; a "Hook denied" result should be returned.
	hooks.RegisterInternalHook("deny-all", func(input hooks.HookInput) (*hooks.HookResult, error) {
		return &hooks.HookResult{
			Continue: false,
			Decision: "deny",
			Reason:   "blocked by test hook",
		}, nil
	})
	defer hooks.RegisterInternalHook("deny-all", nil) // cleanup

	hookSettings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			{
				Matcher: "", // match all tools
				Hooks: []hooks.HookConfig{
					{Command: "forge-internal:deny-all"},
				},
			},
		},
	}

	toolExecuted := false
	hookedTool := &hookTestTool{
		name: "HookedTool",
		fn: func() string {
			toolExecuted = true
			return "should not run"
		},
	}

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("let me use the tool", "HookedTool", `{}`),
			assistantText("I see it was denied."),
		},
	}

	tctx := &tools.ToolContext{
		Cwd:   "/tmp",
		Hooks: hookSettings,
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("test hooks")},
		Tools:    []tools.Tool{hookedTool},
		Model:    "test-model",
		MaxTurns: 10,
		ToolCtx:  tctx,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted, got %v", result.Reason)
	}
	if toolExecuted {
		t.Error("tool should NOT have executed — hook should have denied it")
	}

	// The tool_result message should contain the denial
	toolResultMsg := messages[2] // user + assistant(tool_use) + user(tool_result)
	if len(toolResultMsg.Content) == 0 {
		t.Fatal("expected tool_result block")
	}
	if !toolResultMsg.Content[0].IsError {
		t.Error("expected error tool_result from hook denial")
	}
}

func TestIntegration_StreamingExecutor_PreToolUseHook_ModifiesInput(t *testing.T) {
	// A PreToolUse hook that modifies tool input.
	hooks.RegisterInternalHook("modify-input", func(input hooks.HookInput) (*hooks.HookResult, error) {
		return &hooks.HookResult{
			Continue:     true,
			UpdatedInput: json.RawMessage(`{"modified": true}`),
		}, nil
	})
	defer hooks.RegisterInternalHook("modify-input", nil)

	hookSettings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			{
				Matcher: "",
				Hooks: []hooks.HookConfig{
					{Command: "forge-internal:modify-input"},
				},
			},
		},
	}

	var capturedInput json.RawMessage
	inputCaptureTool := &hookTestTool{
		name: "CaptureTool",
		fn:   func() string { return "ok" },
		onExecute: func(input json.RawMessage) {
			capturedInput = input
		},
	}

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "CaptureTool", `{"original": true}`),
			assistantText("done"),
		},
	}

	tctx := &tools.ToolContext{
		Cwd:   "/tmp",
		Hooks: hookSettings,
	}

	_, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("test")},
		Tools:    []tools.Tool{inputCaptureTool},
		Model:    "test-model",
		MaxTurns: 10,
		ToolCtx:  tctx,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("tool was not executed — capturedInput is nil")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(capturedInput, &parsed); err != nil {
		t.Fatalf("failed to parse captured input: %v", err)
	}
	if _, ok := parsed["modified"]; !ok {
		t.Errorf("expected hook to modify input to {\"modified\": true}, got %s", string(capturedInput))
	}
}

func TestIntegration_StreamingExecutor_PostToolUseHook_Fires(t *testing.T) {
	// A PostToolUse hook that records it was called.
	var postHookCalled atomic.Bool
	hooks.RegisterInternalHook("record-post", func(input hooks.HookInput) (*hooks.HookResult, error) {
		postHookCalled.Store(true)
		return &hooks.HookResult{Continue: true}, nil
	})
	defer hooks.RegisterInternalHook("record-post", nil)

	hookSettings := hooks.HooksSettings{
		hooks.HookEventPostToolUse: {
			{
				Matcher: "",
				Hooks: []hooks.HookConfig{
					{Command: "forge-internal:record-post"},
				},
			},
		},
	}

	simpleTool := &hookTestTool{
		name: "SimpleTool",
		fn:   func() string { return "result" },
	}

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("using tool", "SimpleTool", `{}`),
			assistantText("done"),
		},
	}

	tctx := &tools.ToolContext{
		Cwd:   "/tmp",
		Hooks: hookSettings,
	}

	_, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("test")},
		Tools:    []tools.Tool{simpleTool},
		Model:    "test-model",
		MaxTurns: 10,
		ToolCtx:  tctx,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !postHookCalled.Load() {
		t.Error("PostToolUse hook should have fired after tool execution")
	}
}

// ============================================================
// Streaming executor: permission overlay from ToolContext
// ============================================================

func TestIntegration_StreamingExecutor_PermissionOverlay(t *testing.T) {
	// Verify that the streaming executor's permission path works with
	// the PermissionPrompt callback (deny case).
	askTool := &permAskTool{name: "DangerousTool", result: "ran"}

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("running dangerous", "DangerousTool", `{}`),
			assistantText("it was denied"),
		},
	}

	tctx := &tools.ToolContext{
		Cwd: "/tmp",
		// No PermissionPrompt → PermAsk is treated as deny
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("test")},
		Tools:    []tools.Tool{askTool},
		Model:    "test-model",
		MaxTurns: 10,
		ToolCtx:  tctx,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted, got %v", result.Reason)
	}

	// Tool result should be a permission denied error
	toolResultMsg := messages[2]
	if len(toolResultMsg.Content) == 0 {
		t.Fatal("expected tool_result block")
	}
	if !toolResultMsg.Content[0].IsError {
		t.Error("expected error tool_result from permission denial")
	}
}

// ============================================================
// Helpers
// ============================================================

// hookTestTool is a tool for hook integration tests that calls a custom function.
type hookTestTool struct {
	name      string
	fn        func() string
	onExecute func(json.RawMessage) // optional input capture
}

func (t *hookTestTool) Name() string                 { return t.name }
func (t *hookTestTool) Description() string          { return "hook test tool" }
func (t *hookTestTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *hookTestTool) Execute(_ context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	if t.onExecute != nil {
		t.onExecute(input)
	}
	return &models.ToolResult{Content: t.fn()}, nil
}
func (t *hookTestTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *hookTestTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *hookTestTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *hookTestTool) IsReadOnly(_ json.RawMessage) bool        { return true }

// permAskTool always returns PermAsk, requiring interactive approval.
type permAskTool struct {
	name   string
	result string
}

func (t *permAskTool) Name() string                 { return t.name }
func (t *permAskTool) Description() string          { return "perm ask tool" }
func (t *permAskTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *permAskTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: t.result}, nil
}
func (t *permAskTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  "requires approval",
	}, nil
}
func (t *permAskTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *permAskTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *permAskTool) IsReadOnly(_ json.RawMessage) bool        { return false }

// Ensure uuid import is used (referenced by existing helpers in loop_test.go).
var _ = uuid.NewString
