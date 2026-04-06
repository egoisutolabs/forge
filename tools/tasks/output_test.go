package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/tools"
)

func newOutputTool(t *testing.T) (*OutputTool, *TaskStore) {
	t.Helper()
	s := newStore(t)
	return &OutputTool{store: s}, s
}

// --- interface compliance ---

func TestOutputTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &OutputTool{}
}

func TestOutputTool_Name(t *testing.T) {
	if (&OutputTool{}).Name() != "TaskOutput" {
		t.Error("wrong name")
	}
}

func TestOutputTool_IsReadOnly(t *testing.T) {
	if !(&OutputTool{}).IsReadOnly(nil) {
		t.Error("TaskOutput should be read-only")
	}
}

func TestOutputTool_IsConcurrencySafe(t *testing.T) {
	if !(&OutputTool{}).IsConcurrencySafe(nil) {
		t.Error("TaskOutput should be concurrency-safe")
	}
}

// --- ValidateInput ---

func TestOutputTool_ValidateInput_Valid(t *testing.T) {
	err := (&OutputTool{}).ValidateInput(json.RawMessage(`{"task_id":"1"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputTool_ValidateInput_MissingTaskID(t *testing.T) {
	err := (&OutputTool{}).ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing task_id")
	}
}

func TestOutputTool_ValidateInput_TimeoutTooLarge(t *testing.T) {
	err := (&OutputTool{}).ValidateInput(json.RawMessage(`{"task_id":"1","timeout":999999}`))
	if err == nil {
		t.Error("expected error for timeout > max")
	}
}

func TestOutputTool_ValidateInput_NegativeTimeout(t *testing.T) {
	err := (&OutputTool{}).ValidateInput(json.RawMessage(`{"task_id":"1","timeout":-1}`))
	if err == nil {
		t.Error("expected error for negative timeout")
	}
}

// --- Execute: non-blocking ---

func TestOutputTool_Execute_NonBlocking_NotReady(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Pending task", StatusPending)) //nolint:errcheck

	input := json.RawMessage(`{"task_id":"1","block":false}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "not_ready") {
		t.Errorf("expected 'not_ready', got: %s", result.Content)
	}
}

func TestOutputTool_Execute_NonBlocking_Completed(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Done task", StatusCompleted)) //nolint:errcheck

	input := json.RawMessage(`{"task_id":"1","block":false}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "success") {
		t.Errorf("expected 'success', got: %s", result.Content)
	}
}

func TestOutputTool_Execute_NonBlocking_Killed(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Killed task", StatusKilled)) //nolint:errcheck

	input := json.RawMessage(`{"task_id":"1","block":false}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !strings.Contains(result.Content, "success") {
		t.Errorf("expected 'success' for killed status, got: %s", result.Content)
	}
}

// --- Execute: blocking ---

func TestOutputTool_Execute_Blocking_AlreadyComplete(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Instant done", StatusCompleted)) //nolint:errcheck

	input := json.RawMessage(`{"task_id":"1","block":true,"timeout":5000}`)
	start := time.Now()
	result, err := tool.Execute(context.Background(), input, nil)
	elapsed := time.Since(start)

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: err=%v result=%v", err, result)
	}
	if !strings.Contains(result.Content, "success") {
		t.Errorf("expected 'success', got: %s", result.Content)
	}
	// Should return almost immediately since task is already complete.
	if elapsed > 2*time.Second {
		t.Errorf("blocking on already-complete task took too long: %v", elapsed)
	}
}

func TestOutputTool_Execute_Blocking_Timeout(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Never completes", StatusInProgress)) //nolint:errcheck

	// Use a very short timeout (200ms) so the test is fast.
	input := json.RawMessage(`{"task_id":"1","block":true,"timeout":200}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "timeout") {
		t.Errorf("expected 'timeout', got: %s", result.Content)
	}
}

func TestOutputTool_Execute_Blocking_CompletedWhileWaiting(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Will complete", StatusInProgress)) //nolint:errcheck

	// Complete the task after a short delay in a goroutine.
	go func() {
		time.Sleep(150 * time.Millisecond)
		task, _ := store.LoadTask("1")
		if task != nil {
			task.Status = StatusCompleted
			store.SaveTask(task) //nolint:errcheck
		}
	}()

	input := json.RawMessage(`{"task_id":"1","block":true,"timeout":5000}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: err=%v result=%v", err, result)
	}
	if !strings.Contains(result.Content, "success") {
		t.Errorf("expected 'success', got: %s", result.Content)
	}
}

func TestOutputTool_Execute_ContextCancellation(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("1", "Long task", StatusInProgress)) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	tctx := &tools.ToolContext{AbortCtx: ctx}
	input := json.RawMessage(`{"task_id":"1","block":true,"timeout":30000}`)
	result, err := tool.Execute(ctx, input, tctx)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// After context cancellation, should return timeout or not_ready.
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
}

func TestOutputTool_Execute_NotFound(t *testing.T) {
	tool, _ := newOutputTool(t)
	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"task_id":"99","block":false}`), nil)
	if !result.IsError {
		t.Error("expected error for not-found task")
	}
}

func TestOutputTool_Execute_ResponseContainsTaskID(t *testing.T) {
	tool, store := newOutputTool(t)
	store.SaveTask(makeTask("42", "The answer", StatusCompleted)) //nolint:errcheck

	result, _ := tool.Execute(context.Background(), json.RawMessage(`{"task_id":"42","block":false}`), nil)
	if !strings.Contains(result.Content, "42") {
		t.Errorf("expected task_id '42' in response, got: %s", result.Content)
	}
}

func TestOutputTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := (&OutputTool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}
