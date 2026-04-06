package compact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/models"
	"github.com/google/uuid"
)

// mockCaller replays a fixed response for side-query calls.
type mockCaller struct {
	response *models.Message
	err      error
}

func (m *mockCaller) Stream(_ context.Context, _ api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 2)
	go func() {
		defer close(ch)
		if m.err != nil {
			ch <- api.StreamEvent{Type: "error", Err: m.err}
			return
		}
		if m.response != nil {
			ch <- api.StreamEvent{Type: "message_done", Message: m.response}
		}
	}()
	return ch
}

func summaryMsg(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
	}
}

func userMsg(text string) *models.Message {
	return models.NewUserMessage(text)
}

// --- ShouldCompact tests ---

func TestShouldCompact_BelowThreshold(t *testing.T) {
	// 200000 - 13000 - 1 = 186999 tokens: should NOT compact
	if ShouldCompact(ContextWindowTokens - AutoCompactBufferTokens - 1) {
		t.Error("expected no compact below threshold")
	}
}

func TestShouldCompact_AtThreshold(t *testing.T) {
	// exactly at threshold: 200000 - 13000 = 187000 → compact
	if !ShouldCompact(ContextWindowTokens - AutoCompactBufferTokens) {
		t.Error("expected compact at threshold")
	}
}

func TestShouldCompact_AboveThreshold(t *testing.T) {
	if !ShouldCompact(ContextWindowTokens) {
		t.Error("expected compact above threshold")
	}
}

// --- CompactConversation tests ---

func TestCompactConversation_ProducesOneSummaryMessage(t *testing.T) {
	msgs := []*models.Message{
		userMsg("hello"),
		{
			ID:         uuid.NewString(),
			Role:       models.RoleAssistant,
			StopReason: models.StopEndTurn,
			Content:    []models.Block{{Type: models.BlockText, Text: "hi there"}},
		},
		userMsg("what is 2+2?"),
	}

	caller := &mockCaller{response: summaryMsg("Summary of the conversation.")}
	result, err := CompactConversation(context.Background(), caller, msgs, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 compacted message, got %d", len(result))
	}
	got := result[0].TextContent()
	if got == "" {
		t.Error("compacted message should not be empty")
	}
	// Should mention count and include the summary
	if !containsSubstr(got, "Summary of the conversation.") {
		t.Errorf("compacted message should include summary, got: %s", got)
	}
	if !containsSubstr(got, "3 messages") {
		t.Errorf("compacted message should include message count, got: %s", got)
	}
}

func TestCompactConversation_EmptyMessages_ReturnsEmpty(t *testing.T) {
	caller := &mockCaller{response: summaryMsg("empty")}
	result, err := CompactConversation(context.Background(), caller, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 messages for empty input, got %d", len(result))
	}
}

func TestCompactConversation_StreamError_ReturnsError(t *testing.T) {
	caller := &mockCaller{err: context.DeadlineExceeded}
	_, err := CompactConversation(context.Background(), caller, []*models.Message{userMsg("hi")}, "")
	if err == nil {
		t.Error("expected error on stream failure")
	}
}

func TestCompactConversation_EmptySummary_ReturnsError(t *testing.T) {
	// Model returns a message with no text content
	emptyMsg := &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{},
	}
	caller := &mockCaller{response: emptyMsg}
	_, err := CompactConversation(context.Background(), caller, []*models.Message{userMsg("hi")}, "")
	if err == nil {
		t.Error("expected error for empty summary")
	}
}

func TestCompactConversation_IncludesSystemPrompt(t *testing.T) {
	var capturedPrompt string
	capCaller := &capturingCaller{
		onStream: func(params api.StreamParams) {
			if len(params.Messages) > 0 {
				capturedPrompt = params.Messages[0].TextContent()
			}
		},
		response: summaryMsg("ok"),
	}

	msgs := []*models.Message{userMsg("test")}
	_, _ = CompactConversation(context.Background(), capCaller, msgs, "you are a helpful assistant")

	if !containsSubstr(capturedPrompt, "you are a helpful assistant") {
		t.Errorf("system prompt not included in summary prompt, got: %s", capturedPrompt)
	}
}

func TestCompactConversation_IncludesToolCallsInPrompt(t *testing.T) {
	var capturedPrompt string
	capCaller := &capturingCaller{
		onStream: func(params api.StreamParams) {
			if len(params.Messages) > 0 {
				capturedPrompt = params.Messages[0].TextContent()
			}
		},
		response: summaryMsg("ok"),
	}

	toolUseMsg := &models.Message{
		ID:   uuid.NewString(),
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: "tu1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
		},
	}
	toolResultMsg := &models.Message{
		ID:   uuid.NewString(),
		Role: models.RoleUser,
		Content: []models.Block{
			{Type: models.BlockToolResult, ToolUseID: "tu1", Content: "file1.go\nfile2.go"},
		},
	}

	msgs := []*models.Message{userMsg("list files"), toolUseMsg, toolResultMsg}
	_, _ = CompactConversation(context.Background(), capCaller, msgs, "")

	if !containsSubstr(capturedPrompt, "bash") {
		t.Errorf("tool call not included in summary prompt, got: %s", capturedPrompt)
	}
	if !containsSubstr(capturedPrompt, "file1.go") {
		t.Errorf("tool result not included in summary prompt, got: %s", capturedPrompt)
	}
}

