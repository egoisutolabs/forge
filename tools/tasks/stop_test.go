package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

func newStopTool(t *testing.T) (*StopTool, *TaskStore) {
	t.Helper()
	s := newStore(t)
	return &StopTool{store: s}, s
}

// --- interface compliance ---

func TestStopTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &StopTool{}
}

func TestStopTool_Name(t *testing.T) {
	if (&StopTool{}).Name() != "TaskStop" {
		t.Error("wrong name")
	}
}

func TestStopTool_IsReadOnly(t *testing.T) {
	if (&StopTool{}).IsReadOnly(nil) {
		t.Error("TaskStop should not be read-only")
	}
}

func TestStopTool_IsConcurrencySafe(t *testing.T) {
	if !(&StopTool{}).IsConcurrencySafe(nil) {
		t.Error("TaskStop should be concurrency-safe")
	}
}

// --- ValidateInput ---

func TestStopTool_ValidateInput_Valid(t *testing.T) {
	err := (&StopTool{}).ValidateInput(json.RawMessage(`{"task_id":"1"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStopTool_ValidateInput_MissingTaskID(t *testing.T) {
	err := (&StopTool{}).ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing task_id")
	}
}

// --- Execute ---

func TestStopTool_Execute_SetsKilledStatus(t *testing.T) {
	tool, store := newStopTool(t)
	store.SaveTask(makeTask("1", "Running task", StatusInProgress)) //nolint:errcheck

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"task_id":"1"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "killed") {
		t.Errorf("expected 'killed' in message, got: %s", result.Content)
	}

	task, _ := store.LoadTask("1")
	if task.Status != StatusKilled {
		t.Errorf("expected status 'killed', got %q", task.Status)
	}
}

func TestStopTool_Execute_AlreadyKilled(t *testing.T) {
	tool, store := newStopTool(t)
	task := makeTask("1", "T", StatusKilled)
	store.SaveTask(task) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"task_id":"1"}`), nil)
	if result.IsError {
		t.Fatalf("unexpected error for already-killed task: %s", result.Content)
	}
	if !strings.Contains(result.Content, "already killed") {
		t.Errorf("expected 'already killed' in message, got: %s", result.Content)
	}
}

func TestStopTool_Execute_NotFound(t *testing.T) {
	tool, _ := newStopTool(t)
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"task_id":"99"}`), nil)
	if !result.IsError {
		t.Error("expected error for not-found task")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result.Content)
	}
}

func TestStopTool_Execute_InvalidJSON(t *testing.T) {
	tool, _ := newStopTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid JSON")
	}
}

func TestStopTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&StopTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}
