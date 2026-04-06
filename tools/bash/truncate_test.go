package bash

import (
	"strings"
	"testing"
)

func TestTruncateOutput_ShortOutput(t *testing.T) {
	output := "hello world"
	result := TruncateOutput(output, DefaultMaxOutput)

	if result != output {
		t.Errorf("short output should not be truncated, got %q", result)
	}
}

func TestTruncateOutput_ExactlyAtLimit(t *testing.T) {
	output := strings.Repeat("x", DefaultMaxOutput)
	result := TruncateOutput(output, DefaultMaxOutput)

	if result != output {
		t.Error("output at exactly the limit should not be truncated")
	}
}

func TestTruncateOutput_TruncatesLongOutput(t *testing.T) {
	// 100 lines, each 500 chars → 50,000 chars total, over 30K limit
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, strings.Repeat("a", 499))
	}
	output := strings.Join(lines, "\n")

	result := TruncateOutput(output, DefaultMaxOutput)

	if len(result) >= len(output) {
		t.Error("output should be shorter after truncation")
	}
	if !strings.Contains(result, "lines truncated") {
		t.Error("truncated output should contain truncation indicator")
	}
}

func TestTruncateOutput_KeepsStart(t *testing.T) {
	output := "FIRST LINE\n" + strings.Repeat("filler line\n", 5000)
	result := TruncateOutput(output, DefaultMaxOutput)

	if !strings.HasPrefix(result, "FIRST LINE\n") {
		t.Error("truncation should keep the start of output")
	}
}

func TestTruncateOutput_CountsTruncatedLines(t *testing.T) {
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "line")
	}
	output := strings.Join(lines, "\n")

	result := TruncateOutput(output, 100)

	// Should mention how many lines were truncated
	if !strings.Contains(result, "truncated") {
		t.Error("should contain truncation message")
	}
}

func TestTruncateOutput_EmptyOutput(t *testing.T) {
	result := TruncateOutput("", DefaultMaxOutput)
	if result != "" {
		t.Errorf("empty output should stay empty, got %q", result)
	}
}

func TestTruncateOutput_CustomLimit(t *testing.T) {
	output := strings.Repeat("x", 200)
	result := TruncateOutput(output, 50)

	if len(result) > 200 { // truncated part + message
		t.Errorf("should be truncated to ~50 chars + message, got len=%d", len(result))
	}
	if !strings.Contains(result, "truncated") {
		t.Error("should contain truncation indicator")
	}
}
