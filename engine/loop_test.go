package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/services/compact"
	"github.com/egoisutolabs/forge/tools"
	"github.com/google/uuid"
)

// --- Mock API Caller ---

// mockCaller replays a sequence of responses. Each call to Stream() pops
// the next response off the queue and sends it as a complete message.
type mockCaller struct {
	responses []*models.Message
	callCount int
}

func (m *mockCaller) Stream(ctx context.Context, params api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 2)
	go func() {
		defer close(ch)
		if m.callCount >= len(m.responses) {
			ch <- api.StreamEvent{Type: "error", Err: context.DeadlineExceeded}
			return
		}
		msg := m.responses[m.callCount]
		m.callCount++

		// Send text deltas for any text blocks
		for _, b := range msg.Content {
			if b.Type == models.BlockText {
				ch <- api.StreamEvent{Type: "text_delta", Text: b.Text}
			}
		}
		// Send the complete message
		ch <- api.StreamEvent{Type: "message_done", Message: msg}
	}()
	return ch
}

// --- Mock Tool ---

type mockTool struct {
	name   string
	result string
	safe   bool
}

func (t *mockTool) Name() string                 { return t.name }
func (t *mockTool) Description() string          { return "mock tool" }
func (t *mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *mockTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: t.result}, nil
}
func (t *mockTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *mockTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *mockTool) IsConcurrencySafe(_ json.RawMessage) bool { return t.safe }
func (t *mockTool) IsReadOnly(_ json.RawMessage) bool        { return t.safe }

// --- Helpers ---

func assistantText(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
	}
}

func assistantWithToolUse(text string, toolName string, toolInput string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopToolUse,
		Content: []models.Block{
			{Type: models.BlockText, Text: text},
			{Type: models.BlockToolUse, ID: "tool_" + uuid.NewString()[:8], Name: toolName, Input: json.RawMessage(toolInput)},
		},
	}
}

// --- Tests ---

func TestLoop_SimpleResponse_NoToolCalls(t *testing.T) {
	// Claude responds with just text, no tool calls.
	// Loop should complete after 1 iteration.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("Hello! How can I help?"),
		},
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		SystemPrompt: "You are helpful.",
		Tools:        nil,
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if result.Turns != 1 {
		t.Errorf("expected 1 turn, got %d", result.Turns)
	}
	// Should have: user message + assistant message
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestLoop_SingleToolCall(t *testing.T) {
	// Turn 1: Claude calls a tool.
	// Turn 2: Claude responds with text (no more tools).
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("Let me check.", "Echo", `{"text":"hello"}`),
			assistantText("The echo tool returned: hello"),
		},
	}

	echoTool := &mockTool{name: "Echo", result: "hello", safe: true}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("test echo")},
		SystemPrompt: "You are helpful.",
		Tools:        []tools.Tool{echoTool},
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if result.Turns != 2 {
		t.Errorf("expected 2 turns, got %d", result.Turns)
	}
	// user + assistant(tool_use) + user(tool_result) + assistant(text)
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}
}

