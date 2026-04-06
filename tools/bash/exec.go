package bash

import (
	"context"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// Timeout constants matching Claude Code's timeouts.ts.
const (
	DefaultTimeoutMs = 120_000 // 2 minutes
	MaxTimeoutMs     = 600_000 // 10 minutes
)

// ExecOptions configures how a command is executed.
type ExecOptions struct {
	Cwd        string                // working directory (empty = inherit)
	TimeoutMs  int                   // 0 = use DefaultTimeoutMs
	OnProgress func(e ProgressEvent) // optional: called ~every second with partial output
}

// ExecResult is the outcome of running a shell command.
type ExecResult struct {
	Stdout      string // merged stdout+stderr
	ExitCode    int
	TimedOut    bool
	Interrupted bool // context was cancelled
	DurationMs  int64
}

// ExecCommand runs a command in bash and returns the result.
//
// Key behaviors matching Claude Code's Shell.ts:
//   - Shell: bash -c '<command>'
//   - stdout and stderr merged into single stream
//   - Process group created (Setpgid) so timeout kills all children
//   - On timeout: SIGKILL to entire process group
//   - On context cancel: SIGKILL to entire process group
func ExecCommand(ctx context.Context, command string, opts ExecOptions) ExecResult {
	timeoutMs := opts.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = DefaultTimeoutMs
	}
	if timeoutMs > MaxTimeoutMs {
		timeoutMs = MaxTimeoutMs
	}

	timeoutDur := time.Duration(timeoutMs) * time.Millisecond

	start := time.Now()

	// We do NOT use exec.CommandContext because it only kills the parent process,
	// leaving child processes alive (and holding the pipe open, causing Wait to hang).
	// Instead, we manage the timeout ourselves and kill the entire process group.
	cmd := exec.Command("bash", "-c", command)
	cmd.Env = SanitizedEnv()

	// Merge stdout and stderr into a mutex-protected buffer so the progress
	// goroutine can read partial output concurrently without data races.
	var outBuf safeBuffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}

	// Create a new process group so we can kill all children on timeout.
	// This is the Go equivalent of Claude Code's tree-kill + detached mode.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return ExecResult{
			Stdout:     err.Error(),
			ExitCode:   126, // "command cannot execute" (matches Claude Code)
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Monitor for timeout or context cancellation in a goroutine.
	// When either fires, kill the entire process group.
	// Use atomics to communicate back to avoid data races.
	done := make(chan struct{})
	var didTimeout atomic.Bool
	var didInterrupt atomic.Bool

	// If a progress callback is registered, poll the output buffer every ~1s.
	// progressDone is closed when the goroutine has fully exited; we wait on it
	// after closing done so all callback writes happen-before ExecCommand returns.
	var progressDone <-chan struct{}
	if opts.OnProgress != nil {
		progressDone = startProgressPoller(&outBuf, start, done, opts.OnProgress)
	}

	go func() {
		timer := time.NewTimer(timeoutDur)
		defer timer.Stop()

		select {
		case <-done:
			// Process finished normally
		case <-timer.C:
			// Timeout — kill entire process group
			didTimeout.Store(true)
			killProcessGroup(cmd)
		case <-ctx.Done():
			// Parent context cancelled — kill entire process group
			didInterrupt.Store(true)
			killProcessGroup(cmd)
		}
	}()

	// Wait for completion (now unblocked because process group is killed)
	err := cmd.Wait()
	close(done)

	// Wait for the progress goroutine to fully exit so any writes inside the
	// OnProgress callback are visible to the caller (happens-before guarantee).
	if progressDone != nil {
		<-progressDone
	}

	duration := time.Since(start).Milliseconds()

	result := ExecResult{
		Stdout:     outBuf.String(),
		DurationMs: duration,
	}

	if didTimeout.Load() {
		result.TimedOut = true
		result.ExitCode = 137
	} else if didInterrupt.Load() {
		result.Interrupted = true
		result.ExitCode = 137
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	return result
}

// killProcessGroup sends SIGKILL to the entire process group.
// This ensures child processes spawned by the command are also killed.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		// Fallback: kill just the process
		cmd.Process.Kill()
		return
	}
	// Kill the entire group (negative PID = process group)
	syscall.Kill(-pgid, syscall.SIGKILL)
}
