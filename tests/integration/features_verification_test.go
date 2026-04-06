// Package integration: Custom Tools + Observability verification tests.
//
// These tests verify end-to-end behavior of custom tools loading/execution
// and observability event emission, redaction, and log querying.
package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/observe"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/custom"
)

// ============================================================
// Custom Tools: YAML → Load → Execute roundtrip
// ============================================================

func TestCustomTool_YAMLLoadExecuteRoundtrip(t *testing.T) {
	// Create a temp directory with a custom tool YAML.
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a simple custom tool that echoes input back.
	yaml := `
name: EchoTest
description: Echoes JSON input back to stdout
input_schema:
  type: object
  properties:
    message:
      type: string
      description: Message to echo
  required: [message]
command: "cat"
timeout: 5
read_only: true
concurrency_safe: true
search_hint: "echo test roundtrip"
`
	if err := os.WriteFile(filepath.Join(toolsDir, "echo_test.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Discover tools from the temp directory.
	builtinNames := map[string]bool{"Bash": true, "Read": true}
	customTools, errs := custom.DiscoverTools(dir, builtinNames, toolsDir)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(customTools) != 1 {
		t.Fatalf("expected 1 custom tool, got %d", len(customTools))
	}

	tool := customTools[0]

	// Verify tool implements the interface.
	var _ tools.Tool = tool

	// Verify metadata.
	if tool.Name() != "EchoTest" {
		t.Errorf("Name() = %q, want EchoTest", tool.Name())
	}
	if tool.Description() != "Echoes JSON input back to stdout" {
		t.Errorf("Description() = %q", tool.Description())
	}
	if !tool.IsReadOnly(nil) {
		t.Error("IsReadOnly = false, want true")
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Error("IsConcurrencySafe = false, want true")
	}

	// Verify SearchHinter interface.
	if sh, ok := interface{}(tool).(tools.SearchHinter); ok {
		if sh.SearchHint() != "echo test roundtrip" {
			t.Errorf("SearchHint() = %q", sh.SearchHint())
		}
	} else {
		t.Error("tool does not implement SearchHinter")
	}

	// Verify InputSchema is valid JSON with type=object.
	schema := tool.InputSchema()
	var schemaMap map[string]any
	if err := json.Unmarshal(schema, &schemaMap); err != nil {
		t.Fatalf("InputSchema not valid JSON: %v", err)
	}
	if schemaMap["type"] != "object" {
		t.Errorf("schema.type = %v", schemaMap["type"])
	}

	// Validate input — should pass with required field.
	input := json.RawMessage(`{"message": "hello world"}`)
	if err := tool.ValidateInput(input); err != nil {
		t.Errorf("ValidateInput failed: %v", err)
	}

	// Validate input — should fail without required field.
	if err := tool.ValidateInput(json.RawMessage(`{}`)); err == nil {
		t.Error("ValidateInput should fail for missing required field")
	}

	// Execute the tool.
	tctx := &tools.ToolContext{Cwd: dir}
	result, err := tool.Execute(context.Background(), input, tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("IsError = true, content = %q", result.Content)
	}

	// The 'cat' command should echo back the input JSON.
	if !strings.Contains(result.Content, "hello world") {
		t.Errorf("expected output to contain input, got: %q", result.Content)
	}
}

// ============================================================
// Custom Tools: built-in name collision rejected
// ============================================================

func TestCustomTool_BuiltinCollisionRejected(t *testing.T) {
	dir := t.TempDir()
	yaml := `
name: Bash
description: Collides with built-in Bash tool
input_schema:
  type: object
  properties: {}
command: "echo hi"
`
	if err := os.WriteFile(filepath.Join(dir, "bash.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	builtins := map[string]bool{"Bash": true}
	tools, errs := custom.DiscoverTools(".", builtins, dir)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools (collision), got %d", len(tools))
	}
	if len(errs) != 1 {
		t.Errorf("expected 1 collision error, got %d", len(errs))
	}
}

// ============================================================
// Custom Tools: project-local overrides user-global
// ============================================================

func TestCustomTool_ProjectOverridesGlobal(t *testing.T) {
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

	customTools, _ := custom.DiscoverTools(".", nil, globalDir, projectDir)
	if len(customTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(customTools))
	}
	if customTools[0].Description() != "project version" {
		t.Errorf("expected project version to win, got %q", customTools[0].Description())
	}
}

// ============================================================
// Observe: events emitted for tool call
// ============================================================

func TestObserve_ToolCallEventsEmitted(t *testing.T) {
	dir := t.TempDir()

	// Initialize the emitter with a temp log directory.
	resetObserveGlobals(t)
	if err := observe.Init("test-tool-events", observe.EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	observe.SetTurn(1)

	// Emit tool start and end events.
	traceID := observe.EmitToolStart("Grep", "toolu_123", json.RawMessage(`{"pattern":"func main"}`), true)
	observe.EmitToolEnd(traceID, "Grep", "toolu_123", 85*time.Millisecond, "main.go:41:func main()", false)

	observe.Shutdown()

	// Read and verify events.
	events := readLogEvents(t, filepath.Join(dir, "session-test-tool-events.jsonl"))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify tool_call_start.
	if events[0].EventType != observe.EventToolCallStart {
		t.Errorf("event[0].type = %q, want tool_call_start", events[0].EventType)
	}
	var startPayload observe.ToolCallStartPayload
	json.Unmarshal(events[0].Payload, &startPayload)
	if startPayload.ToolName != "Grep" {
		t.Errorf("tool_name = %q", startPayload.ToolName)
	}
	if startPayload.ToolUseID != "toolu_123" {
		t.Errorf("tool_use_id = %q", startPayload.ToolUseID)
	}
	if !startPayload.IsConcSafe {
		t.Error("is_conc_safe = false, want true")
	}

	// Verify tool_call_end.
	if events[1].EventType != observe.EventToolCallEnd {
		t.Errorf("event[1].type = %q, want tool_call_end", events[1].EventType)
	}
	var endPayload observe.ToolCallEndPayload
	json.Unmarshal(events[1].Payload, &endPayload)
	if endPayload.DurationMs != 85 {
		t.Errorf("duration_ms = %d, want 85", endPayload.DurationMs)
	}
	if endPayload.IsError {
		t.Error("is_error = true, want false")
	}

	// Verify trace IDs match.
	if events[0].TraceID != events[1].TraceID {
		t.Errorf("trace IDs don't match: %q vs %q", events[0].TraceID, events[1].TraceID)
	}
}

// ============================================================
// Observe: redaction strips inputs/outputs
// ============================================================

func TestObserve_RedactionStripsInputsOutputs(t *testing.T) {
	dir := t.TempDir()

	resetObserveGlobals(t)
	if err := observe.Init("test-redact", observe.EmitterOpts{LogDir: dir, Redact: true}); err != nil {
		t.Fatal(err)
	}

	// Emit events with sensitive data.
	observe.EmitToolStart("Bash", "tu1", json.RawMessage(`{"command":"cat /etc/passwd"}`), false)
	observe.EmitToolEnd("t1", "Bash", "tu1", time.Second, "root:x:0:0:root", false)
	observe.EmitAgentSpawn("a1", "search secrets", "general", "opus", false, "find all API keys in the codebase")

	observe.Shutdown()

	events := readLogEvents(t, filepath.Join(dir, "session-test-redact.jsonl"))
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// tool_call_start input should be redacted.
	var startP observe.ToolCallStartPayload
	json.Unmarshal(events[0].Payload, &startP)
	if string(startP.Input) != `"[REDACTED]"` {
		t.Errorf("input not redacted: %s", startP.Input)
	}

	// tool_call_end output should be redacted.
	var endP observe.ToolCallEndPayload
	json.Unmarshal(events[1].Payload, &endP)
	if endP.Output != "[REDACTED]" {
		t.Errorf("output not redacted: %q", endP.Output)
	}

	// agent_spawn prompt should be redacted.
	var agentP observe.AgentSpawnPayload
	json.Unmarshal(events[2].Payload, &agentP)
	if agentP.Prompt != "[REDACTED]" {
		t.Errorf("prompt not redacted: %q", agentP.Prompt)
	}

	// Non-redacted fields should be preserved.
	if startP.ToolName != "Bash" {
		t.Errorf("tool name was redacted: %q", startP.ToolName)
	}
	if agentP.Description != "search secrets" {
		t.Errorf("description was redacted: %q", agentP.Description)
	}
}

// ============================================================
// Observe: log query returns correct stats
// ============================================================

func TestObserve_LogQueryReturnsCorrectStats(t *testing.T) {
	dir := t.TempDir()

	resetObserveGlobals(t)
	if err := observe.Init("test-stats-session", observe.EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}

	// Emit a representative session.
	observe.SetTurn(1)
	observe.EmitAPICall(observe.APICallPayload{
		Model: "claude-sonnet-4-6", InputTokens: 1000, OutputTokens: 200,
		CostUSD: 0.05, DurationMs: 500, StopReason: "tool_use",
	})
	observe.EmitToolStart("Grep", "tu1", json.RawMessage(`{"pattern":"main"}`), true)
	observe.EmitToolEnd("t1", "Grep", "tu1", 50*time.Millisecond, "found 5 matches", false)
	observe.EmitToolStart("Bash", "tu2", json.RawMessage(`{"command":"ls"}`), false)
	observe.EmitToolEnd("t2", "Bash", "tu2", 300*time.Millisecond, "file1\nfile2", false)

	observe.SetTurn(2)
	observe.EmitAPICall(observe.APICallPayload{
		Model: "claude-sonnet-4-6", InputTokens: 1500, OutputTokens: 300,
		CostUSD: 0.08, DurationMs: 800, StopReason: "end_turn",
	})
	observe.EmitAgentSpawn("ag1", "search codebase", "Explore", "sonnet", false, "find files")
	observe.EmitAgentComplete("ag1", 2*time.Second, 3, false, "completed")
	observe.EmitSkillInvoke("commit", "-m fix", "bundled", []string{"Bash"}, 500)
	observe.EmitError("tool", "Bash", "permission denied")

	observe.Shutdown()

	// Read events and verify counts.
	events := readLogEvents(t, filepath.Join(dir, "session-test-stats-session.jsonl"))

	var toolCallEnds, apiCalls, agentSpawns, agentCompletes, skillInvokes, errors int
	for _, e := range events {
		switch e.EventType {
		case observe.EventToolCallEnd:
			toolCallEnds++
		case observe.EventAPICall:
			apiCalls++
		case observe.EventAgentSpawn:
			agentSpawns++
		case observe.EventAgentComplete:
			agentCompletes++
		case observe.EventSkillInvoke:
			skillInvokes++
		case observe.EventError:
			errors++
		}
	}

	if toolCallEnds != 2 {
		t.Errorf("tool_call_end count = %d, want 2", toolCallEnds)
	}
	if apiCalls != 2 {
		t.Errorf("api_call count = %d, want 2", apiCalls)
	}
	if agentSpawns != 1 {
		t.Errorf("agent_spawn count = %d, want 1", agentSpawns)
	}
	if agentCompletes != 1 {
		t.Errorf("agent_complete count = %d, want 1", agentCompletes)
	}
	if skillInvokes != 1 {
		t.Errorf("skill_invoke count = %d, want 1", skillInvokes)
	}
	if errors != 1 {
		t.Errorf("error count = %d, want 1", errors)
	}

	// Verify API call token sums.
	var totalIn, totalOut int
	var totalCost float64
	for _, e := range events {
		if e.EventType == observe.EventAPICall {
			var p observe.APICallPayload
			json.Unmarshal(e.Payload, &p)
			totalIn += p.InputTokens
			totalOut += p.OutputTokens
			totalCost += p.CostUSD
		}
	}
	if totalIn != 2500 {
		t.Errorf("total input tokens = %d, want 2500", totalIn)
	}
	if totalOut != 500 {
		t.Errorf("total output tokens = %d, want 500", totalOut)
	}
	if totalCost < 0.12 || totalCost > 0.14 {
		t.Errorf("total cost = %f, want ~0.13", totalCost)
	}
}

// ============================================================
// Observe: writer is non-blocking
// ============================================================

func TestObserve_WriterNonBlocking(t *testing.T) {
	dir := t.TempDir()

	resetObserveGlobals(t)
	if err := observe.Init("test-nonblock", observe.EmitterOpts{LogDir: dir}); err != nil {
		t.Fatal(err)
	}

	// Emit many events rapidly — must not block.
	start := time.Now()
	for i := 0; i < 10000; i++ {
		observe.EmitToolStart("Bash", "tu", json.RawMessage(`{}`), false)
	}
	elapsed := time.Since(start)

	observe.Shutdown()

	// 10000 non-blocking emits should complete quickly.
	if elapsed > 5*time.Second {
		t.Errorf("emitting 10000 events took %v, expected < 5s (non-blocking)", elapsed)
	}
}

// ============================================================
// Observe: emitter is no-op when uninitialized
// ============================================================

func TestObserve_EmitterNoOpWhenUninitialized(t *testing.T) {
	resetObserveGlobals(t)

	// None of these should panic.
	observe.Emit(observe.EventError, observe.ErrorPayload{Source: "test", Message: "noop"})
	observe.EmitToolStart("Bash", "id1", json.RawMessage(`{}`), false)
	observe.EmitToolEnd("trace1", "Bash", "id1", time.Second, "out", false)
	observe.EmitAgentSpawn("a1", "desc", "general", "opus", false, "prompt")
	observe.EmitAgentComplete("a1", time.Second, 3, false, "done")
	observe.EmitSkillInvoke("commit", "-m fix", "bundled", nil, 100)
	observe.EmitAPICall(observe.APICallPayload{Model: "opus"})
	observe.EmitError("test", "", "msg")

	if observe.Enabled() {
		t.Error("Enabled() = true when uninitialized")
	}
}

// ============================================================
// Observe: rotation deletes old files
// ============================================================

func TestObserve_RotationDeletesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create an old file.
	oldPath := filepath.Join(dir, "session-old.jsonl")
	os.WriteFile(oldPath, []byte("old\n"), 0600)
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(oldPath, oldTime, oldTime)

	// Create a new file.
	newPath := filepath.Join(dir, "session-new.jsonl")
	os.WriteFile(newPath, []byte("new\n"), 0600)

	observe.RotateLogs(dir, 7*24*time.Hour, 500*1024*1024)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old file should have been deleted")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("new file should still exist")
	}
}

// ============================================================
// Custom Tool: structured JSON output parsing
// ============================================================

func TestCustomTool_StructuredJSONOutput(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)

	yaml := `
name: StructuredTool
description: Returns structured JSON
input_schema:
  type: object
  properties: {}
command: "echo '{\"content\": \"result ok\", \"is_error\": false}'"
timeout: 5
read_only: true
`
	os.WriteFile(filepath.Join(toolsDir, "structured.yaml"), []byte(yaml), 0644)

	customTools, _ := custom.DiscoverTools(dir, nil, toolsDir)
	if len(customTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(customTools))
	}

	tctx := &tools.ToolContext{Cwd: dir}
	result, err := customTools[0].Execute(context.Background(), json.RawMessage(`{}`), tctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("IsError = true, content = %q", result.Content)
	}
	if result.Content != "result ok" {
		t.Errorf("Content = %q, want %q", result.Content, "result ok")
	}
}

