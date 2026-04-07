package custom

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scriptPath(name string) string {
	abs, _ := filepath.Abs(filepath.Join("testdata", "scripts", name))
	return abs
}

func TestRunCommand_EchoInput(t *testing.T) {
	input := json.RawMessage(`{"query": "SELECT 1"}`)
	result := RunCommand(context.Background(), scriptPath("echo_input.sh"), input, "TestTool", ".", 10)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, stderr = %q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, `"query"`) {
		t.Errorf("Stdout = %q, expected JSON input echoed back", result.Stdout)
	}
}

func TestRunCommand_StructuredOutput(t *testing.T) {
	result := RunCommand(context.Background(), scriptPath("structured_ok.sh"), json.RawMessage(`{}`), "TestTool", ".", 10)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d", result.ExitCode)
	}

	content, isError := ParseOutput(result, 10)
	if isError {
		t.Error("isError = true, want false")
	}
	if content != "operation successful" {
		t.Errorf("content = %q, want %q", content, "operation successful")
	}
}

func TestRunCommand_StructuredError(t *testing.T) {
	result := RunCommand(context.Background(), scriptPath("structured_error.sh"), json.RawMessage(`{}`), "TestTool", ".", 10)
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d", result.ExitCode)
	}

	content, isError := ParseOutput(result, 10)
	if !isError {
		t.Error("isError = false, want true")
	}
	if content != "something went wrong" {
		t.Errorf("content = %q, want %q", content, "something went wrong")
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	result := RunCommand(context.Background(), scriptPath("fail_exit1.sh"), json.RawMessage(`{}`), "TestTool", ".", 10)
	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}

	content, isError := ParseOutput(result, 10)
	if !isError {
		t.Error("isError = false, want true for non-zero exit")
	}
	if !strings.Contains(content, "something failed") {
		t.Errorf("content = %q, expected stderr message", content)
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	start := time.Now()
	result := RunCommand(context.Background(), scriptPath("slow_timeout.sh"), json.RawMessage(`{}`), "TestTool", ".", 1)
	elapsed := time.Since(start)

	if !result.TimedOut {
		t.Error("TimedOut = false, want true")
	}

	content, isError := ParseOutput(result, 1)
	if !isError {
		t.Error("isError = false, want true for timeout")
	}
	if !strings.Contains(content, "timed out after 1s") {
		t.Errorf("content = %q, expected timeout message", content)
	}

	// Should not have waited the full 60s.
	if elapsed > 5*time.Second {
		t.Errorf("elapsed = %v, expected ~1s", elapsed)
	}
}

func TestParseOutput_PlainText(t *testing.T) {
	result := ExecResult{Stdout: "hello world", ExitCode: 0}
	content, isError := ParseOutput(result, 10)
	if isError {
		t.Error("isError = true for plain text")
	}
	if content != "hello world" {
		t.Errorf("content = %q", content)
	}
}

func TestParseOutput_EmptyOutput(t *testing.T) {
	result := ExecResult{Stdout: "", ExitCode: 0}
	content, isError := ParseOutput(result, 10)
	if isError {
		t.Error("isError = true")
	}
	if content != "(no output)" {
		t.Errorf("content = %q, want %q", content, "(no output)")
	}
}

func TestParseOutput_StderrAppended(t *testing.T) {
	result := ExecResult{Stdout: "output", Stderr: "warning: something", ExitCode: 0}
	content, isError := ParseOutput(result, 10)
	if isError {
		t.Error("isError = true")
	}
	if !strings.Contains(content, "output") || !strings.Contains(content, "[stderr]") || !strings.Contains(content, "warning: something") {
		t.Errorf("content = %q, expected stdout + stderr", content)
	}
}

func TestParseOutput_NonZeroExitStderrFallback(t *testing.T) {
	// Non-zero exit, no stderr, stdout used as error message.
	result := ExecResult{Stdout: "fallback error", Stderr: "", ExitCode: 1}
	content, isError := ParseOutput(result, 10)
	if !isError {
		t.Error("isError = false, want true")
	}
	if content != "fallback error" {
		t.Errorf("content = %q", content)
	}
}

func TestParseOutput_NonZeroExitNoOutput(t *testing.T) {
	result := ExecResult{Stdout: "", Stderr: "", ExitCode: 42}
	content, isError := ParseOutput(result, 10)
	if !isError {
		t.Error("isError = false, want true")
	}
	if !strings.Contains(content, "exited with code 42") {
		t.Errorf("content = %q", content)
	}
}
