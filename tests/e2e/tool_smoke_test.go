package e2e

import (
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/coordinator"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/agent"
	"github.com/egoisutolabs/forge/tools/askuser"
	"github.com/egoisutolabs/forge/tools/astgrep"
	"github.com/egoisutolabs/forge/tools/bash"
	"github.com/egoisutolabs/forge/tools/browser"
	"github.com/egoisutolabs/forge/tools/fileedit"
	"github.com/egoisutolabs/forge/tools/fileread"
	"github.com/egoisutolabs/forge/tools/filewrite"
	"github.com/egoisutolabs/forge/tools/glob"
	"github.com/egoisutolabs/forge/tools/grep"
	"github.com/egoisutolabs/forge/tools/planmode"
	"github.com/egoisutolabs/forge/tools/sendmessage"
	"github.com/egoisutolabs/forge/tools/skill"
	"github.com/egoisutolabs/forge/tools/tasks"
	"github.com/egoisutolabs/forge/tools/toolsearch"
	"github.com/egoisutolabs/forge/tools/webfetch"
	"github.com/egoisutolabs/forge/tools/websearch"
)

// allBuiltinTools returns every built-in tool in the same order as cmd/forge/main.go buildTools().
func allBuiltinTools() []tools.Tool {
	return []tools.Tool{
		&bash.Tool{},
		&fileread.Tool{},
		&fileedit.Tool{},
		&filewrite.Tool{},
		&glob.Tool{},
		&grep.Tool{},
		&askuser.Tool{},
		&planmode.EnterTool{},
		&planmode.ExitTool{},
		&tasks.CreateTool{},
		&tasks.GetTool{},
		&tasks.ListTool{},
		&tasks.UpdateTool{},
		&tasks.StopTool{},
		&tasks.OutputTool{},
		&skill.Tool{},
		&toolsearch.Tool{},
		agent.New(nil, nil),
		sendmessage.New(nil),
		// Non-minimal tools.
		&browser.Tool{},
		&astgrep.Tool{},
		&webfetch.Tool{},
		&websearch.Tool{},
	}
}

// TestToolSmoke_NameNonEmpty ensures every tool has a non-empty Name().
func TestToolSmoke_NameNonEmpty(t *testing.T) {
	for _, tool := range allBuiltinTools() {
		if tool.Name() == "" {
			t.Errorf("tool has empty Name(): %T", tool)
		}
	}
}

// TestToolSmoke_DescriptionNonEmpty ensures every tool has a non-empty Description().
func TestToolSmoke_DescriptionNonEmpty(t *testing.T) {
	for _, tool := range allBuiltinTools() {
		name := tool.Name()
		if tool.Description() == "" {
			t.Errorf("tool %q has empty Description()", name)
		}
	}
}

