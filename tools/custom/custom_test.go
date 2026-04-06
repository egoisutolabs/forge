package custom

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

func testDef() *Definition {
	return &Definition{
		Name:        "TestTool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "the name",
				},
			},
			"required": []any{"name"},
		},
		Command:         "echo test",
		Timeout:         5,
		ReadOnly:        false,
		ConcurrencySafe: false,
		SearchHintText:  "testing hint",
	}
}

func TestTool_Name(t *testing.T) {
	tool := New(testDef())
	if tool.Name() != "TestTool" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestTool_Description(t *testing.T) {
	tool := New(testDef())
	if tool.Description() != "A test tool" {
		t.Errorf("Description() = %q", tool.Description())
	}
}

func TestTool_InputSchema(t *testing.T) {
	tool := New(testDef())
	schema := tool.InputSchema()
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema() not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema.type = %v", parsed["type"])
	}
}

func TestTool_SearchHint(t *testing.T) {
	tool := New(testDef())
	if tool.SearchHint() != "testing hint" {
		t.Errorf("SearchHint() = %q", tool.SearchHint())
	}
}

func TestTool_ValidateInput_Valid(t *testing.T) {
	tool := New(testDef())
	err := tool.ValidateInput(json.RawMessage(`{"name": "test"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTool_ValidateInput_MissingRequired(t *testing.T) {
	tool := New(testDef())
	err := tool.ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
}

func TestTool_ValidateInput_InvalidJSON(t *testing.T) {
	tool := New(testDef())
	err := tool.ValidateInput(json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTool_CheckPermissions_ReadOnly(t *testing.T) {
	def := testDef()
	def.ReadOnly = true
	tool := New(def)

	decision, err := tool.CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("Behavior = %q, want %q", decision.Behavior, models.PermAllow)
	}
}

func TestTool_CheckPermissions_NonReadOnly(t *testing.T) {
	def := testDef()
	def.ReadOnly = false
	tool := New(def)

	decision, err := tool.CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("Behavior = %q, want %q", decision.Behavior, models.PermAsk)
	}
}

func TestTool_IsConcurrencySafe(t *testing.T) {
	def := testDef()
	def.ConcurrencySafe = true
	tool := New(def)
	if !tool.IsConcurrencySafe(nil) {
		t.Error("IsConcurrencySafe = false, want true")
	}

	def.ConcurrencySafe = false
	tool2 := New(def)
	if tool2.IsConcurrencySafe(nil) {
		t.Error("IsConcurrencySafe = true, want false")
	}
}

func TestTool_IsReadOnly(t *testing.T) {
	def := testDef()
	def.ReadOnly = true
	tool := New(def)
	if !tool.IsReadOnly(nil) {
		t.Error("IsReadOnly = false, want true")
	}
}

func TestTool_Execute_Integration(t *testing.T) {
	// Use the echo_input script to verify end-to-end execution.
	absScript, _ := filepath.Abs(filepath.Join("testdata", "scripts", "echo_input.sh"))
	def := &Definition{
		Name:        "EchoTool",
		Description: "Echoes input",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Command: absScript,
		Timeout: 5,
	}
	tool := New(def)

	input := json.RawMessage(`{"key": "value"}`)
	tctx := &tools.ToolContext{Cwd: "."}
	result, err := tool.Execute(context.Background(), input, tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, content = %q", result.Content)
	}
	if result.Content == "" {
		t.Error("Content is empty")
	}
}

func TestTool_ImplementsInterface(t *testing.T) {
	// Compile-time check is in custom.go, but verify at runtime too.
	var _ tools.Tool = New(testDef())
	var _ tools.SearchHinter = New(testDef())
}
