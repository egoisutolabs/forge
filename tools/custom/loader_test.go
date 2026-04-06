package custom

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDefinition_ValidReadOnly(t *testing.T) {
	def, err := ParseDefinition(filepath.Join("testdata", "valid_readonly.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "DatabaseQuery" {
		t.Errorf("Name = %q, want %q", def.Name, "DatabaseQuery")
	}
	if def.Description != "Run a read-only SQL query against the development database" {
		t.Errorf("Description = %q", def.Description)
	}
	if !def.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
	if !def.ConcurrencySafe {
		t.Error("ConcurrencySafe = false, want true")
	}
	if def.Timeout != 15 {
		t.Errorf("Timeout = %d, want 15", def.Timeout)
	}
	if def.SearchHintText != "sql database query postgres" {
		t.Errorf("SearchHint = %q", def.SearchHintText)
	}
	if def.Command != "echo query" {
		t.Errorf("Command = %q", def.Command)
	}

	// Verify input_schema round-trips.
	typ, _ := def.InputSchema["type"].(string)
	if typ != "object" {
		t.Errorf("InputSchema.type = %q, want object", typ)
	}
	req, _ := def.InputSchema["required"].([]any)
	if len(req) != 1 || req[0] != "query" {
		t.Errorf("InputSchema.required = %v, want [query]", req)
	}
}

func TestParseDefinition_ValidReadWrite(t *testing.T) {
	def, err := ParseDefinition(filepath.Join("testdata", "valid_readwrite.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "JiraCreate" {
		t.Errorf("Name = %q", def.Name)
	}
	if def.ReadOnly {
		t.Error("ReadOnly = true, want false")
	}
	if def.ConcurrencySafe {
		t.Error("ConcurrencySafe = true, want false")
	}
	if def.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", def.Timeout)
	}
}

func TestParseDefinition_DefaultTimeout(t *testing.T) {
	// Create a temp YAML with no timeout set.
	dir := t.TempDir()
	data := `
name: TestTool
description: A test tool
input_schema:
  type: object
  properties: {}
command: echo hi
`
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	def, err := ParseDefinition(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Timeout != 10 {
		t.Errorf("default Timeout = %d, want 10", def.Timeout)
	}
}

func TestParseDefinition_InvalidMissingName(t *testing.T) {
	_, err := ParseDefinition(filepath.Join("testdata", "invalid_missing_name.yaml"))
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

func TestParseDefinition_InvalidMissingCommand(t *testing.T) {
	_, err := ParseDefinition(filepath.Join("testdata", "invalid_missing_command.yaml"))
	if err == nil {
		t.Fatal("expected error for missing command, got nil")
	}
}

func TestParseDefinition_InvalidMissingDescription(t *testing.T) {
	_, err := ParseDefinition(filepath.Join("testdata", "invalid_missing_description.yaml"))
	if err == nil {
		t.Fatal("expected error for missing description, got nil")
	}
}

func TestLoadToolsDir_ValidDir(t *testing.T) {
	defs, errs := LoadToolsDir(filepath.Join("testdata"))
	// Should load the 3 valid files and collect errors for 3 invalid files.
	if len(defs) != 3 {
		t.Errorf("got %d valid definitions, want 3", len(defs))
	}
	if len(errs) != 3 {
		t.Errorf("got %d errors, want 3", len(errs))
	}
}

func TestLoadToolsDir_MissingDir(t *testing.T) {
	defs, errs := LoadToolsDir("/nonexistent/path")
	if defs != nil {
		t.Errorf("expected nil defs for missing dir, got %d", len(defs))
	}
	if errs != nil {
		t.Errorf("expected nil errs for missing dir, got %d", len(errs))
	}
}

func TestDiscoverTools_OverridePriority(t *testing.T) {
	// Create two directories: "global" and "project".
	// Both define a tool with the same name but different descriptions.
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	globalYAML := `
name: MyTool
description: global version
input_schema:
  type: object
  properties: {}
command: echo global
`
	projectYAML := `
name: MyTool
description: project version
input_schema:
  type: object
  properties: {}
command: echo project
`
	os.WriteFile(filepath.Join(globalDir, "mytool.yaml"), []byte(globalYAML), 0644)
	os.WriteFile(filepath.Join(projectDir, "mytool.yaml"), []byte(projectYAML), 0644)

	tools, errs := DiscoverTools(".", nil, globalDir, projectDir)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	// Project (later dir) should win.
	if tools[0].Description() != "project version" {
		t.Errorf("Description = %q, want %q", tools[0].Description(), "project version")
	}
}

func TestDiscoverTools_BuiltinCollision(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: Bash
description: Collides with built-in
input_schema:
  type: object
  properties: {}
command: echo hi
`
	os.WriteFile(filepath.Join(dir, "bash.yaml"), []byte(yaml), 0644)

	builtins := map[string]bool{"Bash": true}
	tools, errs := DiscoverTools(".", builtins, dir)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools (collision rejected), got %d", len(tools))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 error for collision, got %d", len(errs))
	}
}
