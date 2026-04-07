package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ListTool implements the TaskList tool — returns all non-internal tasks.
type ListTool struct {
	store *TaskStore
}

func (t *ListTool) Name() string                             { return "TaskList" }
func (t *ListTool) IsReadOnly(_ json.RawMessage) bool        { return true }
func (t *ListTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *ListTool) Description() string {
	return "List all tasks. Returns task IDs, subjects, statuses, owners, and blocker IDs."
}

func (t *ListTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {},
		"required": []
	}`)
}

func (t *ListTool) ValidateInput(input json.RawMessage) error {
	// No required fields.
	var v map[string]any
	return json.Unmarshal(input, &v)
}

func (t *ListTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *ListTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	store, err := t.getStore()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Storage error: %s", err), IsError: true}, nil
	}

	all, err := store.ListTasks()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to list tasks: %s", err), IsError: true}, nil
	}

	// Collect IDs of completed tasks so we can filter them from blockedBy.
	completedIDs := map[string]bool{}
	for _, task := range all {
		if task.Status == StatusCompleted || task.Status == StatusKilled {
			completedIDs[task.ID] = true
		}
	}

	var lines []string
	for _, task := range all {
		// Filter out internal tasks (metadata._internal == true).
		if isInternal(task) {
			continue
		}

		// Filter completed IDs from blockedBy.
		blockedBy := filterCompletedIDs(task.BlockedBy, completedIDs)

		line := fmt.Sprintf("#%s [%s] %s", task.ID, task.Status, task.Subject)
		if task.Owner != "" {
			line += fmt.Sprintf(" (owner: %s)", task.Owner)
		}
		if len(blockedBy) > 0 {
			line += fmt.Sprintf(" [blocked by: %s]", strings.Join(blockedBy, ", "))
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return &models.ToolResult{Content: "No tasks found."}, nil
	}
	return &models.ToolResult{Content: strings.Join(lines, "\n")}, nil
}

func (t *ListTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}

// isInternal returns true if the task should be hidden from TaskList output.
// Tasks with metadata._internal == true are considered internal.
func isInternal(task *Task) bool {
	if task.Metadata == nil {
		return false
	}
	v, ok := task.Metadata["_internal"]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// filterCompletedIDs returns ids with any completed task ID removed.
func filterCompletedIDs(ids []string, completed map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if !completed[id] {
			out = append(out, id)
		}
	}
	return out
}
