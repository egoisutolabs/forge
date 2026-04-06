package bash

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

func TestBashTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestBashTool_Name(t *testing.T) {
	bt := &Tool{}
	if bt.Name() != "Bash" {
		t.Errorf("Name() = %q, want 'Bash'", bt.Name())
	}
}

func TestBashTool_Execute_SimpleCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	bt := &Tool{}
	tctx := &tools.ToolContext{Cwd: "/tmp"}

	input := json.RawMessage(`{"command": "echo hello from bash"}`)
	result, err := bt.Execute(context.Background(), input, tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello from bash") {
		t.Errorf("output = %q, want to contain 'hello from bash'", result.Content)
	}
}

func TestBashTool_Execute_ExitCodeReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	bt := &Tool{}
	tctx := &tools.ToolContext{Cwd: "/tmp"}

	input := json.RawMessage(`{"command": "echo failed && exit 1"}`)
	result, err := bt.Execute(context.Background(), input, tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-zero exit code")
	}
	if !strings.Contains(result.Content, "exit code 1") {
		t.Errorf("should report exit code, got: %s", result.Content)
	}
}

func TestBashTool_Execute_CustomTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	bt := &Tool{}
	tctx := &tools.ToolContext{Cwd: "/tmp"}

	input := json.RawMessage(`{"command": "sleep 30", "timeout": 500}`)
	result, err := bt.Execute(context.Background(), input, tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for timeout")
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("should report timeout, got: %s", result.Content)
	}
}

func TestBashTool_Execute_UsesCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	bt := &Tool{}
	tctx := &tools.ToolContext{Cwd: "/tmp"}

	input := json.RawMessage(`{"command": "pwd"}`)
	result, err := bt.Execute(context.Background(), input, tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "/tmp") {
		t.Errorf("expected /tmp in output, got: %s", result.Content)
	}
}

func TestBashTool_Execute_TruncatesLargeOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash tests require unix")
	}

	bt := &Tool{}
	tctx := &tools.ToolContext{Cwd: "/tmp"}

	// Generate >30K chars of output (python is fast, guaranteed available)
	input := json.RawMessage(`{"command": "python3 -c \"print('x' * 40000)\""}`)
	result, err := bt.Execute(context.Background(), input, tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Error("expected truncation indicator in output")
	}
}

func TestBashTool_ValidateInput_MissingCommand(t *testing.T) {
	bt := &Tool{}

	err := bt.ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected validation error for missing command")
	}
}

func TestBashTool_ValidateInput_EmptyCommand(t *testing.T) {
	bt := &Tool{}

	err := bt.ValidateInput(json.RawMessage(`{"command": ""}`))
	if err == nil {
		t.Error("expected validation error for empty command")
	}
}

func TestBashTool_ValidateInput_ValidCommand(t *testing.T) {
	bt := &Tool{}

	err := bt.ValidateInput(json.RawMessage(`{"command": "echo hello"}`))
	if err == nil || err != nil {
		// Should not error
	}
	err = bt.ValidateInput(json.RawMessage(`{"command": "echo hello"}`))
	if err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestBashTool_CheckPermissions_ReadOnly(t *testing.T) {
	bt := &Tool{}

	input := json.RawMessage(`{"command": "ls -la"}`)
	decision, err := bt.CheckPermissions(input, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("expected Allow for read-only command, got %v", decision.Behavior)
	}
}

func TestBashTool_CheckPermissions_WriteCommand(t *testing.T) {
	bt := &Tool{}

	input := json.RawMessage(`{"command": "rm -rf /tmp/test"}`)
	decision, err := bt.CheckPermissions(input, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("expected Ask for write command, got %v", decision.Behavior)
	}
}

func TestBashTool_IsConcurrencySafe_ReadOnly(t *testing.T) {
	bt := &Tool{}

	if !bt.IsConcurrencySafe(json.RawMessage(`{"command": "ls"}`)) {
		t.Error("read-only command should be concurrency safe")
	}
}

func TestBashTool_IsConcurrencySafe_WriteCommand(t *testing.T) {
	bt := &Tool{}

	if bt.IsConcurrencySafe(json.RawMessage(`{"command": "rm foo"}`)) {
		t.Error("write command should NOT be concurrency safe")
	}
}

func TestBashTool_IsReadOnly(t *testing.T) {
	bt := &Tool{}

	if !bt.IsReadOnly(json.RawMessage(`{"command": "cat foo.go"}`)) {
		t.Error("cat should be read-only")
	}
	if bt.IsReadOnly(json.RawMessage(`{"command": "rm foo.go"}`)) {
		t.Error("rm should not be read-only")
	}
}
