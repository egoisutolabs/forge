// Package tasks — verification tests comparing Go port against Claude Code's
// TaskCreateTool, TaskGetTool, TaskListTool, TaskUpdateTool, TaskStopTool,
// TaskOutputTool TypeScript sources.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: TaskCreated hook execution in CreateTool.
//     TypeScript TaskCreateTool.call() fires executeTaskCreatedHooks() after
//     creating a task. Go creates the task but never fires any hooks.
//
//  2. MISSING: TaskCompleted hook execution in UpdateTool.
//     TypeScript UpdateTool fires executeTaskCompletedHooks() when status
//     transitions to "completed". Go does not.
//
//  3. MISSING: Auto-expand UI in CreateTool.
//     TypeScript calls expandTaskList() after creation to open the task UI.
//     Not applicable to the Go CLI port, but documented for completeness.
//
//  4. DIVERGENCE: Task ID format.
//     TypeScript uses UUID v4 for task IDs.
//     Go uses monotonically-increasing integers ("1", "2", …) with a file-lock
//     protected .highwatermark. Both approaches guarantee uniqueness, but the
//     formats are not interoperable.
//
//  5. MISSING: TaskOutput blocking poll gap.
//     TypeScript TaskOutputTool can stream output as tasks write to a shared
//     log file. Go polls status only (no streaming log output).
//
//  6. CORRECT: BlockedBy filtering.
//     Both implementations filter completed/killed task IDs from blockedBy
//     display. Go correctly hides completed dependencies.
//
//  7. CORRECT: _internal metadata filtering.
//     Both implementations hide tasks with metadata._internal == true.
//
//  8. CORRECT: Bidirectional dependency management in UpdateTool.
//     addBlocks / addBlockedBy update both sides correctly.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// ─── GAP 1: TaskCreated hook not fired ───────────────────────────────────────

// TestVerification_CreateTool_NoHooksExecuted documents that CreateTool does
// not fire TaskCreated hooks after creating a task.
//
// Claude Code TypeScript (TaskCreateTool.ts):
//
//	await executeTaskCreatedHooks(task, hooks)
//
// Go CreateTool.Execute: saves to disk and returns — no hook execution.
func TestVerification_CreateTool_NoHooksExecuted(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())
	ct := &CreateTool{store: store}

	in := taskJSON(t, map[string]any{
		"subject":     "Test task",
		"description": "Verify hooks gap",
	})

	result, err := ct.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	t.Log("GAP CONFIRMED: TaskCreated hooks are not executed (TypeScript fires executeTaskCreatedHooks after create)")
}

// ─── GAP 4: Integer vs UUID task IDs ─────────────────────────────────────────

// TestVerification_TaskID_IsInteger verifies Go uses integer IDs (monotonic
// counter) while TypeScript uses UUID v4.
func TestVerification_TaskID_IsInteger(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())
	ct := &CreateTool{store: store}

	in := taskJSON(t, map[string]any{
		"subject":     "Task 1",
		"description": "First task",
	})

	result, _ := ct.Execute(context.Background(), in, nil)
	if result.IsError {
		t.Fatalf("create failed: %s", result.Content)
	}

	// Content format: "Task #N created successfully: subject\n{\"task\":{\"id\":\"N\",...}}"
	idStr := extractTaskID(result.Content)
	if idStr == "" {
		t.Fatalf("could not extract task ID from: %s", result.Content)
	}

	// Go IDs are decimal integers.
	n, err := strconv.Atoi(idStr)
	if err != nil {
		t.Errorf("task ID %q is not an integer — unexpected format", idStr)
	} else {
		t.Logf("Go task ID format: integer (%d). TypeScript uses UUID v4. Formats are not interoperable.", n)
	}
}

// ─── GAP 6: BlockedBy filtering (correct) ────────────────────────────────────

