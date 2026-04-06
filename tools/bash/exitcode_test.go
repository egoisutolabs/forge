package bash

import (
	"strings"
	"testing"
)

func TestInterpretExitCode_Success(t *testing.T) {
	result := interpretExitCode(0)
	if !strings.Contains(strings.ToLower(result), "success") {
		t.Errorf("exit code 0 should indicate success, got %q", result)
	}
}

func TestInterpretExitCode_GeneralError(t *testing.T) {
	result := interpretExitCode(1)
	if !strings.Contains(strings.ToLower(result), "general") && !strings.Contains(strings.ToLower(result), "error") {
		t.Errorf("exit code 1 should indicate general error, got %q", result)
	}
}

func TestInterpretExitCode_Misuse(t *testing.T) {
	result := interpretExitCode(2)
	if !strings.Contains(strings.ToLower(result), "misuse") {
		t.Errorf("exit code 2 should indicate misuse, got %q", result)
	}
}

func TestInterpretExitCode_CannotExecute(t *testing.T) {
	result := interpretExitCode(126)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "cannot") && !strings.Contains(lower, "execute") && !strings.Contains(lower, "permission") {
		t.Errorf("exit code 126 should indicate cannot execute, got %q", result)
	}
}

func TestInterpretExitCode_NotFound(t *testing.T) {
	result := interpretExitCode(127)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "not found") && !strings.Contains(lower, "command") {
		t.Errorf("exit code 127 should indicate command not found, got %q", result)
	}
}

func TestInterpretExitCode_SIGINT(t *testing.T) {
	result := interpretExitCode(130) // 128 + 2 (SIGINT)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "sigint") && !strings.Contains(lower, "interrupt") {
		t.Errorf("exit code 130 should indicate SIGINT, got %q", result)
	}
}

func TestInterpretExitCode_SIGKILL(t *testing.T) {
	result := interpretExitCode(137) // 128 + 9 (SIGKILL)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "sigkill") && !strings.Contains(lower, "kill") {
		t.Errorf("exit code 137 should indicate SIGKILL, got %q", result)
	}
}

func TestInterpretExitCode_SIGTERM(t *testing.T) {
	result := interpretExitCode(143) // 128 + 15 (SIGTERM)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "sigterm") && !strings.Contains(lower, "terminat") {
		t.Errorf("exit code 143 should indicate SIGTERM, got %q", result)
	}
}

func TestInterpretExitCode_GenericSignal(t *testing.T) {
	// 128 + 6 = SIGABRT, not a named special case
	result := interpretExitCode(134)
	lower := strings.ToLower(result)
	if !strings.Contains(lower, "signal") && !strings.Contains(lower, "6") {
		t.Errorf("exit code 134 should mention signal 6, got %q", result)
	}
}

func TestInterpretExitCode_Unknown(t *testing.T) {
	// An arbitrary non-zero exit code
	result := interpretExitCode(42)
	if result == "" {
		t.Error("interpretExitCode should return non-empty string for any exit code")
	}
}

func TestFormatResult_IncludesReturnCodeInterpretation(t *testing.T) {
	r := ExecResult{
		Stdout:   "some output",
		ExitCode: 0,
	}
	result := formatResult(r, "test-id")
	if result.ReturnCodeInterpretation == "" {
		t.Error("formatResult should set ReturnCodeInterpretation")
	}
}

func TestFormatResult_ReturnCodeInterpretation_Error(t *testing.T) {
	r := ExecResult{
		Stdout:   "failed output",
		ExitCode: 127,
	}
	result := formatResult(r, "test-id")
	lower := strings.ToLower(result.ReturnCodeInterpretation)
	if !strings.Contains(lower, "not found") && !strings.Contains(lower, "command") {
		t.Errorf("exit code 127 interpretation = %q, want 'command not found'", result.ReturnCodeInterpretation)
	}
}
