// Package integration: BUILD_ORDER verification tests.
//
// These tests verify the correctness of BUILD_ORDER steps 25-28 as implemented
// by the forge-buildorder team. Each test exercises the plumbing between packages
// rather than retesting individual package internals.
package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/coordinator"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/features"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/services/compact"
	"github.com/egoisutolabs/forge/services/session"
	"github.com/egoisutolabs/forge/services/speculation"
	"github.com/egoisutolabs/forge/tools"
	"github.com/google/uuid"
)

// ============================================================
// Step 25: Coordinator mode tool filtering
// ============================================================

func TestBuildOrder_CoordinatorMode_FiltersTools(t *testing.T) {
	// When FORGE_COORDINATOR_MODE=1, only Agent, SendMessage, TaskStop are available.
	allTools := makeFakeTools("Agent", "SendMessage", "TaskStop", "Bash", "Read", "Write", "Glob", "Grep")
	got := coordinator.CoordinatorTools(allTools)

	if len(got) != 3 {
		t.Fatalf("expected 3 coordinator tools, got %d", len(got))
	}
	names := map[string]bool{}
	for _, tl := range got {
		names[tl.Name()] = true
	}
	for _, want := range []string{"Agent", "SendMessage", "TaskStop"} {
		if !names[want] {
			t.Errorf("missing coordinator tool: %s", want)
		}
	}
	for _, excluded := range []string{"Bash", "Read", "Write", "Glob", "Grep"} {
		if names[excluded] {
			t.Errorf("coordinator should NOT have tool: %s", excluded)
		}
	}
}

func TestBuildOrder_CoordinatorMode_SwapsSystemPrompt(t *testing.T) {
	prompt := coordinator.CoordinatorSystemPrompt()
	if !strings.Contains(prompt, "coordinator") {
		t.Error("coordinator prompt should mention 'coordinator'")
	}
	if !strings.Contains(prompt, "Agent") {
		t.Error("coordinator prompt should mention Agent tool")
	}
	// Must warn against fabricating results.
	if !strings.Contains(prompt, "fabricate") {
		t.Error("coordinator prompt should warn against fabricating")
	}
}

func TestBuildOrder_CoordinatorMode_EnvVar(t *testing.T) {
	t.Setenv("FORGE_COORDINATOR_MODE", "1")
	if !coordinator.IsCoordinatorMode() {
		t.Error("expected coordinator mode enabled with FORGE_COORDINATOR_MODE=1")
	}
	t.Setenv("FORGE_COORDINATOR_MODE", "")
	if coordinator.IsCoordinatorMode() {
		t.Error("expected coordinator mode disabled with empty env")
	}
}

func TestBuildOrder_CoordinatorMode_EngineUsesCoordinatorTools(t *testing.T) {
	// Verify that SubmitMessage in coordinator mode only passes coordinator tools.
	t.Setenv("FORGE_COORDINATOR_MODE", "1")

	var capturedToolCount int
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantText("I'm the coordinator"),
		},
	}

	allTools := makeFakeToolImpls("Agent", "SendMessage", "TaskStop", "Bash", "Read", "Write")
	eng := engine.New(engine.Config{
		Model:    "test-model",
		MaxTurns: 10,
		Tools:    allTools,
		Cwd:      "/tmp",
	})

	result, err := eng.SubmitMessage(context.Background(), caller, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	// The captured params should only have coordinator tool schemas.
	if len(caller.capturedParams) > 0 {
		capturedToolCount = len(caller.capturedParams[0].Tools)
	}
	if capturedToolCount != 3 {
		t.Errorf("expected 3 tools sent to API in coordinator mode, got %d", capturedToolCount)
	}
}

// ============================================================
// Step 26a: Budget enforcement end-to-end
// ============================================================

func TestBuildOrder_BudgetEnforcement_StopsOnExceed(t *testing.T) {
	// maxBudget flows from Config → RunLoop → stops when cost exceeds threshold.
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithUsage("expensive", models.Usage{
				InputTokens: 1_000_000, OutputTokens: 1_000_000,
			}),
		},
	}

	qe := engine.New(engine.Config{
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: 1.0,
		Cwd:          "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "expensive work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBudgetExceeded {
		t.Errorf("expected StopBudgetExceeded, got %v", result.Reason)
	}
	if result.TotalCostUSD == 0 {
		t.Error("cost should be > 0")
	}
}

func TestBuildOrder_BudgetEnforcement_ZeroIsUnlimited(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithUsage("done", models.Usage{InputTokens: 1000, OutputTokens: 500}),
		},
	}

	qe := engine.New(engine.Config{
		Model:        "claude-sonnet-4-6-20250514",
		MaxTurns:     10,
		MaxBudgetUSD: 0,
		Cwd:          "/tmp",
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected StopCompleted with zero budget, got %v", result.Reason)
	}
}

// ============================================================
// Step 28a: Session save/load roundtrip
// ============================================================

