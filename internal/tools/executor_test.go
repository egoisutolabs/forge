package tools

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
)

type countingTool struct {
	execCount atomic.Int32
}

func (s *countingTool) Name() string        { return "Counter" }
func (s *countingTool) Description() string { return "Counts executions" }
func (s *countingTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
}
func (s *countingTool) Execute(_ context.Context, input json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	s.execCount.Add(1)
	var p struct {
		Text string `json:"text"`
	}
	json.Unmarshal(input, &p)
	return &models.ToolResult{Content: p.Text}, nil
}
func (s *countingTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *countingTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (s *countingTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *countingTool) IsReadOnly(_ json.RawMessage) bool        { return true }

type unsafeTool struct{ echoTool }

func (u *unsafeTool) Name() string                             { return "Unsafe" }
func (u *unsafeTool) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (u *unsafeTool) IsReadOnly(_ json.RawMessage) bool        { return false }

func TestPartitionBatches_Empty(t *testing.T) {
	if batches := PartitionBatches(nil); batches != nil {
		t.Errorf("expected nil, got %v", batches)
	}
}

func TestPartitionBatches_AllSafe(t *testing.T) {
	tool := &echoTool{}
	calls := []ToolCall{
		{Block: models.Block{ID: "1", Input: json.RawMessage(`{}`)}, Tool: tool},
		{Block: models.Block{ID: "2", Input: json.RawMessage(`{}`)}, Tool: tool},
		{Block: models.Block{ID: "3", Input: json.RawMessage(`{}`)}, Tool: tool},
	}
	batches := PartitionBatches(calls)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if !batches[0].ConcurrencySafe {
		t.Error("expected batch to be concurrency safe")
	}
	if len(batches[0].Calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(batches[0].Calls))
	}
}

func TestPartitionBatches_Mixed(t *testing.T) {
	safe := &echoTool{}
	unsafe := &unsafeTool{}

	calls := []ToolCall{
		{Block: models.Block{ID: "1", Input: json.RawMessage(`{}`)}, Tool: safe},
		{Block: models.Block{ID: "2", Input: json.RawMessage(`{}`)}, Tool: safe},
		{Block: models.Block{ID: "3", Input: json.RawMessage(`{}`)}, Tool: unsafe},
		{Block: models.Block{ID: "4", Input: json.RawMessage(`{}`)}, Tool: safe},
		{Block: models.Block{ID: "5", Input: json.RawMessage(`{}`)}, Tool: safe},
	}
	batches := PartitionBatches(calls)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if !batches[0].ConcurrencySafe || len(batches[0].Calls) != 2 {
		t.Errorf("batch 0: safe=%v calls=%d", batches[0].ConcurrencySafe, len(batches[0].Calls))
	}
	if batches[1].ConcurrencySafe || len(batches[1].Calls) != 1 {
		t.Errorf("batch 1: safe=%v calls=%d", batches[1].ConcurrencySafe, len(batches[1].Calls))
	}
	if !batches[2].ConcurrencySafe || len(batches[2].Calls) != 2 {
		t.Errorf("batch 2: safe=%v calls=%d", batches[2].ConcurrencySafe, len(batches[2].Calls))
	}
}

