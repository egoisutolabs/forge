package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

func newGetTool(t *testing.T) (*GetTool, *TaskStore) {
	t.Helper()
	s := newStore(t)
	return &GetTool{store: s}, s
}

// --- interface compliance ---

func TestGetTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &GetTool{}
}

func TestGetTool_Name(t *testing.T) {
	if (&GetTool{}).Name() != "TaskGet" {
		t.Error("wrong name")
	}
}

func TestGetTool_IsReadOnly(t *testing.T) {
	if !(&GetTool{}).IsReadOnly(nil) {
		t.Error("TaskGet should be read-only")
	}
}

func TestGetTool_IsConcurrencySafe(t *testing.T) {
	if !(&GetTool{}).IsConcurrencySafe(nil) {
		t.Error("TaskGet should be concurrency-safe")
	}
}

// --- ValidateInput ---

func TestGetTool_ValidateInput_Valid(t *testing.T) {
	err := (&GetTool{}).ValidateInput(json.RawMessage(`{"taskId":"1"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetTool_ValidateInput_MissingTaskID(t *testing.T) {
	err := (&GetTool{}).ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing taskId")
	}
}

// --- Execute ---

func TestGetTool_Execute_Found(t *testing.T) {
	tool, store := newGetTool(t)
	store.SaveTask(makeTask("1", "My Task", StatusInProgress)) //nolint:errcheck

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"taskId":"1"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "#1") {
		t.Errorf("expected task ID in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "My Task") {
		t.Errorf("expected subject in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "in_progress") {
		t.Errorf("expected status in output, got: %s", result.Content)
	}
}

func TestGetTool_Execute_NotFound(t *testing.T) {
	tool, _ := newGetTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"taskId":"99"}`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for not-found task")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result.Content)
	}
}

func TestGetTool_Execute_ShowsOwner(t *testing.T) {
	tool, store := newGetTool(t)
	task := makeTask("1", "Owned Task", StatusPending)
	task.Owner = "agent-1"
	store.SaveTask(task) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"taskId":"1"}`), nil)
	if !strings.Contains(result.Content, "agent-1") {
		t.Errorf("expected owner in output, got: %s", result.Content)
	}
}

func TestGetTool_Execute_ShowsBlocks(t *testing.T) {
	tool, store := newGetTool(t)
	task := makeTask("1", "Blocking Task", StatusPending)
	task.Blocks = []string{"2", "3"}
	task.BlockedBy = []string{"0"}
	store.SaveTask(task) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"taskId":"1"}`), nil)
	if !strings.Contains(result.Content, "Blocks") {
		t.Errorf("expected blocks in output, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Blocked by") {
		t.Errorf("expected blockedBy in output, got: %s", result.Content)
	}
}

func TestGetTool_Execute_InvalidJSON(t *testing.T) {
	tool, _ := newGetTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid JSON")
	}
}

func TestGetTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&GetTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}
