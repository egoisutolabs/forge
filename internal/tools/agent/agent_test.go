package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// stubCaller is a mock API caller that returns a fixed text response.
type stubCaller struct {
	response string
}

func (s *stubCaller) Stream(_ context.Context, _ api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 1)
	msg := &models.Message{
		ID:   "msg-stub",
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockText, Text: s.response},
		},
		StopReason: models.StopEndTurn,
		Usage:      &models.Usage{InputTokens: 10, OutputTokens: 5},
	}
	ch <- api.StreamEvent{Type: "message_done", Message: msg}
	close(ch)
	return ch
}

// ── Tool metadata ──────────────────────────────────────────────────────────────

func TestAgentTool_Name(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	if tool.Name() != "Agent" {
		t.Errorf("Name() = %q, want Agent", tool.Name())
	}
}

func TestAgentTool_Description_NonEmpty(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
}

func TestAgentTool_InputSchema_Valid(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	var schema map[string]any
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("InputSchema() is not valid JSON: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	for _, required := range []string{"description", "prompt"} {
		if props[required] == nil {
			t.Errorf("InputSchema missing property %q", required)
		}
	}
}

func TestAgentTool_IsConcurrencySafe(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	if !tool.IsConcurrencySafe(nil) {
		t.Error("AgentTool should be concurrency safe")
	}
}

// ── ValidateInput ──────────────────────────────────────────────────────────────

func TestAgentTool_ValidateInput_Valid(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	input := json.RawMessage(`{"description":"do task","prompt":"perform the task"}`)
	if err := tool.ValidateInput(input); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAgentTool_ValidateInput_MissingDescription(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	input := json.RawMessage(`{"prompt":"do something"}`)
	if err := tool.ValidateInput(input); err == nil {
		t.Error("expected error for missing description")
	}
}

func TestAgentTool_ValidateInput_MissingPrompt(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	input := json.RawMessage(`{"description":"task"}`)
	if err := tool.ValidateInput(input); err == nil {
		t.Error("expected error for missing prompt")
	}
}

func TestAgentTool_ValidateInput_EmptyDescription(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	input := json.RawMessage(`{"description":"  ","prompt":"do something"}`)
	if err := tool.ValidateInput(input); err == nil {
		t.Error("expected error for whitespace-only description")
	}
}

func TestAgentTool_ValidateInput_EmptyPrompt(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	input := json.RawMessage(`{"description":"task","prompt":""}`)
	if err := tool.ValidateInput(input); err == nil {
		t.Error("expected error for empty prompt")
	}
}

// ── CheckPermissions ──────────────────────────────────────────────────────────

func TestAgentTool_CheckPermissions_Allows(t *testing.T) {
	tool := New(&stubCaller{}, nil)
	decision, err := tool.CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("behavior = %v, want Allow", decision.Behavior)
	}
}

// ── Execute — sync mode ───────────────────────────────────────────────────────

func TestAgentTool_Execute_Sync_ReturnsAgentOutput(t *testing.T) {
	caller := &stubCaller{response: "the answer is 42"}
	tool := New(caller, nil)

	input := json.RawMessage(`{
		"description": "find answer",
		"prompt": "What is 6 times 7?"
	}`)

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError, content: %s", result.Content)
	}
	if result.Content != "the answer is 42" {
		t.Errorf("Content = %q, want %q", result.Content, "the answer is 42")
	}
}

func TestAgentTool_Execute_Sync_WithSubagentType(t *testing.T) {
	caller := &stubCaller{response: "explored the codebase"}
	tool := New(caller, BuiltInAgents())

	input := json.RawMessage(`{
		"description": "explore files",
		"prompt": "Find all Go test files",
		"subagent_type": "Explore"
	}`)

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
	if result.Content != "explored the codebase" {
		t.Errorf("Content = %q", result.Content)
	}
}

func TestAgentTool_Execute_Sync_UnknownSubagentType(t *testing.T) {
	// Unknown subagent_type falls back to general-purpose — should still work
	caller := &stubCaller{response: "done"}
	tool := New(caller, BuiltInAgents())

	input := json.RawMessage(`{
		"description": "custom task",
		"prompt": "do something",
		"subagent_type": "NoSuchAgent"
	}`)

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

func TestAgentTool_Execute_Sync_WithToolContext(t *testing.T) {
	caller := &stubCaller{response: "ok"}
	tool := New(caller, nil)

	tctx := &tools.ToolContext{
		Cwd:   "/tmp",
		Model: "claude-opus-4-6",
		Tools: []tools.Tool{mt("Read"), mt("Bash")},
	}

	input := json.RawMessage(`{"description":"test","prompt":"do task"}`)
	result, err := tool.Execute(context.Background(), input, tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Content)
	}
}

// ── Execute — async mode ──────────────────────────────────────────────────────

func TestAgentTool_Execute_Async_ReturnsLaunchStatus(t *testing.T) {
	caller := &stubCaller{response: "background result"}
	tool := New(caller, nil)
	tool.registry = NewAgentRegistry() // isolated registry

	input := json.RawMessage(`{
		"description": "background task",
		"prompt": "do something in background",
		"run_in_background": true
	}`)

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("async result is not valid JSON: %v", err)
	}
	if out["status"] != "async_launched" {
		t.Errorf("status = %v, want async_launched", out["status"])
	}
	agentID, ok := out["agentId"].(string)
	if !ok || agentID == "" {
		t.Error("agentId should be a non-empty string")
	}
	if out["outputFile"] == "" {
		t.Error("outputFile should be set")
	}
	if out["description"] != "background task" {
		t.Errorf("description = %v", out["description"])
	}
}

