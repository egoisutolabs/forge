package custom

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubBuiltinTools returns a set of fake built-in tools for testing.
func stubBuiltinTools() []BuiltinInfo {
	return []BuiltinInfo{
		{Name: "Bash", Description: "Execute a bash command"},
		{Name: "FileRead", Description: "Reads a file from the local filesystem."},
		{Name: "Glob", Description: "Find files matching a glob pattern"},
	}
}

func TestListCommand(t *testing.T) {
	// Set up a temp dir with a custom tool.
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)
	os.WriteFile(filepath.Join(toolsDir, "db-query.yaml"), []byte(`
name: db-query
description: Run a database query
input_schema:
  type: object
  properties:
    query:
      type: string
  required: [query]
command: "echo test"
timeout: 10
read_only: true
`), 0o644)

	var buf bytes.Buffer
	err := runList(&buf, stubBuiltinTools(), dir, toolsDir)
	if err != nil {
		t.Fatalf("runList error: %v", err)
	}

	out := buf.String()

	// Should contain built-in tools header and tools.
	if !strings.Contains(out, "Built-in tools:") {
		t.Error("missing 'Built-in tools:' header")
	}
	if !strings.Contains(out, "Bash") {
		t.Error("missing built-in tool Bash")
	}
	if !strings.Contains(out, "FileRead") {
		t.Error("missing built-in tool FileRead")
	}

	// Should contain custom tools header and tool.
	if !strings.Contains(out, "Custom tools:") {
		t.Error("missing 'Custom tools:' header")
	}
	if !strings.Contains(out, "db-query") {
		t.Error("missing custom tool db-query")
	}
}

func TestListCommand_NoCustomTools(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	err := runList(&buf, stubBuiltinTools(), dir)
	if err != nil {
		t.Fatalf("runList error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Built-in tools:") {
		t.Error("missing 'Built-in tools:' header")
	}
	// No custom tools section when none found.
	if strings.Contains(out, "Custom tools:") {
		t.Error("should not show 'Custom tools:' when none exist")
	}
}

func TestShowCommand(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)
	os.WriteFile(filepath.Join(toolsDir, "db-query.yaml"), []byte(`
name: db-query
description: Run a database query
input_schema:
  type: object
  properties:
    query:
      type: string
      description: SQL query to execute
    database:
      type: string
      description: Database name
  required: [query]
command: "psql -c \"$QUERY\""
timeout: 15
read_only: true
concurrency_safe: true
search_hint: "sql database"
`), 0o644)

	var buf bytes.Buffer
	err := runShow(&buf, "db-query", stubBuiltinTools(), dir, toolsDir)
	if err != nil {
		t.Fatalf("runShow error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"Name:",
		"db-query",
		"Description:",
		"Run a database query",
		"Read-only:",
		"yes",
		"Concurrent:",
		"yes",
		"Input Schema:",
		"query",
		"database",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in show output:\n%s", want, out)
		}
	}
}

func TestShowCommand_BuiltinTool(t *testing.T) {
	var buf bytes.Buffer
	err := runShow(&buf, "Bash", stubBuiltinTools(), t.TempDir())
	if err != nil {
		t.Fatalf("runShow error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Bash") {
		t.Error("missing tool name")
	}
	if !strings.Contains(out, "built-in") {
		t.Error("missing 'built-in' source")
	}
}

func TestShowCommand_NotFound(t *testing.T) {
	var buf bytes.Buffer
	err := runShow(&buf, "nonexistent", stubBuiltinTools(), t.TempDir())
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestValidateCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.yaml")
	os.WriteFile(path, []byte(`
name: my-tool
description: A valid tool
input_schema:
  type: object
  properties:
    arg1:
      type: string
  required: [arg1]
command: "echo hello"
timeout: 5
`), 0o644)

	var buf bytes.Buffer
	err := runValidate(&buf, path)
	if err != nil {
		t.Fatalf("runValidate error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "valid") {
		t.Errorf("expected 'valid' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "my-tool") {
		t.Errorf("expected tool name in output, got:\n%s", out)
	}
}

func TestValidateCommand_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte(`
description: Missing name field
command: "echo hello"
`), 0o644)

	var buf bytes.Buffer
	err := runValidate(&buf, path)
	if err == nil {
		t.Fatal("expected error for invalid tool definition")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error about 'name', got: %v", err)
	}
}

func TestValidateCommand_MissingFile(t *testing.T) {
	var buf bytes.Buffer
	err := runValidate(&buf, "/nonexistent/tool.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestTestCommand(t *testing.T) {
	// Create a tool that echoes back input.
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)
	os.WriteFile(filepath.Join(toolsDir, "echo-tool.yaml"), []byte(`
name: echo-tool
description: Echoes input back
input_schema:
  type: object
  properties:
    message:
      type: string
command: "cat"
timeout: 5
read_only: true
`), 0o644)

	var buf bytes.Buffer
	err := runTest(&buf, "echo-tool", `{"message": "hello"}`, dir, toolsDir)
	if err != nil {
		t.Fatalf("runTest error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected input echoed back, got:\n%s", out)
	}
	if !strings.Contains(out, "Exit code: 0") {
		t.Errorf("expected 'Exit code: 0' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Duration:") {
		t.Errorf("expected 'Duration:' in output, got:\n%s", out)
	}
}

func TestTestCommand_NotFound(t *testing.T) {
	var buf bytes.Buffer
	err := runTest(&buf, "nonexistent", `{}`, t.TempDir())
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestTestCommand_InvalidInput(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)
	os.WriteFile(filepath.Join(toolsDir, "needs-arg.yaml"), []byte(`
name: needs-arg
description: Requires an argument
input_schema:
  type: object
  properties:
    arg:
      type: string
  required: [arg]
command: "echo test"
timeout: 5
`), 0o644)

	var buf bytes.Buffer
	err := runTest(&buf, "needs-arg", `{}`, dir, toolsDir)
	if err == nil {
		t.Fatal("expected error for missing required input")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' in error, got: %v", err)
	}
}

func TestDispatchCLI(t *testing.T) {
	tests := []struct {
		name string
		args []string
		ok   bool
	}{
		{"list", []string{"tool", "list"}, true},
		{"show needs arg", []string{"tool", "show"}, false},
		{"validate needs arg", []string{"tool", "validate"}, false},
		{"test needs arg", []string{"tool", "test"}, false},
		{"unknown subcommand", []string{"tool", "unknown"}, false},
	}

	builtins := stubBuiltinTools()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := DispatchCLI(&buf, tt.args, builtins, t.TempDir())
			if tt.ok && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if !tt.ok && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
