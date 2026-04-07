package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

const (
	defaultOutputTimeout = 30_000  // 30 s in milliseconds
	maxOutputTimeout     = 600_000 // 10 min in milliseconds
	pollInterval         = 100 * time.Millisecond
)

// terminalStatuses are the statuses that indicate a task has finished.
var terminalStatuses = map[string]bool{
	StatusCompleted: true,
	StatusKilled:    true,
}

// OutputTool implements the TaskOutput tool — waits for a task to reach a
// terminal status and returns its current state.
type OutputTool struct {
	store *TaskStore
}

type outputInput struct {
	TaskID  string `json:"task_id"`
	Block   *bool  `json:"block,omitempty"`
	Timeout *int   `json:"timeout,omitempty"` // milliseconds
}

func (t *OutputTool) Name() string                             { return "TaskOutput" }
func (t *OutputTool) IsReadOnly(_ json.RawMessage) bool        { return true }
func (t *OutputTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *OutputTool) Description() string {
	return "Get the output of a task. When block=true (default) waits until the task completes or the timeout elapses."
}

func (t *OutputTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"task_id": {
				"type": "string",
				"description": "ID of the task to retrieve output for"
			},
			"block": {
				"type": "boolean",
				"description": "Wait for the task to complete before returning (default: true)"
			},
			"timeout": {
				"type": "integer",
				"description": "Maximum time to wait in milliseconds (default: 30000, max: 600000)"
			}
		},
		"required": ["task_id"]
	}`)
}

func (t *OutputTool) ValidateInput(input json.RawMessage) error {
	var in outputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}
	if in.Timeout != nil && (*in.Timeout < 0 || *in.Timeout > maxOutputTimeout) {
		return fmt.Errorf("timeout must be between 0 and %d ms", maxOutputTimeout)
	}
	return nil
}

func (t *OutputTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *OutputTool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in outputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	// Resolve blocking and timeout settings.
	block := true
	if in.Block != nil {
		block = *in.Block
	}
	timeoutMS := defaultOutputTimeout
	if in.Timeout != nil {
		timeoutMS = *in.Timeout
		if timeoutMS > maxOutputTimeout {
			timeoutMS = maxOutputTimeout
		}
	}

	store, err := t.getStore()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Storage error: %s", err), IsError: true}, nil
	}

	// Determine effective abort context.
	abortCtx := ctx
	if tctx != nil && tctx.AbortCtx != nil {
		abortCtx = tctx.AbortCtx
	}

	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	timeoutCtx, cancel := context.WithDeadline(abortCtx, deadline)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		task, err := store.LoadTask(in.TaskID)
		if err != nil {
			return &models.ToolResult{Content: fmt.Sprintf("Failed to load task: %s", err), IsError: true}, nil
		}
		if task == nil {
			return &models.ToolResult{Content: fmt.Sprintf("task not found: %s", in.TaskID), IsError: true}, nil
		}

		if !block || terminalStatuses[task.Status] {
			status := "success"
			if !terminalStatuses[task.Status] {
				status = "not_ready"
			}
			out, _ := json.Marshal(map[string]any{
				"retrieval_status": status,
				"task": map[string]any{
					"task_id": task.ID,
					"status":  task.Status,
					"subject": task.Subject,
				},
			})
			return &models.ToolResult{Content: string(out)}, nil
		}

		select {
		case <-timeoutCtx.Done():
			out, _ := json.Marshal(map[string]any{
				"retrieval_status": "timeout",
				"task": map[string]any{
					"task_id": in.TaskID,
					"status":  "unknown",
				},
			})
			return &models.ToolResult{Content: string(out)}, nil
		case <-ticker.C:
			// Poll again.
		}
	}
}

func (t *OutputTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}