func TestAgentTool_Execute_Async_RegistersInRegistry(t *testing.T) {
	caller := &stubCaller{response: "done"}
	tool := New(caller, nil)
	registry := NewAgentRegistry()
	tool.registry = registry

	input := json.RawMessage(`{
		"description": "my task",
		"prompt": "do it",
		"run_in_background": true
	}`)

	result, _ := tool.Execute(context.Background(), input, nil)

	var out map[string]any
	_ = json.Unmarshal([]byte(result.Content), &out)
	agentID := out["agentId"].(string)

	// Agent should be registered immediately
	ba := registry.Get(agentID)
	if ba == nil {
		t.Fatal("background agent should be registered immediately after launch")
	}
	if ba.Description != "my task" {
		t.Errorf("description = %q", ba.Description)
	}
}

// ── resolveModel ──────────────────────────────────────────────────────────────

func TestResolveModel_Aliases(t *testing.T) {
	tests := []struct {
		alias string
		want  string
	}{
		{"sonnet", "claude-sonnet-4-6"},
		{"Sonnet", "claude-sonnet-4-6"}, // case-insensitive
		{"opus", "claude-opus-4-6"},
		{"haiku", "claude-haiku-4-5-20251001"},
	}
	for _, tc := range tests {
		got := resolveModel(tc.alias, "", nil)
		if got != tc.want {
			t.Errorf("resolveModel(%q) = %q, want %q", tc.alias, got, tc.want)
		}
	}
}

func TestResolveModel_EmptyFallsBackToParent(t *testing.T) {
	tctx := &tools.ToolContext{Model: "claude-opus-4-6"}
	got := resolveModel("", "", tctx)
	if got != "claude-opus-4-6" {
		t.Errorf("resolveModel empty = %q, want parent model", got)
	}
}

func TestResolveModel_EmptyNoContext(t *testing.T) {
	got := resolveModel("", "", nil)
	if got != "claude-sonnet-4-6" {
		t.Errorf("resolveModel empty/nil = %q, want claude-sonnet-4-6", got)
	}
}

func TestResolveModel_AgentDefaultUsed(t *testing.T) {
	got := resolveModel("", "haiku", nil)
	if got != "claude-haiku-4-5-20251001" {
		t.Errorf("resolveModel('', 'haiku') = %q", got)
	}
}

func TestResolveModel_AliasOverridesAgentDefault(t *testing.T) {
	got := resolveModel("opus", "haiku", nil)
	if got != "claude-opus-4-6" {
		t.Errorf("alias should override agent default, got %q", got)
	}
}

func TestResolveModel_FullModelIDPassthrough(t *testing.T) {
	got := resolveModel("claude-custom-model-xyz", "", nil)
	if got != "claude-custom-model-xyz" {
		t.Errorf("full model ID should pass through unchanged, got %q", got)
	}
}

// ── lastAssistantText ─────────────────────────────────────────────────────────

func TestLastAssistantText_Found(t *testing.T) {
	messages := []*models.Message{
		models.NewUserMessage("hello"),
		{
			Role:    models.RoleAssistant,
			Content: []models.Block{{Type: models.BlockText, Text: "world"}},
		},
	}
	got := lastAssistantText(messages)
	if got != "world" {
		t.Errorf("lastAssistantText = %q, want world", got)
	}
}

func TestLastAssistantText_Empty(t *testing.T) {
	got := lastAssistantText(nil)
	if got != "" {
		t.Errorf("lastAssistantText(nil) = %q, want empty", got)
	}
}

func TestLastAssistantText_NoAssistantMessages(t *testing.T) {
	messages := []*models.Message{
		models.NewUserMessage("hello"),
	}
	got := lastAssistantText(messages)
	if got != "" {
		t.Errorf("lastAssistantText no assistant = %q, want empty", got)
	}
}

// ── resolveAgentDef ───────────────────────────────────────────────────────────

func TestResolveAgentDef_BuiltIn(t *testing.T) {
	tool := New(&stubCaller{}, BuiltInAgents())
	def := tool.resolveAgentDef("Explore")
	if def.Name != "Explore" {
		t.Errorf("resolveAgentDef(Explore).Name = %q", def.Name)
	}
	if !strings.Contains(def.SystemPrompt, "exploration") {
		t.Errorf("Explore system prompt should mention exploration, got: %q", def.SystemPrompt)
	}
}

func TestResolveAgentDef_Unknown_FallsToGeneralPurpose(t *testing.T) {
	tool := New(&stubCaller{}, BuiltInAgents())
	def := tool.resolveAgentDef("NoSuchType")
	if def.Name != "general-purpose" {
		t.Errorf("unknown type should fall back to general-purpose, got %q", def.Name)
	}
}

func TestResolveAgentDef_Empty_FallsToGeneralPurpose(t *testing.T) {
	tool := New(&stubCaller{}, BuiltInAgents())
	def := tool.resolveAgentDef("")
	if def.Name != "general-purpose" {
		t.Errorf("empty type should fall back to general-purpose, got %q", def.Name)
	}
}
