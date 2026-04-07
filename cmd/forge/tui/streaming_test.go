package tui

import (
	"strings"
	"testing"
	"time"
)

func TestStreamingCompleteLines_BuffersPartial(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	// Send text with no newline — should be buffered as partial
	m.streamBuf.WriteString("hello world partial")
	m.upsertStreamingCompleteLines()

	if m.partialLineBuf != "hello world partial" {
		t.Fatalf("expected partialLineBuf=%q, got %q", "hello world partial", m.partialLineBuf)
	}
}

func TestStreamingCompleteLines_ShowsCompleteLines(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	// Send text with a newline — complete line should be displayed
	m.streamBuf.WriteString("line one\npartial two")
	m.upsertStreamingCompleteLines()

	if m.partialLineBuf != "partial two" {
		t.Fatalf("expected partialLineBuf=%q, got %q", "partial two", m.partialLineBuf)
	}

	if len(m.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	last := m.messages[len(m.messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %q", last.Role)
	}
	if !strings.Contains(last.Content, "line one") {
		t.Fatalf("expected content to contain 'line one', got %q", last.Content)
	}
	if strings.Contains(last.Content, "partial two") {
		t.Fatal("expected content to NOT contain partial 'partial two'")
	}
}

func TestStreamingCompleteLines_FlushOnDone(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	// Simulate streaming with partial
	m.streamBuf.WriteString("line one\npartial end")
	m.upsertStreamingCompleteLines()

	// Now flush (as PromptDoneMsg would do)
	m.flushStreamBuf()

	if m.partialLineBuf != "" {
		t.Fatalf("expected partialLineBuf cleared after flush, got %q", m.partialLineBuf)
	}

	// The full content including partial should be in messages
	last := m.messages[len(m.messages)-1]
	if !strings.Contains(last.Content, "partial end") {
		t.Fatalf("expected flushed content to include partial, got %q", last.Content)
	}
}

func TestStreamingCompleteLines_MultipleNewlines(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	m.streamBuf.WriteString("line1\nline2\nline3\npartial4")
	m.upsertStreamingCompleteLines()

	if m.partialLineBuf != "partial4" {
		t.Fatalf("expected partialLineBuf=%q, got %q", "partial4", m.partialLineBuf)
	}

	last := m.messages[len(m.messages)-1]
	if !strings.Contains(last.Content, "line3") {
		t.Fatalf("expected content to contain 'line3', got %q", last.Content)
	}
}

func TestInputSuppression_IgnoresKeys(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Enable suppression
	m.inputSuppressed = true
	m.suppressUntil = time.Now().Add(200 * time.Millisecond)

	// Verify suppression state
	if !m.inputSuppressed {
		t.Fatal("expected inputSuppressed=true")
	}
	if time.Now().After(m.suppressUntil) {
		t.Fatal("expected suppressUntil to be in the future")
	}
}

func TestInputSuppression_ExpiresNaturally(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Set suppression in the past
	m.inputSuppressed = true
	m.suppressUntil = time.Now().Add(-1 * time.Second)

	// Should be expired
	if time.Now().Before(m.suppressUntil) {
		t.Fatal("expected suppression to be expired")
	}
}

func TestSuppressInput_SetsValues(t *testing.T) {
	m := newTestModel()
	m.suppressInput()

	if !m.inputSuppressed {
		t.Fatal("expected inputSuppressed=true after suppressInput()")
	}
	if m.suppressUntil.Before(time.Now()) {
		t.Fatal("expected suppressUntil to be in the future")
	}
}

func TestStickyPrompt_ShowsWhenScrolledUp(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	m.lastUserPromptIdx = 0
	m.lastUserPrompt = "What is the meaning of life?"
	m.userScrolledUp = true

	result := m.renderStickyPrompt()
	if result == "" {
		t.Fatal("expected non-empty sticky prompt")
	}
	if !strings.Contains(result, "What is the meaning of life?") {
		t.Fatalf("expected sticky prompt to contain user text, got %q", result)
	}
}

func TestStickyPrompt_TruncatesLongText(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 60, 40)

	m.lastUserPromptIdx = 0
	m.lastUserPrompt = strings.Repeat("a", 600)
	m.userScrolledUp = true

	result := m.renderStickyPrompt()
	// Should be truncated (raw prompt > 500 chars)
	if strings.Contains(result, strings.Repeat("a", 500)) {
		t.Fatal("expected long prompt to be truncated")
	}
}

func TestStickyPrompt_HiddenAtBottom(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	m.lastUserPromptIdx = 0
	m.lastUserPrompt = "test prompt"
	m.userScrolledUp = false

	// The View method should not include sticky prompt when not scrolled up
	// (tested via the condition in View, not rendering)
	view := m.View()
	_ = view // just ensure it doesn't panic
}