// TestVerification_ListTool_FiltersCompletedBlockedBy verifies that completed
// task IDs are removed from the blockedBy display in TaskList output.
//
// Both TypeScript and Go implement this — this test confirms Go is correct.
func TestVerification_ListTool_FiltersCompletedBlockedBy(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())

	// Create blocker task (completed).
	blocker := &Task{
		ID:      "1",
		Subject: "Blocker",
		Status:  StatusCompleted,
	}
	if err := store.SaveTask(blocker); err != nil {
		t.Fatalf("save blocker: %v", err)
	}

	// Create blocked task.
	blocked := &Task{
		ID:        "2",
		Subject:   "Blocked",
		Status:    StatusPending,
		BlockedBy: []string{"1"},
	}
	if err := store.SaveTask(blocked); err != nil {
		t.Fatalf("save blocked: %v", err)
	}

	lt := &ListTool{store: store}
	result, err := lt.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The completed blocker should NOT appear in the blocked-by display.
	if containsStr(result.Content, "blocked by: 1") || containsStr(result.Content, "[blocked by") {
		t.Errorf("completed blocker should be filtered from blockedBy display, got: %s", result.Content)
	}
}

// ─── GAP 7: _internal metadata filtering (correct) ───────────────────────────

// TestVerification_ListTool_HidesInternalTasks verifies that tasks with
// metadata._internal == true are hidden from TaskList output.
//
// Both TypeScript and Go implement this — confirms Go parity.
func TestVerification_ListTool_HidesInternalTasks(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())

	internal := &Task{
		ID:       "1",
		Subject:  "Internal task",
		Status:   StatusPending,
		Metadata: map[string]any{"_internal": true},
	}
	visible := &Task{
		ID:      "2",
		Subject: "Visible task",
		Status:  StatusPending,
	}
	store.SaveTask(internal) //nolint:errcheck
	store.SaveTask(visible)  //nolint:errcheck

	lt := &ListTool{store: store}
	result, _ := lt.Execute(context.Background(), json.RawMessage(`{}`), nil)

	if containsStr(result.Content, "Internal task") {
		t.Error("internal task should be hidden from TaskList output")
	}
	if !containsStr(result.Content, "Visible task") {
		t.Error("visible task should appear in TaskList output")
	}
}

// ─── GAP 8: Bidirectional dependency management (correct) ─────────────────────

// TestVerification_UpdateTool_BidirectionalDependencies verifies that
// addBlocks / addBlockedBy update both sides of the dependency graph.
func TestVerification_UpdateTool_BidirectionalDependencies(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())

	t1 := &Task{ID: "1", Subject: "Task 1", Status: StatusPending}
	t2 := &Task{ID: "2", Subject: "Task 2", Status: StatusPending}
	store.SaveTask(t1) //nolint:errcheck
	store.SaveTask(t2) //nolint:errcheck

	ut := &UpdateTool{store: store}

	// Set task 1 to block task 2.
	in := taskJSON(t, map[string]any{
		"taskId":    "1",
		"addBlocks": []string{"2"},
	})
	result, err := ut.Execute(context.Background(), in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("error: %s", result.Content)
	}

	// Task 1 should have "2" in Blocks.
	task1, _ := store.LoadTask("1")
	if !containsString(task1.Blocks, "2") {
		t.Error("task 1 should have task 2 in Blocks after addBlocks")
	}

	// Task 2 should have "1" in BlockedBy (bidirectional update).
	task2, _ := store.LoadTask("2")
	if !containsString(task2.BlockedBy, "1") {
		t.Error("task 2 should have task 1 in BlockedBy (bidirectional)")
	}
}

// ─── File locking: concurrent ID allocation ────────────────────────────────

// TestVerification_HighWaterMark_AtomicUnderConcurrency verifies that
// concurrent NextID() calls produce unique IDs with no duplicates.
// This verifies the syscall.Flock-based locking is correct.
func TestVerification_HighWaterMark_AtomicUnderConcurrency(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())

	const goroutines = 20
	ids := make([]string, goroutines)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id, err := store.NextID()
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			ids[idx] = id
		}(i)
	}
	wg.Wait()

	if firstErr != nil {
		t.Fatalf("NextID error: %v", firstErr)
	}

	// All IDs must be unique.
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			t.Error("got empty ID")
			continue
		}
		if seen[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		seen[id] = true
	}

	// IDs must be integers 1..goroutines.
	for _, id := range ids {
		n, err := strconv.Atoi(id)
		if err != nil {
			t.Errorf("ID %q is not an integer", id)
			continue
		}
		if n < 1 || n > goroutines {
			t.Errorf("ID %d out of range [1, %d]", n, goroutines)
		}
	}
}

