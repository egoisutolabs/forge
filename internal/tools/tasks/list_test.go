package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/tools"
)

func newListTool(t *testing.T) (*ListTool, *TaskStore) {
	t.Helper()
	s := newStore(t)
	return &ListTool{store: s}, s
}

// --- interface compliance ---

func TestListTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &ListTool{}
}

func TestListTool_Name(t *testing.T) {
	if (&ListTool{}).Name() != "TaskList" {
		t.Error("wrong name")
	}
}

func TestListTool_IsReadOnly(t *testing.T) {
	if !(&ListTool{}).IsReadOnly(nil) {
		t.Error("TaskList should be read-only")
	}
}

func TestListTool_IsConcurrencySafe(t *testing.T) {
	if !(&ListTool{}).IsConcurrencySafe(nil) {
		t.Error("TaskList should be concurrency-safe")
	}
}

// --- Execute ---

func TestListTool_Execute_Empty(t *testing.T) {
	tool, _ := newListTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No tasks found") {
		t.Errorf("expected empty message, got: %s", result.Content)
	}
}

func TestListTool_Execute_ShowsTasks(t *testing.T) {
	tool, store := newListTool(t)
	store.SaveTask(makeTask("1", "First task", StatusPending))     //nolint:errcheck
	store.SaveTask(makeTask("2", "Second task", StatusInProgress)) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "First task") {
		t.Errorf("expected first task, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Second task") {
		t.Errorf("expected second task, got: %s", result.Content)
	}
}

func TestListTool_Execute_FilterInternalTasks(t *testing.T) {
	tool, store := newListTool(t)
	store.SaveTask(makeTask("1", "Visible task", StatusPending)) //nolint:errcheck
	internal := makeTask("2", "Internal task", StatusPending)
	internal.Metadata = map[string]any{"_internal": true}
	store.SaveTask(internal) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if strings.Contains(result.Content, "Internal task") {
		t.Errorf("internal task should be filtered, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Visible task") {
		t.Errorf("visible task should appear, got: %s", result.Content)
	}
}

func TestListTool_Execute_FilterCompletedBlockers(t *testing.T) {
	tool, store := newListTool(t)
	store.SaveTask(makeTask("1", "Done task", StatusCompleted)) //nolint:errcheck
	task2 := makeTask("2", "Blocked task", StatusPending)
	task2.BlockedBy = []string{"1", "99"} // "1" is completed, "99" is pending (doesn't exist)
	store.SaveTask(task2)                 //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	// "99" should still appear in blockedBy, but "1" should not
	if strings.Contains(result.Content, "blocked by: 1") {
		t.Errorf("completed task ID should be filtered from blockedBy: %s", result.Content)
	}
}

func TestListTool_Execute_ShowsOwner(t *testing.T) {
	tool, store := newListTool(t)
	task := makeTask("1", "Owned", StatusInProgress)
	task.Owner = "agent-2"
	store.SaveTask(task) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if !strings.Contains(result.Content, "agent-2") {
		t.Errorf("expected owner in output, got: %s", result.Content)
	}
}

func TestListTool_Execute_ShowsBlockedBy(t *testing.T) {
	tool, store := newListTool(t)
	task := makeTask("1", "Waiting", StatusPending)
	task.BlockedBy = []string{"2"}
	store.SaveTask(task)                                    //nolint:errcheck
	store.SaveTask(makeTask("2", "Blocker", StatusPending)) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if !strings.Contains(result.Content, "blocked by") {
		t.Errorf("expected blocked-by in output, got: %s", result.Content)
	}
}

func TestListTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&ListTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}

func TestIsInternal(t *testing.T) {
	task := makeTask("1", "T", StatusPending)
	if isInternal(task) {
		t.Error("task without metadata should not be internal")
	}
	task.Metadata = map[string]any{"_internal": true}
	if !isInternal(task) {
		t.Error("task with _internal=true should be internal")
	}
	task.Metadata["_internal"] = false
	if isInternal(task) {
		t.Error("task with _internal=false should not be internal")
	}
}
