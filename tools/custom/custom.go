package custom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// Compile-time interface checks.
var (
	_ tools.Tool         = (*Tool)(nil)
	_ tools.SearchHinter = (*Tool)(nil)
)

// Tool implements tools.Tool for a user-defined custom tool.
type Tool struct {
	def *Definition
}

// New creates a Tool from a parsed Definition.
func New(def *Definition) *Tool {
	return &Tool{def: def}
}

func (t *Tool) Name() string        { return t.def.Name }
func (t *Tool) Description() string { return t.def.Description }

func (t *Tool) InputSchema() json.RawMessage {
	schema := t.def.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	data, _ := json.Marshal(schema)
	return data
}

// SearchHint implements tools.SearchHinter.
func (t *Tool) SearchHint() string { return t.def.SearchHintText }

func (t *Tool) ValidateInput(input json.RawMessage) error {
	// Validate that input is a JSON object.
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return fmt.Errorf("input must be a JSON object: %w", err)
	}

	// Validate required fields from schema.
	required, _ := t.def.InputSchema["required"].([]any)
	for _, r := range required {
		name, _ := r.(string)
		if name == "" {
			continue
		}
		if _, ok := obj[name]; !ok {
			return fmt.Errorf("missing required field: %s", name)
		}
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, tctx *tools.ToolContext) (*models.PermissionDecision, error) {
	if t.def.ReadOnly {
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  fmt.Sprintf("Run custom tool: %s", t.def.Name),
	}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return t.def.ConcurrencySafe }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return t.def.ReadOnly }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	cwd := ""
	if tctx != nil {
		cwd = tctx.Cwd
	}

	result := RunCommand(ctx, t.def.Command, input, t.def.Name, cwd, t.def.Timeout)
	content, isError := ParseOutput(result, t.def.Timeout)

	return &models.ToolResult{
		Content: content,
		IsError: isError,
	}, nil
}

// Def returns the underlying definition. Useful for inspection/testing.
func (t *Tool) Def() *Definition { return t.def }