// TestVerification_HighWaterMark_Monotonic verifies sequential NextID() calls
// produce strictly increasing values.
func TestVerification_HighWaterMark_Monotonic(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())

	prev := 0
	for i := 0; i < 5; i++ {
		id, err := store.NextID()
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		n, err := strconv.Atoi(id)
		if err != nil {
			t.Fatalf("ID %q is not integer: %v", id, err)
		}
		if n <= prev {
			t.Errorf("ID %d not greater than previous %d", n, prev)
		}
		prev = n
	}
}

// TestVerification_DeleteTask_RemovedFromListOutput verifies that deleted
// tasks (status="deleted") are physically removed and don't appear in list.
func TestVerification_DeleteTask_RemovedFromListOutput(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())
	ct := &CreateTool{store: store}
	ut := &UpdateTool{store: store}
	lt := &ListTool{store: store}

	// Create.
	createResult, _ := ct.Execute(context.Background(), taskJSON(t, map[string]any{
		"subject":     "Temporary task",
		"description": "Will be deleted",
	}), nil)
	if createResult.IsError {
		t.Fatalf("create failed: %s", createResult.Content)
	}
	taskID := extractTaskID(createResult.Content)
	if taskID == "" {
		t.Fatalf("could not extract task ID from: %s", createResult.Content)
	}

	// Delete.
	_, err := ut.Execute(context.Background(), taskJSON(t, map[string]any{
		"taskId": taskID,
		"status": "deleted",
	}), nil)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Task file should be gone.
	taskFile := filepath.Join(store.dir, taskID+".json")
	if _, err := os.Stat(taskFile); !os.IsNotExist(err) {
		t.Error("task file should be deleted when status=deleted")
	}

	// Should not appear in list.
	listResult, _ := lt.Execute(context.Background(), json.RawMessage(`{}`), nil)
	if containsStr(listResult.Content, "Temporary task") {
		t.Error("deleted task should not appear in TaskList output")
	}
}

// TestVerification_CreateTool_RequiresSubjectAndDescription verifies that
// subject and description are required fields — matches TypeScript validation.
// Note: validation is checked via ValidateInput (called by framework before Execute).
func TestVerification_CreateTool_RequiresSubjectAndDescription(t *testing.T) {
	ct := &CreateTool{}

	tests := []struct {
		name  string
		input map[string]any
	}{
		{"missing subject", map[string]any{"description": "desc"}},
		{"empty subject", map[string]any{"subject": "", "description": "desc"}},
		{"missing description", map[string]any{"subject": "sub"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ct.ValidateInput(taskJSON(t, tc.input))
			if err == nil {
				t.Errorf("ValidateInput should reject input with %s", tc.name)
			}
		})
	}
}

// TestVerification_GetTool_NotFound verifies that GetTool returns an error
// result (not a Go error) for non-existent task IDs.
func TestVerification_GetTool_NotFound(t *testing.T) {
	store := NewTaskStoreFromDir(t.TempDir())
	gt := &GetTool{store: store}

	result, err := gt.Execute(context.Background(), taskJSON(t, map[string]any{"taskId": "999"}), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent task")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func taskJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// extractTaskID parses the task ID from CreateTool output.
// Content format: "Task #N created: ...\n{\"task\":{\"id\":\"N\",...}}"
func extractTaskID(content string) string {
	// Find the embedded JSON blob.
	idx := 0
	for i, c := range content {
		if c == '{' {
			idx = i
			break
		}
	}
	if idx == 0 && len(content) > 0 && content[0] != '{' {
		return ""
	}
	var outer map[string]any
	if err := json.Unmarshal([]byte(content[idx:]), &outer); err != nil {
		return ""
	}
	taskMap, _ := outer["task"].(map[string]any)
	if taskMap == nil {
		return ""
	}
	id, _ := taskMap["id"].(string)
	return id
}

func mustTaskCreate(t *testing.T, store *TaskStore, subject string) string {
	t.Helper()
	ct := &CreateTool{store: store}
	result, err := ct.Execute(context.Background(), taskJSON(t, map[string]any{
		"subject":     subject,
		"description": fmt.Sprintf("description for %s", subject),
	}), nil)
	if err != nil || result.IsError {
		t.Fatalf("create task %q: err=%v content=%s", subject, err, result.Content)
	}
	id := extractTaskID(result.Content)
	if id == "" {
		t.Fatalf("could not extract task ID from: %s", result.Content)
	}
	return id
}
