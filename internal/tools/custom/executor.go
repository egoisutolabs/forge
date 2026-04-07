package custom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoisutolabs/forge/internal/tools/bash"
)

// ExecResult is the raw output of running a custom tool command.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// RunCommand executes the tool's shell command with JSON input on stdin.
// It uses SanitizedEnv for the environment and kills the process group on timeout.
func RunCommand(ctx context.Context, command string, input json.RawMessage, toolName, cwd string, timeoutSec int) ExecResult {
	timeout := time.Duration(timeoutSec) * time.Second

	cmd := exec.Command("bash", "-c", command)
	if cwd != "" {
		cmd.Dir = cwd
	}
	cmd.Stdin = bytes.NewReader(input)

	// Use sanitized env plus custom tool variables.
	env := bash.SanitizedEnv()
	env = append(env, "FORGE_TOOL_NAME="+toolName, "FORGE_CWD="+cwd)
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Process group so we can kill all children on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return ExecResult{
			Stderr:   err.Error(),
			ExitCode: 126,
		}
	}

	done := make(chan struct{})
	var didTimeout atomic.Bool

	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			didTimeout.Store(true)
			killProcessGroup(cmd)
		case <-ctx.Done():
			killProcessGroup(cmd)
		}
	}()

	err := cmd.Wait()
	close(done)

	result := ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if didTimeout.Load() {
		result.TimedOut = true
		result.ExitCode = 137
		return result
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	return result
}

// killProcessGroup sends SIGKILL to the entire process group.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		cmd.Process.Kill()
		return
	}
	syscall.Kill(-pgid, syscall.SIGKILL)
}

// structuredOutput is the optional JSON format a tool command can return.
type structuredOutput struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

// ParseOutput interprets the raw execution result into tool result content.
func ParseOutput(result ExecResult, timeoutSec int) (content string, isError bool) {
	if result.TimedOut {
		return fmt.Sprintf("Custom tool timed out after %ds", timeoutSec), true
	}

	if result.ExitCode != 0 {
		msg := result.Stderr
		if msg == "" {
			msg = result.Stdout
		}
		if msg == "" {
			msg = fmt.Sprintf("exited with code %d", result.ExitCode)
		}
		return msg, true
	}

	// Try structured JSON output first.
	var structured structuredOutput
	if err := json.Unmarshal([]byte(result.Stdout), &structured); err == nil && structured.Content != "" {
		content = structured.Content
		isError = structured.IsError
	} else {
		content = result.Stdout
	}

	// Append stderr if present (warnings, debug output).
	if result.Stderr != "" {
		content += "\n[stderr]\n" + result.Stderr
	}

	if content == "" {
		content = "(no output)"
	}

	return content, isError
}