// TestToolSmoke_InputSchemaValidJSON ensures every tool returns valid JSON from InputSchema().
func TestToolSmoke_InputSchemaValidJSON(t *testing.T) {
	for _, tool := range allBuiltinTools() {
		name := tool.Name()
		schema := tool.InputSchema()
		if len(schema) == 0 {
			t.Errorf("tool %q returned empty InputSchema()", name)
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal(schema, &parsed); err != nil {
			t.Errorf("tool %q InputSchema() is not valid JSON: %v", name, err)
			continue
		}
		// Every schema should be type: "object".
		typ, _ := parsed["type"].(string)
		if typ != "object" {
			t.Errorf("tool %q InputSchema() type = %q, want \"object\"", name, typ)
		}
	}
}

// TestToolSmoke_ReadOnlyTools verifies expected read-only tools return IsReadOnly=true.
func TestToolSmoke_ReadOnlyTools(t *testing.T) {
	readOnlyExpected := map[string]bool{
		"Read":            true,
		"Glob":            true,
		"Grep":            true,
		"AstGrep":         true,
		"WebSearch":       true,
		"WebFetch":        true,
		"ToolSearch":      true,
		"TaskGet":         true,
		"TaskList":        true,
		"TaskOutput":      true,
		"AskUserQuestion": true,
		"EnterPlanMode":   true,
	}

	for _, tool := range allBuiltinTools() {
		name := tool.Name()
		isRO := tool.IsReadOnly(nil)
		expected := readOnlyExpected[name]
		if isRO != expected {
			t.Errorf("tool %q: IsReadOnly(nil) = %v, want %v", name, isRO, expected)
		}
	}
}

// TestToolSmoke_ConcurrencySafeTools verifies that expected concurrency-safe tools
// return IsConcurrencySafe=true.
func TestToolSmoke_ConcurrencySafeTools(t *testing.T) {
	concurrencySafeExpected := map[string]bool{
		"Bash":            false, // Bash returns false for nil input
		"Read":            true,
		"Edit":            false,
		"Write":           false,
		"Glob":            true,
		"Grep":            true,
		"AskUserQuestion": true,
		"EnterPlanMode":   true,
		"ExitPlanMode":    true,
		"TaskCreate":      true,
		"TaskGet":         true,
		"TaskList":        true,
		"TaskUpdate":      true,
		"TaskStop":        true,
		"TaskOutput":      true,
		"Skill":           false,
		"ToolSearch":      true,
		"Agent":           true,
		"SendMessage":     true,
		"Browser":         false,
		"AstGrep":         true,
		"WebFetch":        true,
		"WebSearch":       true,
	}

	for _, tool := range allBuiltinTools() {
		name := tool.Name()
		isSafe := tool.IsConcurrencySafe(nil)
		expected, ok := concurrencySafeExpected[name]
		if !ok {
			t.Errorf("tool %q not in concurrency-safe expectations map", name)
			continue
		}
		if isSafe != expected {
			t.Errorf("tool %q: IsConcurrencySafe(nil) = %v, want %v", name, isSafe, expected)
		}
	}
}

// TestToolSmoke_NameUniqueness ensures no two tools share the same Name().
func TestToolSmoke_NameUniqueness(t *testing.T) {
	seen := make(map[string]int)
	for _, tool := range allBuiltinTools() {
		name := tool.Name()
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("tool name %q appears %d times (expected unique)", name, count)
		}
	}
}

// TestToolSmoke_CoordinatorToolFiltering verifies CoordinatorTools keeps only
// Agent, SendMessage, and TaskStop.
func TestToolSmoke_CoordinatorToolFiltering(t *testing.T) {
	all := allBuiltinTools()
	filtered := coordinator.CoordinatorTools(all)

	allowed := map[string]bool{
		"Agent":       true,
		"SendMessage": true,
		"TaskStop":    true,
	}

	if len(filtered) != len(allowed) {
		t.Errorf("CoordinatorTools returned %d tools, want %d", len(filtered), len(allowed))
	}
	for _, tool := range filtered {
		if !allowed[tool.Name()] {
			t.Errorf("unexpected coordinator tool: %q", tool.Name())
		}
	}
}

// TestToolSmoke_FindTool verifies tools.FindTool looks up by name correctly.
func TestToolSmoke_FindTool(t *testing.T) {
	all := allBuiltinTools()

	found := tools.FindTool(all, "Read")
	if found == nil {
		t.Fatal("FindTool(\"Read\") returned nil")
	}
	if found.Name() != "Read" {
		t.Errorf("FindTool returned tool named %q, want \"Read\"", found.Name())
	}

	notFound := tools.FindTool(all, "NonExistentTool")
	if notFound != nil {
		t.Error("FindTool should return nil for unknown tool")
	}
}

// TestToolSmoke_ToAPISchema verifies ToAPISchema produces valid structure.
func TestToolSmoke_ToAPISchema(t *testing.T) {
	tool := &fileread.Tool{}
	schema := tools.ToAPISchema(tool)

	if schema["name"] != "Read" {
		t.Errorf("ToAPISchema name = %q, want \"Read\"", schema["name"])
	}
	if schema["description"] == nil || schema["description"] == "" {
		t.Error("ToAPISchema description should be non-empty")
	}
	if schema["input_schema"] == nil {
		t.Error("ToAPISchema input_schema should be non-nil")
	}
}
