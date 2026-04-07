package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// GetTool implements the TaskGet tool — retrieves a single task by ID.
type GetTool struct {
	store *TaskStore
}

type getInput struct {
	TaskID string `json:"taskId"`
}

func (t *GetTool) Name() string                             { return "TaskGet" }
func (t *GetTool) IsReadOnly(_ json.RawMessage) bool        { return true }
func (t *GetTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *GetTool) Description() string {
	return "Get the details of a specific task by its ID."
}

func (t *GetTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId": {
				"type": "string",
				"description": "The task ID to retrieve"
			}
		},
		"required": ["taskId"]
	}`)
}

func (t *GetTool) ValidateInput(input json.RawMessage) error {
	var in getInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		return fmt.Errorf("taskId is required")
	}
	return nil
}

func (t *GetTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *GetTool) Execute(_ context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in getInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	store, err := t.getStore()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Storage error: %s", err), IsError: true}, nil
	}

	task, err := store.LoadTask(in.TaskID)
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to load task: %s", err), IsError: true}, nil
	}
	if task == nil {
		return &models.ToolResult{Content: fmt.Sprintf("task not found: %s", in.TaskID), IsError: true}, nil
	}

	return &models.ToolResult{Content: formatTask(task)}, nil
}

func (t *GetTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}

// formatTask renders a Task as a human-readable multi-line string.
func formatTask(task *Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "#%s [%s] %s", task.ID, task.Status, task.Subject)
	if task.Owner != "" {
		fmt.Fprintf(&sb, " (owner: %s)", task.Owner)
	}
	if task.Description != "" {
		fmt.Fprintf(&sb, "\n%s", task.Description)
	}
	if len(task.Blocks) > 0 {
		fmt.Fprintf(&sb, "\nBlocks: %s", strings.Join(task.Blocks, ", "))
	}
	if len(task.BlockedBy) > 0 {
		fmt.Fprintf(&sb, "\nBlocked by: %s", strings.Join(task.BlockedBy, ", "))
	}
	return sb.String()
}
