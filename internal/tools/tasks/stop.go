package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// StopTool implements the TaskStop tool — marks a task as killed.
//
// Actual process termination will be wired up when background-task integration
// lands. For now this simply transitions the task to status="killed".
type StopTool struct {
	store *TaskStore
}

type stopInput struct {
	TaskID string `json:"task_id"`
}

func (t *StopTool) Name() string                             { return "TaskStop" }
func (t *StopTool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (t *StopTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *StopTool) Description() string {
	return "Stop a running task. Sets its status to \"killed\"."
}

func (t *StopTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "string",
				"description": "ID of the task to stop"
			}
		},
		"required": ["task_id"]
	}`)
}

func (t *StopTool) ValidateInput(input json.RawMessage) error {
	var in stopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	return nil
}

func (t *StopTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *StopTool) Execute(_ context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in stopInput
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

	if task.Status == StatusKilled {
		return &models.ToolResult{Content: fmt.Sprintf("Task #%s is already killed.", in.TaskID)}, nil
	}

	task.Status = StatusKilled
	task.UpdatedAt = time.Now()

	if err := store.SaveTask(task); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to stop task: %s", err), IsError: true}, nil
	}

	return &models.ToolResult{Content: fmt.Sprintf("Task #%s stopped (status: killed).", in.TaskID)}, nil
}

func (t *StopTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}
