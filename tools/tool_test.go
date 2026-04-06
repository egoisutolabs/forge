package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
)

// echoTool is a minimal tool for testing.
type echoTool struct{}

func (e *echoTool) Name() string        { return "Echo" }
func (e *echoTool) Description() string { return "Returns its input as output" }
func (e *echoTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)
}
func (e *echoTool) Execute(_ context.Context, input json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	var params struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &models.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	return &models.ToolResult{Content: params.Text, IsError: false}, nil
}
func (e *echoTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (e *echoTool) ValidateInput(input json.RawMessage) error {
	var params struct {
		Text string `json:"text"`
	}
	return json.Unmarshal(input, &params)
}
func (e *echoTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (e *echoTool) IsReadOnly(_ json.RawMessage) bool        { return true }

func TestFindTool(t *testing.T) {
	tools := []Tool{&echoTool{}}

	if found := FindTool(tools, "Echo"); found == nil {
		t.Fatal("expected to find Echo tool")
	}
	if found := FindTool(tools, "Nonexistent"); found != nil {
		t.Error("expected nil for nonexistent tool")
	}
}

func TestToAPISchema(t *testing.T) {
	schema := ToAPISchema(&echoTool{})
	if schema["name"] != "Echo" {
		t.Errorf("expected name %q, got %v", "Echo", schema["name"])
	}
	if schema["description"] != "Returns its input as output" {
		t.Errorf("unexpected description: %v", schema["description"])
	}
}

func TestEchoTool_Execute(t *testing.T) {
	result, err := (&echoTool{}).Execute(context.Background(), json.RawMessage(`{"text":"hello"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError || result.Content != "hello" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestEchoTool_Execute_InvalidInput(t *testing.T) {
	result, err := (&echoTool{}).Execute(context.Background(), json.RawMessage(`{bad}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid input")
	}
}