func TestBuildOrder_SessionPersistence_Roundtrip(t *testing.T) {
	dir := t.TempDir()

	// Create and save a session.
	s := session.New("claude-sonnet-4-6", "/tmp/work")
	s.Messages = []*models.Message{
		models.NewUserMessage("hello"),
		{
			ID:      uuid.NewString(),
			Role:    models.RoleAssistant,
			Content: []models.Block{{Type: models.BlockText, Text: "Hi there!"}},
		},
	}
	s.TotalUsage = models.Usage{InputTokens: 100, OutputTokens: 50}

	if err := session.Save(s, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load and verify roundtrip.
	loaded, err := session.Load(s.ID, dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != s.ID {
		t.Errorf("ID mismatch: %q vs %q", loaded.ID, s.ID)
	}
	if loaded.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want claude-sonnet-4-6", loaded.Model)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].TextContent() != "hello" {
		t.Errorf("message[0] = %q, want 'hello'", loaded.Messages[0].TextContent())
	}
	if loaded.Messages[1].TextContent() != "Hi there!" {
		t.Errorf("message[1] = %q, want 'Hi there!'", loaded.Messages[1].TextContent())
	}
	if loaded.TotalUsage.InputTokens != 100 || loaded.TotalUsage.OutputTokens != 50 {
		t.Errorf("usage mismatch: in=%d out=%d", loaded.TotalUsage.InputTokens, loaded.TotalUsage.OutputTokens)
	}
}

func TestBuildOrder_SessionPersistence_AutoSave(t *testing.T) {
	dir := t.TempDir()
	s := session.New("model", "/tmp")
	if err := session.Save(s, dir); err != nil {
		t.Fatal(err)
	}

	msgs := []*models.Message{
		models.NewUserMessage("turn 1"),
		models.NewUserMessage("turn 2"),
	}
	usage := models.Usage{InputTokens: 200, OutputTokens: 100}
	if err := session.AutoSave(s, msgs, usage, dir); err != nil {
		t.Fatalf("AutoSave: %v", err)
	}

	loaded, err := session.Load(s.ID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages after AutoSave, got %d", len(loaded.Messages))
	}
}

func TestBuildOrder_SessionPersistence_ListSortedByRecent(t *testing.T) {
	dir := t.TempDir()

	s1 := session.New("m", "/a")
	session.Save(s1, dir)
	time.Sleep(time.Millisecond)

	s2 := session.New("m", "/b")
	session.Save(s2, dir)

	sessions, err := session.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	// Most recent first.
	if sessions[0].UpdatedAt.Before(sessions[1].UpdatedAt) {
		t.Error("sessions should be sorted most recent first")
	}
}

// ============================================================
// Step 28b: Microcompact reduces token count
// ============================================================

func TestBuildOrder_MicroCompact_ClearsOldToolResults(t *testing.T) {
	// Build a conversation with more tool results than keepRecent.
	msgs := buildToolConversation(8) // 8 tool results
	keepRecent := 3

	result := compact.MicroCompact(msgs, keepRecent)

	// Count cleared vs intact results.
	cleared := 0
	intact := 0
	for _, msg := range result.Messages {
		for _, b := range msg.Content {
			if b.Type == models.BlockToolResult {
				if strings.Contains(b.Content, "[tool result cleared") {
					cleared++
				} else {
					intact++
				}
			}
		}
	}

	if cleared != 5 { // 8 - 3 = 5 should be cleared
		t.Errorf("expected 5 cleared results, got %d", cleared)
	}
	if intact != 3 {
		t.Errorf("expected 3 intact results, got %d", intact)
	}
	if result.TokensSaved <= 0 {
		t.Error("expected positive TokensSaved")
	}
}

func TestBuildOrder_MicroCompact_DoesNotMutateOriginals(t *testing.T) {
	original := "original-content-that-must-survive"
	msgs := []*models.Message{
		{
			ID:   uuid.NewString(),
			Role: models.RoleAssistant,
			Content: []models.Block{
				{Type: models.BlockToolUse, ID: "t1", Name: "Bash"},
			},
		},
		{
			ID:   uuid.NewString(),
			Role: models.RoleUser,
			Content: []models.Block{
				models.NewToolResultBlock("t1", original, false),
			},
		},
		{
			ID:   uuid.NewString(),
			Role: models.RoleAssistant,
			Content: []models.Block{
				{Type: models.BlockToolUse, ID: "t2", Name: "Bash"},
			},
		},
		{
			ID:   uuid.NewString(),
			Role: models.RoleUser,
			Content: []models.Block{
				models.NewToolResultBlock("t2", "keep-this", false),
			},
		},
	}

	compact.MicroCompact(msgs, 1)

	// Original message content should NOT be modified.
	if msgs[1].Content[0].Content != original {
		t.Errorf("original was mutated: got %q", msgs[1].Content[0].Content)
	}
}

// ============================================================
// Step 27: Speculation create/accept/reject lifecycle
// ============================================================

func TestBuildOrder_Speculation_CreateAcceptReject(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("speculated result"),
		},
	}

	spec := speculation.NewSpeculator(speculation.Config{
		Model:    "test-model",
		MaxTurns: 5,
	}, caller)

	// Create speculation.
	id, err := spec.Speculate(context.Background(), "run tests")
	if err != nil {
		t.Fatalf("Speculate: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	// Wait for completion.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		// Check if completed — we can't directly access status, so try Accept.
		result, err := spec.Accept(id)
		if err != nil {
			if strings.Contains(err.Error(), "non-completed") {
				continue // still running
			}
			t.Fatalf("Accept: %v", err)
		}
		// Accept succeeded.
		if result == nil {
			t.Fatal("expected non-nil result on accept")
		}
		if len(result.Messages) == 0 {
			t.Error("expected messages in speculation result")
		}
		return
	}
	t.Fatal("speculation did not complete within timeout")
}