func TestExecuteBatches_RunsAll(t *testing.T) {
	tool := &countingTool{}
	calls := []ToolCall{
		{Block: models.Block{ID: "t1", Name: "Counter", Input: json.RawMessage(`{"text":"a"}`)}, Tool: tool},
		{Block: models.Block{ID: "t2", Name: "Counter", Input: json.RawMessage(`{"text":"b"}`)}, Tool: tool},
	}
	batches := PartitionBatches(calls)
	results := ExecuteBatches(context.Background(), batches, nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if tool.execCount.Load() != 2 {
		t.Errorf("expected 2 executions, got %d", tool.execCount.Load())
	}
	if results[0].Content != "a" {
		t.Errorf("expected first result %q, got %q", "a", results[0].Content)
	}
}

func TestResolveToolCalls_Unknown(t *testing.T) {
	tools := []Tool{&echoTool{}}
	blocks := []models.Block{
		{ID: "t1", Name: "Echo", Input: json.RawMessage(`{}`)},
		{ID: "t2", Name: "DoesNotExist", Input: json.RawMessage(`{}`)},
	}
	resolved, unknown := ResolveToolCalls(blocks, tools)
	if len(resolved) != 1 {
		t.Errorf("expected 1 resolved, got %d", len(resolved))
	}
	if len(unknown) != 1 || !unknown[0].IsError {
		t.Errorf("expected 1 unknown error, got %d", len(unknown))
	}
}

func TestExecuteToolBlocks_EndToEnd(t *testing.T) {
	blocks := []models.Block{
		{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"hello"}`)},
		{ID: "t2", Name: "Missing", Input: json.RawMessage(`{}`)},
	}
	results := ExecuteToolBlocks(context.Background(), blocks, []Tool{&echoTool{}}, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	resultMap := make(map[string]models.Block)
	for _, r := range results {
		resultMap[r.ToolUseID] = r
	}
	if r := resultMap["t2"]; !r.IsError {
		t.Error("expected error for unknown tool")
	}
	if r := resultMap["t1"]; r.IsError || r.Content != "hello" {
		t.Errorf("unexpected result for t1: %+v", r)
	}
}

// ── PermAsk handling ──────────────────────────────────────────────────────────

// askTool always returns PermAsk with a fixed message.
type askTool struct{}

func (a *askTool) Name() string                             { return "Asker" }
func (a *askTool) Description() string                      { return "always asks" }
func (a *askTool) InputSchema() json.RawMessage             { return json.RawMessage(`{}`) }
func (a *askTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (a *askTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (a *askTool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (a *askTool) Execute(_ context.Context, _ json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "executed"}, nil
}
func (a *askTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAsk, Message: "run asker?"}, nil
}

func TestExecuteSingle_PermAsk_NilContext_Denies(t *testing.T) {
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	result := executeSingle(context.Background(), call, nil)
	if !result.IsError {
		t.Error("PermAsk with nil context should be denied")
	}
}

func TestExecuteSingle_PermAsk_NilPrompt_Denies(t *testing.T) {
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	tctx := &ToolContext{PermissionPrompt: nil}
	result := executeSingle(context.Background(), call, tctx)
	if !result.IsError {
		t.Error("PermAsk with nil PermissionPrompt should be denied")
	}
}

func TestExecuteSingle_PermAsk_PromptApproves_Executes(t *testing.T) {
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	var promptMsg string
	tctx := &ToolContext{
		PermissionPrompt: func(msg string) bool {
			promptMsg = msg
			return true // approve
		},
	}
	result := executeSingle(context.Background(), call, tctx)
	if result.IsError {
		t.Errorf("PermAsk with approving prompt should execute, got: %s", result.Content)
	}
	if result.Content != "executed" {
		t.Errorf("Content = %q, want executed", result.Content)
	}
	if promptMsg != "run asker?" {
		t.Errorf("prompt message = %q, want 'run asker?'", promptMsg)
	}
}

func TestExecuteSingle_PermAsk_PromptDenies_Denies(t *testing.T) {
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	tctx := &ToolContext{
		PermissionPrompt: func(_ string) bool { return false }, // deny
	}
	result := executeSingle(context.Background(), call, tctx)
	if !result.IsError {
		t.Error("PermAsk with denying prompt should be denied")
	}
}

// ── permissions.Context overlay (SEC-5) ──────────────────────────────────────

func TestExecuteSingle_PermissionsContext_DenyRule_Denies(t *testing.T) {
	// echoTool returns PermAllow, but a deny rule in permissions.Context overrides it.
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"hi"}`)},
		Tool:  &echoTool{},
	}
	tctx := &ToolContext{
		Permissions: &permissions.Context{
			Mode: models.ModeDefault,
			DenyRules: []models.PermissionRule{
				{ToolName: "Echo", Behavior: models.PermDeny, Source: "test"},
			},
		},
	}
	result := executeSingle(context.Background(), call, tctx)
	if !result.IsError {
		t.Error("deny rule in permissions.Context should deny tool execution")
	}
}

func TestExecuteSingle_PermissionsContext_PlanMode_DeniesWrite(t *testing.T) {
	// echoTool is non-read-only (unsafeTool variant) and plan mode blocks writes.
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Unsafe", Input: json.RawMessage(`{}`)},
		Tool:  &unsafeTool{},
	}
	tctx := &ToolContext{
		Permissions: &permissions.Context{Mode: models.ModePlan},
	}
	result := executeSingle(context.Background(), call, tctx)
	if !result.IsError {
		t.Error("plan mode should deny non-read-only tool")
	}
}

func TestExecuteSingle_PermissionsContext_BypassPermissions_AllowsAsk(t *testing.T) {
	// askTool returns PermAsk; bypassPermissions should promote to Allow without a prompt.
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	tctx := &ToolContext{
		Permissions: &permissions.Context{Mode: models.ModeBypassPermissions},
	}
	result := executeSingle(context.Background(), call, tctx)
	if result.IsError {
		t.Errorf("bypassPermissions should allow without prompt, got error: %s", result.Content)
	}
}

func TestExecuteSingle_PermissionsContext_AllowRule_AllowsAsk(t *testing.T) {
	// askTool returns PermAsk; an explicit allow rule in permissions.Context overrides to Allow.
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Asker", Input: json.RawMessage(`{}`)},
		Tool:  &askTool{},
	}
	tctx := &ToolContext{
		Permissions: &permissions.Context{
			Mode: models.ModeDefault,
			AllowRules: []models.PermissionRule{
				{ToolName: "Asker", Behavior: models.PermAllow, Source: "test"},
			},
		},
	}
	result := executeSingle(context.Background(), call, tctx)
	if result.IsError {
		t.Errorf("allow rule should promote PermAsk to Allow, got error: %s", result.Content)
	}
}
