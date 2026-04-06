package bash

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestProgressEvent_Fields(t *testing.T) {
	// Verify the struct fields exist and are the right types
	e := ProgressEvent{
		Output:     "hello",
		TotalLines: 1,
		TotalBytes: 5,
		ElapsedMs:  42,
	}
	if e.Output != "hello" {
		t.Errorf("Output = %q, want 'hello'", e.Output)
	}
	if e.TotalLines != 1 {
		t.Errorf("TotalLines = %d, want 1", e.TotalLines)
	}
	if e.TotalBytes != 5 {
		t.Errorf("TotalBytes = %d, want 5", e.TotalBytes)
	}
	if e.ElapsedMs != 42 {
		t.Errorf("ElapsedMs = %d, want 42", e.ElapsedMs)
	}
}

func TestExecOptions_HasOnProgress(t *testing.T) {
	// Verify OnProgress field exists on ExecOptions
	var called bool
	opts := ExecOptions{
		OnProgress: func(e ProgressEvent) {
			called = true
		},
	}
	if opts.OnProgress == nil {
		t.Error("OnProgress should be settable on ExecOptions")
	}
	opts.OnProgress(ProgressEvent{})
	if !called {
		t.Error("OnProgress callback should be callable")
	}
}

func TestExecCommand_ProgressCallback_FiresDuringExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	var callCount atomic.Int32
	var lastOutput string

	// Run a command that produces output over ~1.5 seconds.
	// The progress goroutine fires every 1s, so we expect at least 1 callback.
	result := ExecCommand(context.Background(),
		`echo "start" && sleep 1.2 && echo "end"`,
		ExecOptions{
			TimeoutMs: 5000,
			OnProgress: func(e ProgressEvent) {
				callCount.Add(1)
				lastOutput = e.Output
			},
		},
	)

	if result.ExitCode != 0 {
		t.Fatalf("command failed: exit %d, output: %s", result.ExitCode, result.Stdout)
	}

	if callCount.Load() == 0 {
		t.Error("expected at least one progress callback during a 1.2s command")
	}

	_ = lastOutput // used to verify type
}

func TestExecCommand_ProgressCallback_ContainsTotalBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	var lastEvent ProgressEvent

	ExecCommand(context.Background(),
		`printf "hello\n" && sleep 1.1`,
		ExecOptions{
			TimeoutMs: 5000,
			OnProgress: func(e ProgressEvent) {
				lastEvent = e
			},
		},
	)

	if lastEvent.TotalBytes == 0 {
		t.Error("expected TotalBytes > 0 in progress event")
	}
	if lastEvent.ElapsedMs <= 0 {
		t.Error("expected ElapsedMs > 0 in progress event")
	}
}

func TestExecCommand_ProgressCallback_NotRequiredForNormalExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	// Without OnProgress set, the command should still work normally
	result := ExecCommand(context.Background(), "echo hello", ExecOptions{})
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0 without OnProgress, got %d", result.ExitCode)
	}
}

func TestExecCommand_ProgressCallback_TotalLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	var lastEvent ProgressEvent

	ExecCommand(context.Background(),
		`printf "line1\nline2\nline3\n" && sleep 1.1`,
		ExecOptions{
			TimeoutMs: 5000,
			OnProgress: func(e ProgressEvent) {
				lastEvent = e
			},
		},
	)

	if lastEvent.TotalLines < 3 {
		t.Errorf("expected TotalLines >= 3 for 3-line output, got %d", lastEvent.TotalLines)
	}
}

func TestSafeBuffer_ThreadSafety(t *testing.T) {
	// Verify safeBuffer can be written and read concurrently without data races.
	// Run with -race flag to catch issues.
	buf := &safeBuffer{}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				buf.Write([]byte("x")) //nolint
			}
		}
	}()

	// Read concurrently
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		_ = buf.String()
	}

	close(done)
}
