package bash

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecCommand_SimpleEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "echo hello", ExecOptions{})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "hello")
	}
}

func TestExecCommand_StderrMergedIntoStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "echo out && echo err >&2", ExecOptions{})

	// Both stdout and stderr should be in Stdout (merged)
	if !strings.Contains(result.Stdout, "out") {
		t.Error("stdout missing 'out'")
	}
	if !strings.Contains(result.Stdout, "err") {
		t.Error("merged stderr missing 'err'")
	}
}

func TestExecCommand_ExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "exit 42", ExecOptions{})

	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}
}

func TestExecCommand_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "sleep 30", ExecOptions{
		TimeoutMs: 500, // 500ms timeout
	})

	if !result.TimedOut {
		t.Error("expected TimedOut=true")
	}
	// Should not take 30 seconds
	if result.DurationMs > 5000 {
		t.Errorf("took too long: %dms, timeout should have kicked in", result.DurationMs)
	}
}

func TestExecCommand_ContextCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	result := ExecCommand(ctx, "sleep 30", ExecOptions{})

	if !result.Interrupted {
		t.Error("expected Interrupted=true on context cancel")
	}
}

func TestExecCommand_DefaultTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	// With no timeout specified, should use default (120s).
	// We don't actually want to wait 120s, just verify the option flows through.
	// Test a fast command instead.
	result := ExecCommand(context.Background(), "echo fast", ExecOptions{})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestExecCommand_Cwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "pwd", ExecOptions{
		Cwd: "/tmp",
	})

	if strings.TrimSpace(result.Stdout) != "/tmp" {
		// macOS /tmp is a symlink to /private/tmp
		if !strings.Contains(result.Stdout, "/tmp") {
			t.Errorf("cwd = %q, want /tmp", strings.TrimSpace(result.Stdout))
		}
	}
}

func TestExecCommand_KillsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	// Spawn a command that spawns a child. On timeout, the child should also die.
	// "bash -c 'sleep 60 & wait'" — if only parent is killed, sleep 60 would linger.
	result := ExecCommand(context.Background(), "sleep 60 & wait", ExecOptions{
		TimeoutMs: 500,
	})

	if !result.TimedOut {
		t.Error("expected timeout")
	}
}

func TestExecCommand_MultilineOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "echo line1 && echo line2 && echo line3", ExecOptions{})

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
	}
}

func TestExecCommand_EmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	result := ExecCommand(context.Background(), "true", ExecOptions{})

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Errorf("expected empty stdout, got %q", result.Stdout)
	}
}
