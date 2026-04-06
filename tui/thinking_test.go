package tui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderThinkingBlock_Empty(t *testing.T) {
	got := renderThinkingBlock("", false, 80)
	if got != "" {
		t.Fatalf("expected empty output for empty thinking, got %q", got)
	}
}

func TestRenderThinkingBlock_Collapsed(t *testing.T) {
	got := renderThinkingBlock("some thinking here", false, 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "💭") {
		t.Fatal("expected thinking emoji in collapsed block")
	}
	if !strings.Contains(stripped, "Thinking") {
		t.Fatal("expected 'Thinking' text in collapsed block")
	}
	if !strings.Contains(stripped, "Tab to expand") {
		t.Fatal("expected 'Tab to expand' hint in collapsed block")
	}
	// Should be single line
	lines := strings.Split(strings.TrimSpace(stripped), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for collapsed thinking, got %d", len(lines))
	}
}

func TestRenderThinkingBlock_Expanded(t *testing.T) {
	thinking := "I need to think about this.\nLet me consider the options.\nOk I have a plan."
	got := renderThinkingBlock(thinking, true, 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "💭") {
		t.Fatal("expected thinking emoji in expanded block")
	}
	if !strings.Contains(stripped, "│") {
		t.Fatal("expected left border │ in expanded thinking")
	}
	if !strings.Contains(stripped, "think about this") {
		t.Fatal("expected thinking content in expanded block")
	}
	if !strings.Contains(stripped, "consider the options") {
		t.Fatal("expected second line in expanded block")
	}
}

func TestRenderThinkingBlock_ExpandedTruncation(t *testing.T) {
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, "thinking line content here")
	}
	thinking := strings.Join(lines, "\n")
	got := renderThinkingBlock(thinking, true, 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "more lines") {
		t.Fatal("expected 'more lines' indicator for long thinking content")
	}
}

func TestRenderThinkingBlock_ExpandedLongLine(t *testing.T) {
	thinking := strings.Repeat("x", 200)
	got := renderThinkingBlock(thinking, true, 80)
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "...") {
		t.Fatal("expected truncation marker for long line")
	}
}

func TestRenderMessage_AssistantWithThinking_Collapsed(t *testing.T) {
	msg := DisplayMessage{
		Role:     "assistant",
		Content:  "Here is my answer.",
		Thinking: "Let me think about this problem carefully.",
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "💭") {
		t.Fatal("expected thinking emoji in assistant message with thinking")
	}
	if !strings.Contains(stripped, "Tab to expand") {
		t.Fatal("expected collapsed thinking hint")
	}
	if !strings.Contains(stripped, "Forge") {
		t.Fatal("expected 'Forge' label")
	}
}

func TestRenderMessage_AssistantWithThinking_Expanded(t *testing.T) {
	msg := DisplayMessage{
		Role:             "assistant",
		Content:          "Here is my answer.",
		Thinking:         "Deep analysis of the problem.",
		ThinkingExpanded: true,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "💭") {
		t.Fatal("expected thinking emoji")
	}
	if !strings.Contains(stripped, "Deep analysis") {
		t.Fatal("expected thinking content visible when expanded")
	}
}

func TestRenderMessage_AssistantWithModel(t *testing.T) {
	msg := DisplayMessage{
		Role:    "assistant",
		Content: "Hello!",
		Model:   "claude-sonnet-4-6",
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Forge") {
		t.Fatal("expected 'Forge' label")
	}
	if !strings.Contains(stripped, "sonnet-4.6") {
		t.Fatalf("expected model indicator 'sonnet-4.6' in output, got: %s", stripped)
	}
}

func TestRenderMessage_UserWithTimestamp(t *testing.T) {
	msg := DisplayMessage{
		Role:           "user",
		Content:        "Hello there",
		Timestamp:      time.Now().Add(-3 * time.Minute),
		ShowTimestamps: true,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "3m ago") {
		t.Fatalf("expected '3m ago' in user message with timestamp, got: %s", stripped)
	}
}

func TestRenderMessage_AssistantWithTimestamp(t *testing.T) {
	msg := DisplayMessage{
		Role:           "assistant",
		Content:        "Hello!",
		Timestamp:      time.Now().Add(-10 * time.Minute),
		ShowTimestamps: true,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "10m ago") {
		t.Fatalf("expected '10m ago' in assistant message with timestamp, got: %s", stripped)
	}
}

func TestRenderMessage_NoTimestampWhenDisabled(t *testing.T) {
	msg := DisplayMessage{
		Role:           "user",
		Content:        "Hello",
		Timestamp:      time.Now().Add(-5 * time.Minute),
		ShowTimestamps: false,
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	if strings.Contains(stripped, "ago") {
		t.Fatal("expected no timestamp when ShowTimestamps is false")
	}
}

func TestRenderMessage_AssistantNoModelWhenEmpty(t *testing.T) {
	msg := DisplayMessage{
		Role:    "assistant",
		Content: "Hello!",
	}
	result := renderMessage(msg, 80)
	stripped := stripANSI(result)
	// Should not show parenthetical model tag
	if strings.Contains(stripped, "(") {
		t.Fatal("expected no model indicator when Model is empty")
	}
}
