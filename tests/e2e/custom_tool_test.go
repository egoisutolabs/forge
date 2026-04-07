package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/tools"
	"github.com/egoisutolabs/forge/internal/tools/custom"
)

// TestCustomTool_DiscoverFromYAML writes a sample YAML to a temp dir,
// discovers it via DiscoverTools, and verifies it loads correctly.
func TestCustomTool_DiscoverFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: greet
description: Greet the user
command: echo "hello"
read_only: true
concurrency_safe: true
input_schema:
  type: object
  properties:
    name:
      type: string
`
	mustWriteFile(t, filepath.Join(toolsDir, "greet.yaml"), yamlContent)

	builtinNames := map[string]bool{"Read": true, "Bash": true}
	discovered, errs := custom.DiscoverTools(tmpDir, builtinNames, toolsDir)
	for _, e := range errs {
		t.Errorf("unexpected discovery error: %v", e)
	}

	if len(discovered) != 1 {
		t.Fatalf("expected 1 custom tool, got %d", len(discovered))
	}

	tool := discovered[0]
	if tool.Name() != "greet" {
		t.Errorf("Name() = %q, want \"greet\"", tool.Name())
	}
	if tool.Description() != "Greet the user" {
		t.Errorf("Description() = %q, want \"Greet the user\"", tool.Description())
	}
	if !tool.IsReadOnly(nil) {
		t.Error("expected IsReadOnly=true for read_only tool")
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Error("expected IsConcurrencySafe=true for concurrency_safe tool")
	}

	// Verify InputSchema is valid JSON with type: "object".
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("InputSchema() not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("InputSchema() type = %q, want \"object\"", schema["type"])
	}
}

// TestCustomTool_Execute runs a simple echo custom tool and verifies output.
func TestCustomTool_Execute(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: echo-test
description: Echo input back
command: echo "it works"
read_only: true
`
	mustWriteFile(t, filepath.Join(toolsDir, "echo-test.yaml"), yamlContent)

	discovered, errs := custom.DiscoverTools(tmpDir, nil, toolsDir)
	for _, e := range errs {
		t.Errorf("unexpected error: %v", e)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(discovered))
	}

	tctx := &tools.ToolContext{Cwd: tmpDir}
	result, err := discovered[0].Execute(context.Background(), json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "it works") {
		t.Errorf("expected output to contain 'it works', got: %s", result.Content)
	}
}

// TestCustomTool_ReadOnlyPermission verifies a read_only custom tool gets PermAllow.
func TestCustomTool_ReadOnlyPermission(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: readonly-tool
description: A read only tool
command: echo ok
read_only: true
`
	mustWriteFile(t, filepath.Join(toolsDir, "readonly-tool.yaml"), yamlContent)

	discovered, _ := custom.DiscoverTools(tmpDir, nil, toolsDir)
	if len(discovered) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(discovered))
	}

	decision, err := discovered[0].CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("CheckPermissions error: %v", err)
	}
	if decision.Behavior != "allow" {
		t.Errorf("read_only tool should get PermAllow, got %q", decision.Behavior)
	}
}

// TestCustomTool_WriteToolGetsPermAsk verifies a non-read_only custom tool gets PermAsk.
func TestCustomTool_WriteToolGetsPermAsk(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: write-tool
description: A write tool
command: echo write
`
	mustWriteFile(t, filepath.Join(toolsDir, "write-tool.yaml"), yamlContent)

	discovered, _ := custom.DiscoverTools(tmpDir, nil, toolsDir)
	if len(discovered) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(discovered))
	}

	decision, err := discovered[0].CheckPermissions(json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("CheckPermissions error: %v", err)
	}
	if decision.Behavior != "ask" {
		t.Errorf("non-read_only tool should get PermAsk, got %q", decision.Behavior)
	}
}

// TestCustomTool_InvalidYAML verifies that invalid YAML produces a clear error.
func TestCustomTool_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Missing required fields.
	mustWriteFile(t, filepath.Join(toolsDir, "bad.yaml"), `name: ""`)

	_, errs := custom.DiscoverTools(tmpDir, nil, toolsDir)
	if len(errs) == 0 {
		t.Error("expected errors for invalid YAML, got none")
	}
}

// TestCustomTool_NameCollisionWithBuiltin verifies that a custom tool whose
// name matches a built-in tool is rejected with a clear error.
func TestCustomTool_NameCollisionWithBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: Read
description: Conflicts with built-in Read
command: echo conflict
`
	mustWriteFile(t, filepath.Join(toolsDir, "read.yaml"), yamlContent)

	builtinNames := map[string]bool{"Read": true}
	discovered, errs := custom.DiscoverTools(tmpDir, builtinNames, toolsDir)

	if len(discovered) != 0 {
		t.Errorf("conflicting tool should not be loaded, got %d tools", len(discovered))
	}

	foundCollisionErr := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "conflicts with built-in") {
			foundCollisionErr = true
			break
		}
	}
	if !foundCollisionErr {
		t.Error("expected 'conflicts with built-in' error for name collision")
	}
}

// TestCustomTool_MissingCommand verifies that YAML without a command field fails.
func TestCustomTool_MissingCommand(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `name: no-cmd
description: Missing command field
`
	mustWriteFile(t, filepath.Join(toolsDir, "no-cmd.yaml"), yamlContent)

	_, errs := custom.DiscoverTools(tmpDir, nil, toolsDir)
	if len(errs) == 0 {
		t.Error("expected errors for missing command field, got none")
	}
	foundCmdErr := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "command") {
			foundCmdErr = true
			break
		}
	}
	if !foundCmdErr {
		t.Error("expected error to mention 'command' for missing command field")
	}
}

// TestCustomTool_EmptyDir verifies DiscoverTools handles an empty directory.
func TestCustomTool_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	discovered, errs := custom.DiscoverTools(tmpDir, nil, toolsDir)
	if len(discovered) != 0 {
		t.Errorf("expected 0 tools from empty dir, got %d", len(discovered))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors from empty dir, got %d", len(errs))
	}
}

// TestCustomTool_NonexistentDir verifies DiscoverTools handles a missing directory.
func TestCustomTool_NonexistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	noDir := filepath.Join(tmpDir, "does-not-exist")

	discovered, errs := custom.DiscoverTools(tmpDir, nil, noDir)
	if len(discovered) != 0 {
		t.Errorf("expected 0 tools from missing dir, got %d", len(discovered))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors from missing dir, got %d", len(errs))
	}
}
