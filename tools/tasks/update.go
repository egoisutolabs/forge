package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// UpdateTool implements the TaskUpdate tool — merges partial updates into an
// existing task and persists the result.
type UpdateTool struct {
	store *TaskStore
}

// updateInput mirrors the TaskUpdate input schema.
// Pointer fields are optional: nil means "do not change this field".
type updateInput struct {
	TaskID       string         `json:"taskId"`
	Subject      *string        `json:"subject,omitempty"`
	Description  *string        `json:"description,omitempty"`
	ActiveForm   *string        `json:"activeForm,omitempty"`
	Status       *string        `json:"status,omitempty"`
	Owner        *string        `json:"owner,omitempty"`
	AddBlocks    []string       `json:"addBlocks,omitempty"`
	AddBlockedBy []string       `json:"addBlockedBy,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

func (t *UpdateTool) Name() string                             { return "TaskUpdate" }
func (t *UpdateTool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (t *UpdateTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *UpdateTool) Description() string {
	return "Update an existing task. Supports partial updates — only supplied fields are changed. " +
		"Setting status to \"deleted\" permanently removes the task."
}

func (t *UpdateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"taskId":      {"type": "string", "description": "ID of the task to update"},
			"subject":     {"type": "string", "description": "New task title"},
			"description": {"type": "string", "description": "New task description"},
			"activeForm":  {"type": "string", "description": "New active-form text"},
			"status":      {
				"type": "string",
				"enum": ["pending", "in_progress", "completed", "killed", "deleted"],
				"description": "New status. \"deleted\" removes the task permanently."
			},
			"owner":       {"type": "string", "description": "Agent/user that owns this task"},
			"addBlocks":   {
				"type": "array",
				"items": {"type": "string"},
				"description": "Task IDs that this task now blocks (bidirectional)"
			},
			"addBlockedBy": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Task IDs that now block this task (bidirectional)"
			},
			"metadata": {
				"type": "object",
				"description": "Metadata keys to merge. A null value deletes the key."
			}
		},
		"required": ["taskId"]
	}`)
}

func (t *UpdateTool) ValidateInput(input json.RawMessage) error {
	var in updateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		return fmt.Errorf("taskId is required")
	}
	return nil
}

func (t *UpdateTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *UpdateTool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in updateInput
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

	// status = "deleted" → remove the file.
	if in.Status != nil && *in.Status == StatusDeleted {
		if err := store.DeleteTask(in.TaskID); err != nil {
			return &models.ToolResult{Content: fmt.Sprintf("Failed to delete task: %s", err), IsError: true}, nil
		}
		return &models.ToolResult{Content: fmt.Sprintf("Task #%s deleted.", in.TaskID)}, nil
	}

	var updated []string

	if in.Subject != nil && *in.Subject != task.Subject {
		task.Subject = *in.Subject
		updated = append(updated, "subject")
	}
	if in.Description != nil && *in.Description != task.Description {
		task.Description = *in.Description
		updated = append(updated, "description")
	}
	if in.ActiveForm != nil && *in.ActiveForm != task.ActiveForm {
		task.ActiveForm = *in.ActiveForm
		updated = append(updated, "activeForm")
	}
	var statusTransitionedToCompleted bool
	if in.Status != nil && *in.Status != task.Status {
		if *in.Status == StatusCompleted {
			statusTransitionedToCompleted = true
		}
		task.Status = *in.Status
		updated = append(updated, "status")
	}
	if in.Owner != nil && *in.Owner != task.Owner {
		task.Owner = *in.Owner
		updated = append(updated, "owner")
	}

	// addBlocks: bidirectional — update this task's Blocks AND the target's BlockedBy.
	for _, blockID := range in.AddBlocks {
		if !containsString(task.Blocks, blockID) {
			task.Blocks = appendUnique(task.Blocks, blockID)
			updated = append(updated, "blocks")
			// Update the reverse side.
			if err := addBlockedByToTask(store, blockID, in.TaskID); err != nil {
				// Non-fatal: log but continue.
				_ = err
			}
		}
	}

	// addBlockedBy: bidirectional — update this task's BlockedBy AND the target's Blocks.
	for _, blockerID := range in.AddBlockedBy {
		if !containsString(task.BlockedBy, blockerID) {
			task.BlockedBy = appendUnique(task.BlockedBy, blockerID)
			updated = append(updated, "blockedBy")
			// Update the reverse side.
			if err := addBlocksToTask(store, blockerID, in.TaskID); err != nil {
				_ = err
			}
		}
	}

	// Metadata merge: null values delete keys, other values upsert.
	if len(in.Metadata) > 0 {
		if task.Metadata == nil {
			task.Metadata = make(map[string]any)
		}
		for k, v := range in.Metadata {
			if v == nil {
				delete(task.Metadata, k)
			} else {
				task.Metadata[k] = v
			}
		}
		updated = append(updated, "metadata")
	}

	if len(updated) == 0 {
		return &models.ToolResult{Content: fmt.Sprintf("Task #%s: no changes.", in.TaskID)}, nil
	}

	task.UpdatedAt = time.Now()
	if err := store.SaveTask(task); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to save task: %s", err), IsError: true}, nil
	}

	// Fire TaskCompleted hook on status transition to "completed" (best-effort).
	if statusTransitionedToCompleted && tctx != nil && len(tctx.Hooks) > 0 {
		taskJSON, _ := json.Marshal(task)
		hooks.ExecuteHooks(ctx, tctx.Hooks, hooks.HookEventTaskCompleted, hooks.HookInput{ //nolint:errcheck
			EventName: hooks.HookEventTaskCompleted,
			ToolName:  t.Name(),
			ToolInput: taskJSON,
		}, tctx.TrustedSources)
	}

	return &models.ToolResult{
		Content: fmt.Sprintf("Task #%s updated: %v", in.TaskID, updated),
	}, nil
}

func (t *UpdateTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}

// addBlockedByToTask loads targetID and appends sourceID to its BlockedBy list.
func addBlockedByToTask(store *TaskStore, targetID, sourceID string) error {
	target, err := store.LoadTask(targetID)
	if err != nil || target == nil {
		return err
	}
	target.BlockedBy = appendUnique(target.BlockedBy, sourceID)
	target.UpdatedAt = time.Now()
	return store.SaveTask(target)
}

// addBlocksToTask loads targetID and appends sourceID to its Blocks list.
func addBlocksToTask(store *TaskStore, targetID, sourceID string) error {
	target, err := store.LoadTask(targetID)
	if err != nil || target == nil {
		return err
	}
	target.Blocks = appendUnique(target.Blocks, sourceID)
	target.UpdatedAt = time.Now()
	return store.SaveTask(target)
}
