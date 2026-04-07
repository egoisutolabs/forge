package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/egoisutolabs/forge/internal/hooks"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// CreateTool implements the TaskCreate tool — allocates a new task ID, writes
// the task to disk, and returns its ID and subject.
type CreateTool struct {
	store *TaskStore // nil → use NewTaskStore(defaultListID)
}

type createInput struct {
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (t *CreateTool) Name() string                             { return "TaskCreate" }
func (t *CreateTool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (t *CreateTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *CreateTool) Description() string {
	return "Create a new task and add it to the task list. Returns the task ID and subject."
}

func (t *CreateTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subject": {
				"type": "string",
				"description": "Brief title for the task (imperative form, e.g. \"Fix the login bug\")"
			},
			"description": {
				"type": "string",
				"description": "Full description of what needs to be done"
			},
			"activeForm": {
				"type": "string",
				"description": "Present continuous form shown while the task is in_progress (e.g. \"Fixing the login bug\")"
			},
			"metadata": {
				"type": "object",
				"description": "Arbitrary key/value metadata to attach to the task"
			}
		},
		"required": ["subject", "description"]
	}`)
}

func (t *CreateTool) ValidateInput(input json.RawMessage) error {
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if in.Description == "" {
		return fmt.Errorf("description is required")
	}
	return nil
}

func (t *CreateTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *CreateTool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in createInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	store, err := t.getStore()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Storage error: %s", err), IsError: true}, nil
	}

	id, err := store.NextID()
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to allocate task ID: %s", err), IsError: true}, nil
	}

	now := time.Now()
	task := &Task{
		ID:          id,
		Subject:     in.Subject,
		Description: in.Description,
		Status:      StatusPending,
		ActiveForm:  in.ActiveForm,
		Metadata:    in.Metadata,
		Blocks:      []string{},
		BlockedBy:   []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := store.SaveTask(task); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Failed to save task: %s", err), IsError: true}, nil
	}

	// Fire TaskCreated hook (best-effort; errors are non-fatal).
	if tctx != nil && len(tctx.Hooks) > 0 {
		taskJSON, _ := json.Marshal(task)
		hooks.ExecuteHooks(ctx, tctx.Hooks, hooks.HookEventTaskCreated, hooks.HookInput{ //nolint:errcheck
			EventName: hooks.HookEventTaskCreated,
			ToolName:  t.Name(),
			ToolInput: taskJSON,
		}, tctx.TrustedSources)
	}

	result, _ := json.Marshal(map[string]any{
		"task": map[string]string{
			"id":      id,
			"subject": in.Subject,
		},
	})
	return &models.ToolResult{Content: fmt.Sprintf("Task #%s created successfully: %s\n%s", id, in.Subject, result)}, nil
}

func (t *CreateTool) getStore() (*TaskStore, error) {
	if t.store != nil {
		return t.store, nil
	}
	return NewTaskStore(defaultListID)
}