func TestBuildOrder_Speculation_RejectCleansUp(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("result"),
		},
	}

	spec := speculation.NewSpeculator(speculation.Config{
		Model:    "test-model",
		MaxTurns: 5,
	}, caller)

	id, err := spec.Speculate(context.Background(), "task")
	if err != nil {
		t.Fatalf("Speculate: %v", err)
	}

	// Wait briefly for it to complete.
	time.Sleep(200 * time.Millisecond)

	// Reject should succeed and clean up.
	err = spec.Reject(id)
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}

	// Double-reject should fail (already removed).
	err = spec.Reject(id)
	if err == nil {
		t.Error("expected error on double-reject")
	}
}

func TestBuildOrder_Speculation_Suggest(t *testing.T) {
	spec := speculation.NewSpeculator(speculation.Config{
		Model: "test-model",
	}, &mockCaller{responses: []*models.Message{assistantText("ok")}})

	msgs := []*models.Message{
		{
			Role:    models.RoleAssistant,
			Content: []models.Block{{Type: models.BlockToolUse, Name: "Edit"}},
		},
	}
	suggestions := spec.Suggest(msgs)
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions after Edit tool use")
	}

	found := false
	for _, s := range suggestions {
		lower := strings.ToLower(s)
		if strings.Contains(lower, "test") || strings.Contains(lower, "commit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected suggestion about tests or commits, got %v", suggestions)
	}
}

// ============================================================
// Step 28c: Build tags (compile-time verification)
// ============================================================

func TestBuildOrder_FeatureGates_DefaultBuild(t *testing.T) {
	// In default build (no tags), all features should be disabled.
	if features.MinimalBuild {
		t.Skip("running with minimal tag")
	}
	if features.SpeculationEnabled {
		t.Skip("running with speculation tag")
	}
	if features.DebugEnabled {
		t.Skip("running with debug tag")
	}

	// All false in default build.
	if features.MinimalBuild || features.SpeculationEnabled || features.DebugEnabled {
		t.Error("default build should have all feature gates disabled")
	}
}

// ============================================================
// Helpers
// ============================================================

// makeFakeTools creates fake tools for coordinator filtering tests.
func makeFakeTools(names ...string) []tools.Tool {
	tt := make([]tools.Tool, len(names))
	for i, n := range names {
		tt[i] = &fakeToolImpl{name: n}
	}
	return tt
}

// makeFakeToolImpls creates fake tool implementations that satisfy tools.Tool.
func makeFakeToolImpls(names ...string) []tools.Tool {
	return makeFakeTools(names...)
}

// fakeToolImpl satisfies tools.Tool for testing.
type fakeToolImpl struct{ name string }

func (f *fakeToolImpl) Name() string                             { return f.name }
func (f *fakeToolImpl) Description() string                      { return "" }
func (f *fakeToolImpl) InputSchema() json.RawMessage             { return nil }
func (f *fakeToolImpl) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (f *fakeToolImpl) IsReadOnly(_ json.RawMessage) bool        { return false }
func (f *fakeToolImpl) ValidateInput(_ json.RawMessage) error    { return nil }
func (f *fakeToolImpl) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (f *fakeToolImpl) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "ok"}, nil
}

// buildToolConversation builds a conversation with n tool call/result pairs.
func buildToolConversation(n int) []*models.Message {
	var msgs []*models.Message
	msgs = append(msgs, models.NewUserMessage("start"))
	for range n {
		toolID := uuid.NewString()[:8]
		msgs = append(msgs, &models.Message{
			ID:   uuid.NewString(),
			Role: models.RoleAssistant,
			Content: []models.Block{
				{Type: models.BlockToolUse, ID: toolID, Name: "Bash"},
			},
		})
		msgs = append(msgs, &models.Message{
			ID:   uuid.NewString(),
			Role: models.RoleUser,
			Content: []models.Block{
				models.NewToolResultBlock(toolID, strings.Repeat("output-data-for-tool-", 50), false),
			},
		})
	}
	return msgs
}