// ============================================================
// Custom Tool: timeout kills process
// ============================================================

func TestCustomTool_TimeoutKillsProcess(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, ".forge", "tools")
	os.MkdirAll(toolsDir, 0o755)

	yaml := `
name: SlowTool
description: Sleeps forever
input_schema:
  type: object
  properties: {}
command: "sleep 60"
timeout: 1
read_only: true
`
	os.WriteFile(filepath.Join(toolsDir, "slow.yaml"), []byte(yaml), 0644)

	customTools, _ := custom.DiscoverTools(dir, nil, toolsDir)
	if len(customTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(customTools))
	}

	start := time.Now()
	tctx := &tools.ToolContext{Cwd: dir}
	result, err := customTools[0].Execute(context.Background(), json.RawMessage(`{}`), tctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected IsError = true for timeout")
	}
	if !strings.Contains(result.Content, "timed out") {
		t.Errorf("expected timeout message, got: %q", result.Content)
	}
	if elapsed > 5*time.Second {
		t.Errorf("expected timeout in ~1s, took %v", elapsed)
	}
}

// ============================================================
// Custom Tool + Observe: events emitted for custom tool execution
// ============================================================

func TestCustomToolAndObserve_EventsEmittedForCustomToolExecution(t *testing.T) {
	logDir := t.TempDir()

	resetObserveGlobals(t)
	if err := observe.Init("test-custom-observe", observe.EmitterOpts{LogDir: logDir}); err != nil {
		t.Fatal(err)
	}
	observe.SetTurn(1)

	// Simulate what executeTool/executeSingle does for a custom tool.
	traceID := observe.EmitToolStart("EchoTest", "toolu_custom1", json.RawMessage(`{"message":"hello"}`), true)
	// Simulate execution delay.
	time.Sleep(10 * time.Millisecond)
	observe.EmitToolEnd(traceID, "EchoTest", "toolu_custom1", 10*time.Millisecond, `{"message":"hello"}`, false)

	observe.Shutdown()

	events := readLogEvents(t, filepath.Join(logDir, "session-test-custom-observe.jsonl"))
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify the custom tool name appears in events.
	var startP observe.ToolCallStartPayload
	json.Unmarshal(events[0].Payload, &startP)
	if startP.ToolName != "EchoTest" {
		t.Errorf("expected custom tool name EchoTest, got %q", startP.ToolName)
	}

	var endP observe.ToolCallEndPayload
	json.Unmarshal(events[1].Payload, &endP)
	if endP.ToolName != "EchoTest" {
		t.Errorf("expected custom tool name EchoTest, got %q", endP.ToolName)
	}
	if endP.IsError {
		t.Error("expected no error for successful custom tool")
	}
}

