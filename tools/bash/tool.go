package bash

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// toolInput is the JSON schema for BashTool input.
type toolInput struct {
	Command         string `json:"command"`
	Timeout         int    `json:"timeout,omitempty"`           // ms, 0 = default (120s)
	Description     string `json:"description,omitempty"`       // for UI display
	RunInBackground bool   `json:"run_in_background,omitempty"` // spawn as background task
}

// Tool implements the Bash tool — execute shell commands.
//
// This is the Go equivalent of Claude Code's BashTool.
// Key behaviors:
//   - Executes via bash -c '<command>'
//   - stdout+stderr merged
//   - Process group kill on timeout (kills all children)
//   - Output truncated at 30K chars (configurable)
//   - Read-only commands auto-approved
type Tool struct{}

func (t *Tool) Name() string        { return "Bash" }
func (t *Tool) Description() string { return "Execute a bash command" }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The command to execute"
			},
			"timeout": {
				"type": "number",
				"description": "Optional timeout in milliseconds (max 600000)"
			},
			"description": {
				"type": "string",
				"description": "Clear, concise description of what this command does"
			},
			"run_in_background": {
				"type": "boolean",
				"description": "Set to true to run this command in the background. Returns immediately with a task ID; output is written to ~/.forge/tasks/{taskId}.output"
			}
		},
		"required": ["command"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.Command) == "" {
		return fmt.Errorf("command is required and cannot be empty")
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, tctx *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}

	if IsReadOnly(in.Command) {
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}

	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  fmt.Sprintf("Run command: %s", in.Command),
	}, nil
}

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	cwd := ""
	if tctx != nil {
		cwd = tctx.Cwd
	}

	if in.RunInBackground {
		task, err := StartBackground(in.Command, ExecOptions{Cwd: cwd})
		if err != nil {
			return &models.ToolResult{
				Content: fmt.Sprintf("Failed to start background task: %s", err),
				IsError: true,
			}, nil
		}
		content := fmt.Sprintf(
			"Background task started.\nTask ID: %s\nOutput file: %s",
			task.TaskID, task.OutputFile,
		)
		return &models.ToolResult{Content: content}, nil
	}

	result := ExecCommand(ctx, in.Command, ExecOptions{
		Cwd:       cwd,
		TimeoutMs: in.Timeout,
	})

	return formatResult(result, generateID()), nil
}

// generateID returns a unique ID for naming persisted output files.
func generateID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck // crypto/rand.Read never errors on supported platforms
	return "bash-" + hex.EncodeToString(b)
}

func (t *Tool) IsConcurrencySafe(input json.RawMessage) bool {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return false
	}
	return IsReadOnly(in.Command)
}

func (t *Tool) IsReadOnly(input json.RawMessage) bool {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return false
	}
	return IsReadOnly(in.Command)
}

// formatResult converts an ExecResult into a ToolResult for the model.
// toolUseId is used to name the persisted output file when output is large.
func formatResult(r ExecResult, toolUseId string) *models.ToolResult {
	originalOutput := r.Stdout
	output := TruncateOutput(r.Stdout, DefaultMaxOutput)
	returnCodeInterp := interpretExitCode(r.ExitCode)

	if r.TimedOut {
		msg := fmt.Sprintf("Command timed out after %dms\n%s", r.DurationMs, output)
		return &models.ToolResult{Content: msg, IsError: true, ReturnCodeInterpretation: returnCodeInterp}
	}

	if r.Interrupted {
		msg := fmt.Sprintf("Command interrupted\n%s", output)
		return &models.ToolResult{Content: msg, IsError: true, ReturnCodeInterpretation: returnCodeInterp}
	}

	if r.ExitCode != 0 {
		msg := output
		if msg == "" {
			msg = fmt.Sprintf("Command failed with exit code %d", r.ExitCode)
		} else {
			msg = fmt.Sprintf("%s\n(exit code %d)", msg, r.ExitCode)
		}
		return &models.ToolResult{Content: msg, IsError: true, ReturnCodeInterpretation: returnCodeInterp}
	}

	// Detect image output: if stdout is a base64 data URI under the size cap,
	// mark the result as an image so callers can render it appropriately.
	if isImageOutput(originalOutput) && len(originalOutput) < MaxImageFileSize {
		return &models.ToolResult{
			Content:                  originalOutput,
			IsImage:                  true,
			ReturnCodeInterpretation: returnCodeInterp,
		}
	}

	// Persist the full output to disk when it exceeds the size threshold,
	// and return a structured preview so the model can use the Read tool
	// to access the full content if needed.
	if len(originalOutput) > MaxResultSizeChars {
		if path, preview, err := PersistOutput(originalOutput, toolUseId); err == nil && path != "" {
			content := fmt.Sprintf(
				"<persisted-output>\nFull output saved to: %s\nPreview:\n%s\n</persisted-output>",
				path, preview,
			)
			return &models.ToolResult{Content: content, ReturnCodeInterpretation: returnCodeInterp}
		}
	}

	if output == "" {
		output = "(no output)"
	}

	return &models.ToolResult{Content: output, ReturnCodeInterpretation: returnCodeInterp}
}