// --- EstimateTokens tests ---

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("EstimateTokens(nil) = %d, want 0", got)
	}
	if got := EstimateTokens([]*models.Message{}); got != 0 {
		t.Errorf("EstimateTokens([]) = %d, want 0", got)
	}
}

func TestEstimateTokens_TextMessage(t *testing.T) {
	// 40-char string → 40/4 = 10 tokens
	msg := userMsg("12345678901234567890123456789012345678901") // 41 chars → 10
	got := EstimateTokens([]*models.Message{msg})
	want := 41 / 4
	if got != want {
		t.Errorf("EstimateTokens = %d, want %d", got, want)
	}
}

func TestEstimateTokens_MultipleMessages(t *testing.T) {
	msgs := []*models.Message{
		userMsg("hello"),    // 5 chars → 1
		userMsg("world!!!"), // 8 chars → 2
	}
	got := EstimateTokens(msgs)
	want := 5/4 + 8/4 // integer division
	if got != want {
		t.Errorf("EstimateTokens = %d, want %d", got, want)
	}
}

func TestEstimateTokens_IncludesToolUseInput(t *testing.T) {
	input := json.RawMessage(`{"cmd":"ls -la"}`) // 16 chars → 4
	msg := &models.Message{
		ID:   "m1",
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: "tu1", Name: "bash", Input: input},
		},
	}
	got := EstimateTokens([]*models.Message{msg})
	want := len(input) / 4
	if got != want {
		t.Errorf("EstimateTokens with tool use = %d, want %d", got, want)
	}
}

func TestEstimateTokens_IncludesToolResult(t *testing.T) {
	content := "file1.go\nfile2.go" // 17 chars → 4
	msg := &models.Message{
		ID:   "m1",
		Role: models.RoleUser,
		Content: []models.Block{
			{Type: models.BlockToolResult, ToolUseID: "tu1", Content: content},
		},
	}
	got := EstimateTokens([]*models.Message{msg})
	want := len(content) / 4
	if got != want {
		t.Errorf("EstimateTokens with tool result = %d, want %d", got, want)
	}
}

func TestEstimateTokens_LargeMessageCrossesThreshold(t *testing.T) {
	// A message with enough text to cross ShouldCompact threshold.
	// Need > (200000-13000)*4 = 748000 chars.
	bigText := make([]byte, 800000)
	for i := range bigText {
		bigText[i] = 'x'
	}
	msg := userMsg(string(bigText))
	if !ShouldCompact(EstimateTokens([]*models.Message{msg})) {
		t.Error("large message should trigger ShouldCompact via EstimateTokens")
	}
}

// --- IncrementalEstimate tests ---

func TestIncrementalEstimate_MatchesFullEstimate(t *testing.T) {
	msgs := []*models.Message{
		userMsg("hello world"),        // 11 chars → 2
		userMsg("goodbye world!!!"),   // 15 chars → 3
		userMsg("third message here"), // 18 chars → 4
	}

	// Full estimate over all messages
	fullEstimate := EstimateTokens(msgs)

	// Incremental: first two, then add third
	partial := EstimateTokens(msgs[:2])
	incremental := IncrementalEstimate(partial, msgs[2:])

	if incremental != fullEstimate {
		t.Errorf("IncrementalEstimate=%d, want %d (full EstimateTokens)", incremental, fullEstimate)
	}
}

func TestIncrementalEstimate_EmptyNew(t *testing.T) {
	existing := 42
	got := IncrementalEstimate(existing, nil)
	if got != existing {
		t.Errorf("IncrementalEstimate with no new messages = %d, want %d", got, existing)
	}
}

func TestIncrementalEstimate_ZeroExisting(t *testing.T) {
	msgs := []*models.Message{userMsg("test")}
	got := IncrementalEstimate(0, msgs)
	want := EstimateTokens(msgs)
	if got != want {
		t.Errorf("IncrementalEstimate from zero = %d, want %d", got, want)
	}
}

func TestIncrementalEstimate_WithToolContent(t *testing.T) {
	textMsgs := []*models.Message{userMsg("hello")}
	toolMsg := &models.Message{
		ID:   "m1",
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: "tu1", Name: "bash", Input: json.RawMessage(`{"cmd":"ls"}`)},
		},
	}

	partial := EstimateTokens(textMsgs)
	incremental := IncrementalEstimate(partial, []*models.Message{toolMsg})
	full := EstimateTokens(append(textMsgs, toolMsg))

	if incremental != full {
		t.Errorf("IncrementalEstimate with tool = %d, want %d", incremental, full)
	}
}

// --- helpers ---

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

type capturingCaller struct {
	onStream func(api.StreamParams)
	response *models.Message
}

func (c *capturingCaller) Stream(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
	if c.onStream != nil {
		c.onStream(params)
	}
	ch := make(chan api.StreamEvent, 1)
	go func() {
		defer close(ch)
		if c.response != nil {
			ch <- api.StreamEvent{Type: "message_done", Message: c.response}
		}
	}()
	return ch
}