// ============================================================
// Custom Tool: permission model (read-only vs non-read-only)
// ============================================================

func TestCustomTool_PermissionModel(t *testing.T) {
	dir := t.TempDir()

	// Read-only tool.
	roYAML := `
name: ReadOnlyTool
description: A read-only tool
input_schema:
  type: object
  properties: {}
command: echo ok
read_only: true
`
	// Non-read-only tool.
	rwYAML := `
name: ReadWriteTool
description: A read-write tool
input_schema:
  type: object
  properties: {}
command: echo ok
read_only: false
`
	os.WriteFile(filepath.Join(dir, "ro.yaml"), []byte(roYAML), 0644)
	os.WriteFile(filepath.Join(dir, "rw.yaml"), []byte(rwYAML), 0644)

	customTools, _ := custom.DiscoverTools(".", nil, dir)
	if len(customTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(customTools))
	}

	for _, ct := range customTools {
		decision, err := ct.CheckPermissions(json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("CheckPermissions error for %s: %v", ct.Name(), err)
		}

		if ct.Name() == "ReadOnlyTool" {
			if decision.Behavior != models.PermAllow {
				t.Errorf("ReadOnlyTool: expected PermAllow, got %v", decision.Behavior)
			}
		}
		if ct.Name() == "ReadWriteTool" {
			if decision.Behavior != models.PermAsk {
				t.Errorf("ReadWriteTool: expected PermAsk, got %v", decision.Behavior)
			}
		}
	}
}

// ============================================================
// Helpers
// ============================================================

// resetObserveGlobals shuts down any active emitter.
// Uses Shutdown() which is the public API to reset the global state.
func resetObserveGlobals(t *testing.T) {
	t.Helper()
	observe.Shutdown()
}

// readLogEvents parses a JSONL file into events.
func readLogEvents(t *testing.T, path string) []observe.Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer f.Close()

	var events []observe.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e observe.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("invalid JSON line: %v\nline: %s", err, scanner.Text())
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}