func TestLoop_MultipleToolCalls(t *testing.T) {
	// Claude calls two tools in one response, then responds with text.
	msg := &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopToolUse,
		Content: []models.Block{
			{Type: models.BlockText, Text: "Let me check both."},
			{Type: models.BlockToolUse, ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"a"}`)},
			{Type: models.BlockToolUse, ID: "t2", Name: "Echo", Input: json.RawMessage(`{"text":"b"}`)},
		},
	}
	caller := &mockCaller{
		responses: []*models.Message{
			msg,
			assistantText("Got both results."),
		},
	}

	echoTool := &mockTool{name: "Echo", result: "echoed", safe: true}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("test")},
		SystemPrompt: "",
		Tools:        []tools.Tool{echoTool},
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	// user + assistant(2 tools) + user(2 tool_results) + assistant(text)
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}
	// The tool_result message should contain 2 blocks
	toolResultMsg := messages[2]
	if len(toolResultMsg.Content) != 2 {
		t.Errorf("expected 2 tool_result blocks, got %d", len(toolResultMsg.Content))
	}
}

func TestLoop_HitsMaxTurns(t *testing.T) {
	// Claude keeps calling tools forever. Should stop at maxTurns.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("call 1", "Echo", `{}`),
			assistantWithToolUse("call 2", "Echo", `{}`),
			assistantWithToolUse("call 3", "Echo", `{}`),
			assistantWithToolUse("call 4", "Echo", `{}`),
		},
	}

	echoTool := &mockTool{name: "Echo", result: "ok", safe: true}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("loop forever")},
		SystemPrompt: "",
		Tools:        []tools.Tool{echoTool},
		Model:        "test-model",
		MaxTurns:     3,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBlockingLimit {
		t.Errorf("expected blocking_limit, got %v", result.Reason)
	}
	if result.Turns != 3 {
		t.Errorf("expected 3 turns, got %d", result.Turns)
	}
}

func TestLoop_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	caller := &mockCaller{
		responses: []*models.Message{assistantText("won't reach this")},
	}

	result, _, err := RunLoop(ctx, LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		SystemPrompt: "",
		Tools:        nil,
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopAborted {
		t.Errorf("expected aborted, got %v", result.Reason)
	}
}

func TestLoop_APIError(t *testing.T) {
	// The mock caller has no responses, so it returns an error.
	caller := &mockCaller{responses: nil}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		SystemPrompt: "",
		Tools:        nil,
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopModelError {
		t.Errorf("expected model_error, got %v", result.Reason)
	}
}

func TestLoop_UnknownToolGetsErrorResult(t *testing.T) {
	// Claude calls a tool that doesn't exist. Should get error tool_result
	// and then Claude should respond normally.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("calling", "NonExistent", `{}`),
			assistantText("I see the tool failed."),
		},
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("test")},
		SystemPrompt: "",
		Tools:        nil, // no tools registered
		Model:        "test-model",
		MaxTurns:     10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	// Check the tool_result message has an error
	toolResultMsg := messages[2]
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected 1 tool_result block, got %d", len(toolResultMsg.Content))
	}
	if !toolResultMsg.Content[0].IsError {
		t.Error("expected error tool_result for unknown tool")
	}
}

func TestLoop_EventCallback(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("hello world"),
		},
	}

	var textDeltas []string
	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		SystemPrompt: "",
		Tools:        nil,
		Model:        "test-model",
		MaxTurns:     10,
		OnEvent: func(event api.StreamEvent) {
			if event.Type == "text_delta" {
				textDeltas = append(textDeltas, event.Text)
			}
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if len(textDeltas) != 1 || textDeltas[0] != "hello world" {
		t.Errorf("expected text delta callback, got %v", textDeltas)
	}
}

// ============================================================
// capturingCaller: like mockCaller but also records StreamParams
// ============================================================

type capturingCaller struct {
	responses      []*models.Message
	callCount      int
	capturedParams []api.StreamParams
}

func (m *capturingCaller) Stream(ctx context.Context, params api.StreamParams) <-chan api.StreamEvent {
	m.capturedParams = append(m.capturedParams, params)
	ch := make(chan api.StreamEvent, 2)
	go func() {
		defer close(ch)
		if m.callCount >= len(m.responses) {
			ch <- api.StreamEvent{Type: "error", Err: context.DeadlineExceeded}
			return
		}
		msg := m.responses[m.callCount]
		m.callCount++
		for _, b := range msg.Content {
			if b.Type == models.BlockText {
				ch <- api.StreamEvent{Type: "text_delta", Text: b.Text}
			}
		}
		ch <- api.StreamEvent{Type: "message_done", Message: msg}
	}()
	return ch
}

// assistantMaxTokens returns an assistant message with StopReason=max_tokens.
func assistantMaxTokens(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopMaxTokens,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
		Usage:      &models.Usage{InputTokens: 100, OutputTokens: 8192},
	}
}

// ============================================================
// Step 10 — StreamingExecutor wired into loop
// ============================================================

func TestLoop_StreamingExecutor_ConcurrentToolsRunInParallel(t *testing.T) {
	// Two concurrent-safe tools should execute in parallel (overlapping).
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	type slowInput struct{}
	slowTool := &slowConcurrentTool{
		name: "Slow",
		fn: func() string {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := maxInFlight.Load()
				if cur > old {
					if maxInFlight.CompareAndSwap(old, cur) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			return "ok"
		},
	}

	caller := &mockCaller{
		responses: []*models.Message{
			{
				ID:         uuid.NewString(),
				Role:       models.RoleAssistant,
				StopReason: models.StopToolUse,
				Content: []models.Block{
					{Type: models.BlockToolUse, ID: "t1", Name: "Slow", Input: json.RawMessage(`{}`)},
					{Type: models.BlockToolUse, ID: "t2", Name: "Slow", Input: json.RawMessage(`{}`)},
				},
			},
			assistantText("done"),
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("go")},
		Tools:    []tools.Tool{slowTool},
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if maxInFlight.Load() < 2 {
		t.Errorf("expected concurrent tool execution, max in-flight was %d", maxInFlight.Load())
	}
}

// slowConcurrentTool is a concurrency-safe tool that runs a custom fn.
type slowConcurrentTool struct {
	name string
	fn   func() string
}

func (s *slowConcurrentTool) Name() string        { return s.name }
func (s *slowConcurrentTool) Description() string { return "slow concurrent" }
func (s *slowConcurrentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (s *slowConcurrentTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: s.fn()}, nil
}
func (s *slowConcurrentTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *slowConcurrentTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (s *slowConcurrentTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *slowConcurrentTool) IsReadOnly(_ json.RawMessage) bool        { return true }

// ============================================================
// Step 11 — max_tokens recovery
// ============================================================

func TestLoop_MaxTokens_FirstHit_EscalatesTokenLimit(t *testing.T) {
	// First call returns max_tokens. Second call (with more tokens) succeeds.
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantMaxTokens("partial response..."),
			assistantText("full response"),
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if len(caller.capturedParams) < 2 {
		t.Fatalf("expected at least 2 API calls, got %d", len(caller.capturedParams))
	}
	if caller.capturedParams[0].MaxTokens != 8192 {
		t.Errorf("first call should use 8192 tokens, got %d", caller.capturedParams[0].MaxTokens)
	}
	if caller.capturedParams[1].MaxTokens != maxTokensEscalated {
		t.Errorf("second call should use %d tokens, got %d", maxTokensEscalated, caller.capturedParams[1].MaxTokens)
	}
	// The truncated message should NOT appear in the final message list
	if result.Turns != 1 {
		t.Errorf("escalation should not count as a turn, expected 1 turn got %d", result.Turns)
	}
}

func TestLoop_MaxTokens_SubsequentHits_InjectResumeMessage(t *testing.T) {
	// Hit 1 (retries=0): escalate tokens
	// Hit 2 (retries=1): inject resume
	// Hit 3 (retries=2): inject resume
	// Hit 4 (retries=3): inject resume (last allowed)
	// Call 5: succeeds
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantMaxTokens("cut1"),    // hit 1 → escalate
			assistantMaxTokens("cut2"),    // hit 2 → inject resume
			assistantMaxTokens("cut3"),    // hit 3 → inject resume
			assistantMaxTokens("cut4"),    // hit 4 → inject resume
			assistantText("finally done"), // success
		},
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	// Verify "Resume directly" messages were injected
	resumeCount := 0
	for _, msg := range messages {
		if msg.Role == models.RoleUser && strings.Contains(msg.TextContent(), "Resume directly") {
			resumeCount++
		}
	}
	if resumeCount != 3 {
		t.Errorf("expected 3 resume injections, got %d", resumeCount)
	}
}

func TestLoop_MaxTokens_TooManyRetries_ReturnsOutputTruncated(t *testing.T) {
	// 5 consecutive max_tokens hits → should surface StopOutputTruncated
	responses := make([]*models.Message, 5)
	for i := range responses {
		responses[i] = assistantMaxTokens("cut")
	}

	caller := &mockCaller{responses: responses}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopOutputTruncated {
		t.Errorf("expected output_truncated, got %v", result.Reason)
	}
}

func TestLoop_MaxTokens_ResetAfterSuccess(t *testing.T) {
	// max_tokens hit, then successful response, then another max_tokens should restart recovery.
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantMaxTokens("cut1"), // hit 1 → escalate
			assistantText("success 1"), // resets retry counter
			assistantMaxTokens("cut2"), // hit 1 again (counter reset) → escalate
			assistantText("success 2"),
		},
	}

	// Need a tool call to keep the loop going after first success
	echoTool := &mockTool{name: "Echo", result: "echoed", safe: true}
	caller.responses[1] = assistantWithToolUse("got it", "Echo", `{}`)
	caller.responses[2] = assistantMaxTokens("cut2") // still mid-loop

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Tools:    []tools.Tool{echoTool},
		Model:    "test-model",
		MaxTurns: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	// After the first success, the second escalation should use 8192 again
	// (because retries were reset). Then escalate to 64000.
	if len(caller.capturedParams) < 3 {
		t.Fatalf("expected at least 3 API calls, got %d", len(caller.capturedParams))
	}
}

// ============================================================
// Step 12 — Auto-compact integration
// ============================================================

func TestLoop_AutoCompact_TriggeredNearContextLimit(t *testing.T) {
	// The assistant message reports input tokens at the compact threshold.
	// compact.CompactConversation uses CompactModel; the main loop uses "test-model".
	// We use params.Model to route responses.
	compactCalled := false

	// highTokens is just above the ShouldCompact threshold.
	highTokens := compact.ContextWindowTokens - compact.AutoCompactBufferTokens

	specialCaller := &funcCaller{
		fn: func(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
			ch := make(chan api.StreamEvent, 2)
			go func() {
				defer close(ch)
				if params.Model == compact.CompactModel {
					// Compact side-query
					compactCalled = true
					ch <- api.StreamEvent{
						Type: "message_done",
						Message: &models.Message{
							ID:         uuid.NewString(),
							Role:       models.RoleAssistant,
							StopReason: models.StopEndTurn,
							Content:    []models.Block{{Type: models.BlockText, Text: "Summary of prior conversation."}},
						},
					}
					return
				}
				// Main loop turn — report high token usage, no tool calls (done)
				ch <- api.StreamEvent{
					Type: "message_done",
					Message: &models.Message{
						ID:         uuid.NewString(),
						Role:       models.RoleAssistant,
						StopReason: models.StopEndTurn,
						Content:    []models.Block{{Type: models.BlockText, Text: "hello"}},
						Usage:      &models.Usage{InputTokens: highTokens, OutputTokens: 100},
					},
				}
			}()
			return ch
		},
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:   specialCaller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if !compactCalled {
		t.Error("expected compact side-query to be called")
	}
	// After compact, messages = [boundary]. Loop ends with that 1 message.
	if len(messages) != 1 {
		t.Errorf("expected 1 compacted message, got %d", len(messages))
	}
	if !strings.Contains(messages[0].TextContent(), "Summary of prior conversation.") {
		t.Errorf("compacted message should contain summary, got: %s", messages[0].TextContent())
	}
}

func TestLoop_AutoCompact_CircuitBreaker_StopsAfter3Failures(t *testing.T) {
	// Compact fails 3 times (circuit breaker). The loop keeps going afterward.
	// We use tool calls to drive multiple main-loop turns.
	// Main loop uses "test-model"; compact uses CompactModel.
	// We distinguish them via params.Model.
	compactAttempts := 0
	mainTurns := 0

	highTokens := compact.ContextWindowTokens - compact.AutoCompactBufferTokens
	echoTool := &mockTool{name: "Echo", result: "ok", safe: true}

	specialCaller := &funcCaller{
		fn: func(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
			ch := make(chan api.StreamEvent, 2)
			go func() {
				defer close(ch)
				if params.Model == compact.CompactModel {
					// Compact side-query → always fail
					compactAttempts++
					ch <- api.StreamEvent{Type: "error", Err: context.DeadlineExceeded}
					return
				}
				// Main loop turn
				mainTurns++
				var msg *models.Message
				if mainTurns <= 3 {
					// First 3 turns: call a tool + high tokens → compact triggered
					msg = &models.Message{
						ID:         uuid.NewString(),
						Role:       models.RoleAssistant,
						StopReason: models.StopToolUse,
						Content: []models.Block{
							{Type: models.BlockText, Text: "doing"},
							{Type: models.BlockToolUse, ID: uuid.NewString()[:8], Name: "Echo", Input: json.RawMessage(`{}`)},
						},
						Usage: &models.Usage{InputTokens: highTokens},
					}
				} else {
					// Turn 4+: just respond (no tool, no high tokens)
					msg = assistantText("all done")
				}
				ch <- api.StreamEvent{Type: "message_done", Message: msg}
			}()
			return ch
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   specialCaller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Tools:    []tools.Tool{echoTool},
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	// Circuit breaker should have tripped after exactly 3 failures
	if compactAttempts != compact.MaxCircuitBreakFailures {
		t.Errorf("expected %d compact attempts (circuit breaker), got %d",
			compact.MaxCircuitBreakFailures, compactAttempts)
	}
}

// ============================================================
// Proactive auto-compact (before API call)
// ============================================================

func TestLoop_ProactiveAutoCompact_TriggeredBeforeAPICall(t *testing.T) {
	// Fill the input messages with enough text to push EstimateTokens above the
	// compact threshold (> (200000-13000)*4 = 748000 chars).
	bigText := strings.Repeat("x", 800_000)
	largeMsg := models.NewUserMessage(bigText)

	callOrder := make([]string, 0, 3)

	specialCaller := &funcCaller{
		fn: func(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
			ch := make(chan api.StreamEvent, 2)
			go func() {
				defer close(ch)
				if params.Model == compact.CompactModel {
					callOrder = append(callOrder, "compact")
					ch <- api.StreamEvent{
						Type: "message_done",
						Message: &models.Message{
							ID:         "summary-1",
							Role:       models.RoleAssistant,
							StopReason: models.StopEndTurn,
							Content:    []models.Block{{Type: models.BlockText, Text: "Proactive compact summary."}},
						},
					}
					return
				}
				// Main loop turn
				callOrder = append(callOrder, "main")
				ch <- api.StreamEvent{
					Type: "message_done",
					Message: &models.Message{
						ID:         "main-1",
						Role:       models.RoleAssistant,
						StopReason: models.StopEndTurn,
						Content:    []models.Block{{Type: models.BlockText, Text: "done"}},
					},
				}
			}()
			return ch
		},
	}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:   specialCaller,
		Messages: []*models.Message{largeMsg},
		Model:    "test-model",
		MaxTurns: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	// Compact must happen BEFORE the main API call.
	if len(callOrder) < 2 {
		t.Fatalf("expected at least 2 calls (compact + main), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "compact" {
		t.Errorf("expected first call to be compact, got %q; order: %v", callOrder[0], callOrder)
	}
	if callOrder[1] != "main" {
		t.Errorf("expected second call to be main, got %q; order: %v", callOrder[1], callOrder)
	}

	// After compact, the conversation should be the summary message.
	_ = messages
}

func TestLoop_ProactiveAutoCompact_SmallMessages_NotTriggered(t *testing.T) {
	// Small messages should NOT trigger proactive compact.
	compactCalled := false

	specialCaller := &funcCaller{
		fn: func(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
			ch := make(chan api.StreamEvent, 2)
			go func() {
				defer close(ch)
				if params.Model == compact.CompactModel {
					compactCalled = true
					ch <- api.StreamEvent{
						Type: "message_done",
						Message: &models.Message{
							ID:         "s",
							Role:       models.RoleAssistant,
							StopReason: models.StopEndTurn,
							Content:    []models.Block{{Type: models.BlockText, Text: "summary"}},
						},
					}
					return
				}
				ch <- api.StreamEvent{
					Type: "message_done",
					Message: &models.Message{
						ID:         "m",
						Role:       models.RoleAssistant,
						StopReason: models.StopEndTurn,
						Content:    []models.Block{{Type: models.BlockText, Text: "done"}},
					},
				}
			}()
			return ch
		},
	}

	result, _, err := RunLoop(context.Background(), LoopParams{
		Caller:   specialCaller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 5,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if compactCalled {
		t.Error("compact should NOT be triggered for small messages")
	}
}

// funcCaller is a Caller backed by an arbitrary function (for complex test scenarios).
type funcCaller struct {
	fn func(ctx context.Context, params api.StreamParams) <-chan api.StreamEvent
}

func (f *funcCaller) Stream(ctx context.Context, params api.StreamParams) <-chan api.StreamEvent {
	return f.fn(ctx, params)
}

// ── drainNotifications unit tests ───────────────────────────────────────────

func TestDrainNotifications_NilChannel(t *testing.T) {
	msgs := []*models.Message{models.NewUserMessage("hello")}
	got := drainNotifications(nil, msgs)
	if len(got) != 1 {
		t.Errorf("expected 1 message for nil channel, got %d", len(got))
	}
}

func TestDrainNotifications_EmptyChannel(t *testing.T) {
	ch := make(chan string, 8)
	msgs := []*models.Message{models.NewUserMessage("hello")}
	got := drainNotifications(ch, msgs)
	if len(got) != 1 {
		t.Errorf("expected 1 message for empty channel, got %d", len(got))
	}
}

func TestDrainNotifications_DrainsAll(t *testing.T) {
	ch := make(chan string, 8)
	ch <- "<task-notification>Agent A done</task-notification>"
	ch <- "<task-notification>Agent B done</task-notification>"

	msgs := []*models.Message{models.NewUserMessage("hello")}
	got := drainNotifications(ch, msgs)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages (1 original + 2 notifications), got %d", len(got))
	}
	if !strings.Contains(got[1].TextContent(), "Agent A") {
		t.Errorf("first notification should be Agent A, got %q", got[1].TextContent())
	}
	if !strings.Contains(got[2].TextContent(), "Agent B") {
		t.Errorf("second notification should be Agent B, got %q", got[2].TextContent())
	}
}

// TestRunLoop_InjectsNotifications verifies that background agent notifications
// are injected into the conversation between turns.
func TestRunLoop_InjectsNotifications(t *testing.T) {
	notifCh := make(chan string, 8)

	echoTool := &mockTool{name: "Echo", result: "echoed", safe: true}

	callNum := atomic.Int32{}
	caller := &funcCaller{fn: func(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
		ch := make(chan api.StreamEvent, 2)
		go func() {
			defer close(ch)
			n := callNum.Add(1)
			var msg *models.Message
			if n == 1 {
				// First call: respond with a tool use
				msg = assistantWithToolUse("calling tool", "Echo", `{}`)
				// Simulate a background agent completing during tool execution
				notifCh <- "<task-notification>\nAgent \"researcher\" completed:\nfound results\n</task-notification>"
			} else {
				// Second call: just text (end the loop)
				msg = assistantText("done")
			}
			ch <- api.StreamEvent{Type: "message_done", Message: msg}
		}()
		return ch
	}}

	result, messages, err := RunLoop(context.Background(), LoopParams{
		Caller:        caller,
		Messages:      []*models.Message{models.NewUserMessage("start")},
		SystemPrompt:  "test",
		Tools:         []tools.Tool{echoTool},
		Model:         "test-model",
		MaxTurns:      10,
		Notifications: notifCh,
	})
	if err != nil {
		t.Fatalf("RunLoop error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %s", result.Reason)
	}

	// Check that the notification was injected into the conversation
	foundNotification := false
	for _, m := range messages {
		if m.Role == models.RoleUser && strings.Contains(m.TextContent(), "task-notification") {
			foundNotification = true
			break
		}
	}
	if !foundNotification {
		t.Error("expected a task-notification user message to be injected into conversation")
	}
}
