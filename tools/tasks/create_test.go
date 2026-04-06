package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

func newCreateTool(t *testing.T) *CreateTool {
	t.Helper()
	return &CreateTool{store: newStore(t)}
}

// --- interface compliance ---

func TestCreateTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &CreateTool{}
}

func TestCreateTool_Name(t *testing.T) {
	if (&CreateTool{}).Name() != "TaskCreate" {
		t.Error("wrong name")
	}
}

func TestCreateTool_IsReadOnly(t *testing.T) {
	if (&CreateTool{}).IsReadOnly(nil) {
		t.Error("TaskCreate should not be read-only")
	}
}

func TestCreateTool_IsConcurrencySafe(t *testing.T) {
	if !(&CreateTool{}).IsConcurrencySafe(nil) {
		t.Error("TaskCreate should be concurrency-safe")
	}
}

// --- ValidateInput ---

func TestCreateTool_ValidateInput_Valid(t *testing.T) {
	err := (&CreateTool{}).ValidateInput(json.RawMessage(`{"subject":"S","description":"D"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateTool_ValidateInput_MissingSubject(t *testing.T) {
	err := (&CreateTool{}).ValidateInput(json.RawMessage(`{"description":"D"}`))
	if err == nil {
		t.Error("expected error for missing subject")
	}
}

func TestCreateTool_ValidateInput_MissingDescription(t *testing.T) {
	err := (&CreateTool{}).ValidateInput(json.RawMessage(`{"subject":"S"}`))
	if err == nil {
		t.Error("expected error for missing description")
	}
}

func TestCreateTool_ValidateInput_BadJSON(t *testing.T) {
	err := (&CreateTool{}).ValidateInput(json.RawMessage(`{bad`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// --- Execute ---

func TestCreateTool_Execute_Basic(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"Fix bug","description":"Needs fixing"}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task #1 created") {
		t.Errorf("expected creation message, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Fix bug") {
		t.Errorf("expected subject in message, got: %s", result.Content)
	}
}

func TestCreateTool_Execute_IDs_Increment(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"T","description":"D"}`)
	for i := 1; i <= 3; i++ {
		result, _ := tool.Execute(context.Background(), input, nil)
		if result.IsError {
			t.Fatalf("iter %d: tool error: %s", i, result.Content)
		}
		expected := "#" + string(rune('0'+i))
		if !strings.Contains(result.Content, expected) {
			t.Errorf("iter %d: expected ID %s in %q", i, expected, result.Content)
		}
	}
}

func TestCreateTool_Execute_TaskPersistedWithPendingStatus(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"X","description":"Y"}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	task, err := tool.store.LoadTask("1")
	if err != nil || task == nil {
		t.Fatalf("expected task to be persisted: err=%v task=%v", err, task)
	}
	if task.Status != StatusPending {
		t.Errorf("expected status pending, got %q", task.Status)
	}
	if task.Subject != "X" {
		t.Errorf("expected subject X, got %q", task.Subject)
	}
}

func TestCreateTool_Execute_WithMetadata(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"M","description":"D","metadata":{"key":"val"}}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	task, _ := tool.store.LoadTask("1")
	if task.Metadata["key"] != "val" {
		t.Errorf("expected metadata key=val, got %v", task.Metadata)
	}
}

func TestCreateTool_Execute_WithActiveForm(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"S","description":"D","activeForm":"Running tests"}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	task, _ := tool.store.LoadTask("1")
	if task.ActiveForm != "Running tests" {
		t.Errorf("expected activeForm 'Running tests', got %q", task.ActiveForm)
	}
}

func TestCreateTool_Execute_EmptyBlocksAndBlockedBy(t *testing.T) {
	tool := newCreateTool(t)
	input := json.RawMessage(`{"subject":"S","description":"D"}`)
	tool.Execute(context.Background(), input, nil) //nolint:errcheck

	task, _ := tool.store.LoadTask("1")
	if task.Blocks == nil || len(task.Blocks) != 0 {
		t.Errorf("expected empty blocks, got %v", task.Blocks)
	}
	if task.BlockedBy == nil || len(task.BlockedBy) != 0 {
		t.Errorf("expected empty blockedBy, got %v", task.BlockedBy)
	}
}

func TestCreateTool_Execute_InvalidJSON(t *testing.T) {
	tool := newCreateTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid JSON")
	}
}

func TestCreateTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&CreateTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}

func TestCreateTool_InputSchema_Valid(t *testing.T) {
	schema := (&CreateTool{}).InputSchema()
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}
}
