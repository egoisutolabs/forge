package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

func newUpdateTool(t *testing.T) (*UpdateTool, *TaskStore) {
	t.Helper()
	s := newStore(t)
	return &UpdateTool{store: s}, s
}

// --- interface compliance ---

func TestUpdateTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &UpdateTool{}
}

func TestUpdateTool_Name(t *testing.T) {
	if (&UpdateTool{}).Name() != "TaskUpdate" {
		t.Error("wrong name")
	}
}

func TestUpdateTool_IsReadOnly(t *testing.T) {
	if (&UpdateTool{}).IsReadOnly(nil) {
		t.Error("TaskUpdate should not be read-only")
	}
}

// --- ValidateInput ---

func TestUpdateTool_ValidateInput_Valid(t *testing.T) {
	err := (&UpdateTool{}).ValidateInput(json.RawMessage(`{"taskId":"1"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateTool_ValidateInput_MissingTaskID(t *testing.T) {
	err := (&UpdateTool{}).ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing taskId")
	}
}

// --- Execute: basic field updates ---

func TestUpdateTool_Execute_UpdateSubject(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "Old subject", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","subject":"New subject"}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: err=%v result=%v", err, result)
	}

	task, _ := store.LoadTask("1")
	if task.Subject != "New subject" {
		t.Errorf("expected 'New subject', got %q", task.Subject)
	}
	if !strings.Contains(result.Content, "subject") {
		t.Errorf("expected 'subject' in update message, got: %s", result.Content)
	}
}

func TestUpdateTool_Execute_UpdateStatus(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "T", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","status":"in_progress"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}

	task, _ := store.LoadTask("1")
	if task.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %q", task.Status)
	}
}

func TestUpdateTool_Execute_UpdateOwner(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "T", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","owner":"agent-X"}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	task, _ := store.LoadTask("1")
	if task.Owner != "agent-X" {
		t.Errorf("expected owner 'agent-X', got %q", task.Owner)
	}
}

func TestUpdateTool_Execute_NoChanges(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "Same", StatusPending)) //nolint:errcheck

	// Provide the same subject as what's already there.
	input := json.RawMessage(`{"taskId":"1","subject":"Same"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "no changes") {
		t.Errorf("expected 'no changes' message, got: %s", result.Content)
	}
}

// --- Execute: deleted status ---

func TestUpdateTool_Execute_DeletesTask(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "T", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","status":"deleted"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deleted") {
		t.Errorf("expected deletion message, got: %s", result.Content)
	}
	task, _ := store.LoadTask("1")
	if task != nil {
		t.Error("expected task to be deleted from disk")
	}
}

// --- Execute: not found ---

func TestUpdateTool_Execute_NotFound(t *testing.T) {
	tool, _ := newUpdateTool(t)
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"taskId":"99","status":"completed"}`), nil)
	if !result.IsError {
		t.Error("expected error for not-found task")
	}
}

// --- Execute: addBlocks / addBlockedBy ---

func TestUpdateTool_Execute_AddBlocks_Bidirectional(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "Blocker", StatusPending)) //nolint:errcheck
	store.SaveTask(makeTask("2", "Blocked", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","addBlocks":["2"]}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	t1, _ := store.LoadTask("1")
	if !containsString(t1.Blocks, "2") {
		t.Errorf("task 1 should have '2' in blocks, got %v", t1.Blocks)
	}
	t2, _ := store.LoadTask("2")
	if !containsString(t2.BlockedBy, "1") {
		t.Errorf("task 2 should have '1' in blockedBy, got %v", t2.BlockedBy)
	}
}

func TestUpdateTool_Execute_AddBlockedBy_Bidirectional(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "Child", StatusPending))  //nolint:errcheck
	store.SaveTask(makeTask("2", "Parent", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","addBlockedBy":["2"]}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	t1, _ := store.LoadTask("1")
	if !containsString(t1.BlockedBy, "2") {
		t.Errorf("task 1 should have '2' in blockedBy, got %v", t1.BlockedBy)
	}
	t2, _ := store.LoadTask("2")
	if !containsString(t2.Blocks, "1") {
		t.Errorf("task 2 should have '1' in blocks, got %v", t2.Blocks)
	}
}

func TestUpdateTool_Execute_AddBlocks_NoDuplicates(t *testing.T) {
	tool, store := newUpdateTool(t)
	store.SaveTask(makeTask("1", "T", StatusPending)) //nolint:errcheck
	store.SaveTask(makeTask("2", "T", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"taskId":"1","addBlocks":["2"]}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck
	tool.Execute(context.Background(), input, nil) //nolint:errcheck // second time should not duplicate

	t1, _ := store.LoadTask("1")
	count := 0
	for _, b := range t1.Blocks {
		if b == "2" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one '2' in blocks, got %d: %v", count, t1.Blocks)
	}
}

// --- Execute: metadata merge ---

func TestUpdateTool_Execute_MetadataMerge(t *testing.T) {
	tool, store := newUpdateTool(t)
	task := makeTask("1", "T", StatusPending)
	task.Metadata = map[string]any{"keep": "yes", "remove": "old"}
	store.SaveTask(task) //nolint:errcheck

	// Add a key, remove "remove" by setting to null.
	input := json.RawMessage(`{"taskId":"1","metadata":{"new":"val","remove":null}}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	got, _ := store.LoadTask("1")
	if got.Metadata["keep"] != "yes" {
		t.Errorf("expected keep=yes, got %v", got.Metadata["keep"])
	}
	if got.Metadata["new"] != "val" {
		t.Errorf("expected new=val, got %v", got.Metadata["new"])
	}
	if _, exists := got.Metadata["remove"]; exists {
		t.Error("expected 'remove' key to be deleted")
	}
}

func TestUpdateTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&UpdateTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}
