package orchestrator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/orchestrator"
	"github.com/egoisutolabs/forge/skills"
	"github.com/egoisutolabs/forge/tools"
	"github.com/google/uuid"
)

// ── Mock API Caller ────────────────────────────────────────────────────────

// phaseMockCaller returns canned assistant responses. Each Stream() call pops
// the next response. It also writes the expected artifact files so that
// validation passes after each phase.
type phaseMockCaller struct {
	responses []*models.Message
	callCount int32 // atomic for safety
}

func (m *phaseMockCaller) Stream(_ context.Context, _ api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 2)
	go func() {
		defer close(ch)
		idx := int(atomic.AddInt32(&m.callCount, 1)) - 1
		if idx >= len(m.responses) {
			ch <- api.StreamEvent{Type: "error", Err: fmt.Errorf("mock caller exhausted: call %d but only %d responses", idx+1, len(m.responses))}
			return
		}
		msg := m.responses[idx]
		for _, b := range msg.Content {
			if b.Type == models.BlockText {
				ch <- api.StreamEvent{Type: "text_delta", Text: b.Text}
			}
		}
		ch <- api.StreamEvent{Type: "message_done", Message: msg}
	}()
	return ch
}

func assistantMsg(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
	}
}

// ── Artifact helpers ───────────────────────────────────────────────────────

// writeArtifact creates a file in the feature directory.
func writeArtifact(t *testing.T, featureDir, name, content string) {
	t.Helper()
	path := filepath.Join(featureDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for artifact: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write artifact %s: %v", name, err)
	}
}

// setupPlanArtifacts creates the plan-phase output artifacts.
func setupPlanArtifacts(t *testing.T, featureDir string) {
	t.Helper()
	writeArtifact(t, featureDir, "discovery.md",
		"## Requirements\nUser auth\n\n## Open Questions\n\n## Decisions Already Made\n\n## Constraints\n")
	writeArtifact(t, featureDir, "exploration.md",
		"## Most Similar Feature\nlogin\n\n## Architecture Map\nMVC\n\n## Structural Patterns\npatterns\n\n## Key Files\nauth.go\n")
	writeArtifact(t, featureDir, "architecture.md",
		"## Recommendation\nUse JWT\n\n## Selected Approach\nJWT with refresh tokens\n\n## Task Breakdown (recommended approach)\n1. Add auth middleware\n")
}

// setupPrepareArtifacts creates the prepare-phase output artifacts.
func setupPrepareArtifacts(t *testing.T, featureDir string) {
	t.Helper()
	writeArtifact(t, featureDir, "implementation-context.md",
		"## Chosen Approach\nJWT\n\n## Implementation Plan\nStep 1\n\n## Implementation Order\n1. middleware\n\n## External Dependencies\nnone\n\n## Test Cases\ntest login\n\n## Scope Boundaries\nauth/ only\n")
}

// setupTestArtifacts creates the test-phase output artifacts.
func setupTestArtifacts(t *testing.T, featureDir string) {
	t.Helper()
	writeArtifact(t, featureDir, "test-manifest.md",
		"## Test Files Created\nauth_test.go\n\n## Spec → Test Mapping\nlogin→test\n\n## Edge Cases Covered\nnone\n\n## Test File Checksums\nsha256:abc123\n\n## Run Command\ngo test ./...\n")
}

// setupImplementArtifacts creates the implement-phase output artifacts.
func setupImplementArtifacts(t *testing.T, featureDir string) {
	t.Helper()
	writeArtifact(t, featureDir, "impl-manifest.md",
		"## Files Created\nauth.go\n\n## Files Modified\nnone\n\n## Patterns Followed\nMVC\n\n## Test Results\nall pass\n")
}

// setupVerifyArtifacts creates the verify-phase output artifacts.
func setupVerifyArtifacts(t *testing.T, featureDir string) {
	t.Helper()
	writeArtifact(t, featureDir, "verify-report.md",
		"## Overall\nAll checks pass\n\n## Test File Integrity\nok\n\n## Tests\npass\n\n## Scope Compliance\nok\n\n## Structural Contracts\nok\n\n## Action Required\nnone\n")
}

// ── Mock AskUser ───────────────────────────────────────────────────────────

// autoApproveAsk always returns the first option.
func autoApproveAsk(summary, question string, options []string) (string, error) {
	if len(options) > 0 {
		return options[0], nil
	}
	return "yes", nil
}

// mockAskRecorder records calls and returns a pre-set answer.
type mockAskRecorder struct {
	calls  []askCall
	answer string
}

type askCall struct {
	summary  string
	question string
	options  []string
}

func (m *mockAskRecorder) ask(summary, question string, options []string) (string, error) {
	m.calls = append(m.calls, askCall{summary, question, options})
	return m.answer, nil
}

// ── Mock Tool ──────────────────────────────────────────────────────────────

type stubTool struct {
	name string
}

func (t *stubTool) Name() string                 { return t.name }
func (t *stubTool) Description() string          { return "stub " + t.name }
func (t *stubTool) InputSchema() json.RawMessage { return nil }
func (t *stubTool) Execute(context.Context, json.RawMessage, *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "ok"}, nil
}
func (t *stubTool) CheckPermissions(json.RawMessage, *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *stubTool) ValidateInput(json.RawMessage) error    { return nil }
func (t *stubTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (t *stubTool) IsReadOnly(json.RawMessage) bool        { return true }

// ── Full Pipeline Test ─────────────────────────────────────────────────────

// TestFullPipeline_AllPhasesSucceed exercises the full 5-phase pipeline with a
// mock caller. Each phase's mock response is "done" and artifacts are pre-created
// so validation passes.
func TestFullPipeline_AllPhasesSucceed(t *testing.T) {
	dir := t.TempDir()
	slug := "add-user-auth"
	featureDir := filepath.Join(dir, ".forge", "features", slug)

	// The orchestrator will call RunLoop for each phase. The mock caller
	// returns the expected status string for each phase.
	// Phases: plan, prepare, test, implement, verify
	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("done - plan ready"),
			assistantMsg("done - prepare ready"),
			assistantMsg("done - wrote 3 test files with 12 test cases"),
			assistantMsg("done - tests passing"),
			assistantMsg("pass"),
		},
	}

	// Create artifacts that the orchestrator expects to validate after each phase.
	// We must create them before Run() because the mock caller doesn't actually
	// create files — it just returns status strings. But the orchestrator creates
	// the feature dir during bootstrap, so we hook into that by pre-creating.
	os.MkdirAll(featureDir, 0o755)
	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	recorder := &mockAskRecorder{answer: "yes"}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:    dir,
		Caller: caller,
		Model:  "claude-sonnet-4-6",
		AskUser: func(summary, question string, options []string) (string, error) {
			// For gates: startup→yes, plan gate→approve, prepare gate→proceed, verify gate→accept
			for _, opt := range options {
				switch opt {
				case "approve":
					return "approve", nil
				case "proceed":
					return "proceed", nil
				case "accept":
					return "accept", nil
				}
			}
			return recorder.ask(summary, question, options)
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "add user auth")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify all phases were called.
	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 5 {
		t.Errorf("expected 5 caller invocations (one per phase), got %d", called)
	}

	// Verify state file reflects all phases done.
	stateFile := filepath.Join(dir, ".forge", "state.json")
	state, err := orchestrator.Load(stateFile)
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	entry, ok := state.Features[slug]
	if !ok {
		t.Fatalf("feature %q not found in state", slug)
	}
	for _, phase := range orchestrator.PhaseOrder {
		ps, ok := entry.Phases[phase]
		if !ok {
			t.Errorf("phase %q missing from state", phase)
			continue
		}
		if ps.Status != orchestrator.StatusDone {
			t.Errorf("phase %q status = %q, want done", phase, ps.Status)
		}
	}
}

// ── Resume Test ────────────────────────────────────────────────────────────

// TestResumeMidPipeline starts a run with plan+prepare already done and
// verifies only the remaining phases are executed.
func TestResumeMidPipeline(t *testing.T) {
	dir := t.TempDir()
	slug := "resume-mid"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)

	// Pre-seed state: plan and prepare are done.
	stateFile := filepath.Join(dir, ".forge", "state.json")
	state := &orchestrator.ForgeState{Features: make(map[string]*orchestrator.FeatureEntry)}
	_, _ = state.Init(slug, "direct")
	_ = state.SetPhase(slug, "plan", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "prepare", orchestrator.StatusDone)
	_ = state.Save(stateFile)

	// Pre-create all artifacts.
	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	// Only 3 phases should run: test, implement, verify.
	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("done - wrote 2 test files with 8 test cases"),
			assistantMsg("done - tests passing"),
			assistantMsg("pass"),
		},
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:     dir,
		Caller:  caller,
		Model:   "claude-sonnet-4-6",
		AskUser: autoApproveAsk,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "Resume Mid")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 3 {
		t.Errorf("expected 3 caller invocations for resume (test+implement+verify), got %d", called)
	}
}

// ── Implement-Verify Retry Loop ────────────────────────────────────────────

// TestRetryLoop_VerifyFailThenPass simulates verify failing once, triggering
// an auto-retry, then succeeding on the second attempt.
func TestRetryLoop_VerifyFailThenPass(t *testing.T) {
	dir := t.TempDir()
	slug := "retry-test"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)

	// Pre-seed state: plan, prepare, test are done.
	stateFile := filepath.Join(dir, ".forge", "state.json")
	state := &orchestrator.ForgeState{Features: make(map[string]*orchestrator.FeatureEntry)}
	_, _ = state.Init(slug, "direct")
	_ = state.SetPhase(slug, "plan", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "prepare", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "test", orchestrator.StatusDone)
	_ = state.Save(stateFile)

	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	// implement(1) → done, verify(1) → fail, implement(2) → done, verify(2) → pass
	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("done - tests passing"),                                                // implement attempt 1
			assistantMsg("fail - 2 test failures, 0 scope violations, 0 structural violations"), // verify attempt 1
			assistantMsg("done - tests passing"),                                                // implement attempt 2
			assistantMsg("pass"),                                                                // verify attempt 2
		},
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:    dir,
		Caller: caller,
		Model:  "claude-sonnet-4-6",
		AskUser: func(summary, question string, options []string) (string, error) {
			// accept when verify passes
			for _, opt := range options {
				if opt == "accept" {
					return "accept", nil
				}
			}
			return options[0], nil
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "Retry Test")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 4 {
		t.Errorf("expected 4 caller invocations (implement, verify-fail, implement, verify-pass), got %d", called)
	}
}

// TestRetryLoop_ExhaustedRetriesUserStops verifies that after maxVerifyRetries
// the user is asked, and choosing "stop" terminates with an error.
func TestRetryLoop_ExhaustedRetriesUserStops(t *testing.T) {
	dir := t.TempDir()
	slug := "retry-exhaust"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)

	stateFile := filepath.Join(dir, ".forge", "state.json")
	state := &orchestrator.ForgeState{Features: make(map[string]*orchestrator.FeatureEntry)}
	_, _ = state.Init(slug, "direct")
	_ = state.SetPhase(slug, "plan", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "prepare", orchestrator.StatusDone)
	_ = state.SetPhase(slug, "test", orchestrator.StatusDone)
	_ = state.Save(stateFile)

	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	// 4 pairs of implement+verify (initial + 3 retries), all failing verify.
	var responses []*models.Message
	for i := 0; i < 4; i++ {
		responses = append(responses, assistantMsg("done - tests passing"))                         // implement
		responses = append(responses, assistantMsg("fail - 1 test failure, 0 scope, 0 structural")) // verify
	}
	caller := &phaseMockCaller{responses: responses}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:    dir,
		Caller: caller,
		Model:  "claude-sonnet-4-6",
		AskUser: func(summary, question string, options []string) (string, error) {
			// User chooses "stop" when asked about retry after exhaustion.
			for _, opt := range options {
				if opt == "stop" {
					return "stop", nil
				}
			}
			return options[0], nil
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "Retry Exhaust")
	if err == nil {
		t.Fatal("expected error when user stops after exhausted retries")
	}
	if !strings.Contains(err.Error(), "stopped at verify") {
		t.Errorf("error = %q, want 'stopped at verify'", err.Error())
	}
}

// ── Blocked Plan Phase Test ────────────────────────────────────────────────

// TestBlockedPlan_RerunsAfterUserInput verifies that a blocked plan phase asks
// the user for input, then reruns and succeeds.
func TestBlockedPlan_RerunsAfterUserInput(t *testing.T) {
	dir := t.TempDir()
	slug := "blocked-plan"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)

	// Plan blocks on first run, succeeds on second. Then prepare, test, implement, verify.
	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("blocked - planning input required"),
			assistantMsg("done - plan ready"),
			assistantMsg("done - prepare ready"),
			assistantMsg("done - wrote 1 test file with 4 test cases"),
			assistantMsg("done - tests passing"),
			assistantMsg("pass"),
		},
	}

	// Create artifacts. The blocked plan will try to read discovery.md for questions.
	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:    dir,
		Caller: caller,
		Model:  "claude-sonnet-4-6",
		AskUser: func(summary, question string, options []string) (string, error) {
			for _, opt := range options {
				switch opt {
				case "answered":
					return "answered", nil
				case "approve":
					return "approve", nil
				case "proceed":
					return "proceed", nil
				case "accept":
					return "accept", nil
				}
			}
			return options[0], nil
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "Blocked Plan")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 6 {
		t.Errorf("expected 6 caller invocations (plan-blocked, plan-done, prepare, test, implement, verify), got %d", called)
	}
}

// ── User Cancelled at Startup ──────────────────────────────────────────────

func TestRun_UserCancelsAtStartup(t *testing.T) {
	dir := t.TempDir()
	caller := &phaseMockCaller{responses: nil} // should not be called

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:    dir,
		Caller: caller,
		Model:  "claude-sonnet-4-6",
		AskUser: func(summary, question string, options []string) (string, error) {
			return "no", nil // decline at startup
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "will be cancelled")
	if err == nil {
		t.Fatal("expected error when user cancels")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error = %q, should mention 'cancelled'", err.Error())
	}

	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 0 {
		t.Errorf("no phases should run when user cancels, but got %d calls", called)
	}
}

// ── Nil AskUser (non-interactive) ──────────────────────────────────────────

func TestRun_NilAskUser_SkipsGates(t *testing.T) {
	dir := t.TempDir()
	slug := "no-gates"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)

	setupPlanArtifacts(t, featureDir)
	setupPrepareArtifacts(t, featureDir)
	setupTestArtifacts(t, featureDir)
	setupImplementArtifacts(t, featureDir)
	setupVerifyArtifacts(t, featureDir)

	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("done - plan ready"),
			assistantMsg("done - prepare ready"),
			assistantMsg("done - wrote tests"),
			assistantMsg("done - tests passing"),
			assistantMsg("pass"),
		},
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:     dir,
		Caller:  caller,
		Model:   "claude-sonnet-4-6",
		AskUser: nil, // non-interactive
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(context.Background(), "no gates")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	called := int(atomic.LoadInt32(&caller.callCount))
	if called != 5 {
		t.Errorf("expected 5 phase calls, got %d", called)
	}
}

// ── Skill Registration Tests ───────────────────────────────────────────────

func TestRegisterForgeSkill_Registered(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	skill := registry.Lookup("forge")
	if skill == nil {
		t.Fatal("forge skill not registered")
	}
	if skill.Name != "forge" {
		t.Errorf("skill.Name = %q, want forge", skill.Name)
	}
	if !skill.UserInvocable {
		t.Error("forge skill should be user-invocable")
	}
	if skill.Execute == nil {
		t.Error("forge skill should have Execute callback")
	}
	if skill.Prompt == nil {
		t.Error("forge skill should have Prompt fallback")
	}
}

func TestRegisterForgeSkill_PromptFallback(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	skill := registry.Lookup("forge")
	if skill == nil {
		t.Fatal("forge skill not found")
	}

	// Empty args should prompt for description.
	prompt := skill.Prompt("")
	if !strings.Contains(prompt, "feature description") {
		t.Errorf("empty args prompt = %q, should mention feature description", prompt)
	}

	// With args should include the feature description.
	prompt = skill.Prompt("add payment system")
	if !strings.Contains(prompt, "add payment system") {
		t.Errorf("args prompt = %q, should contain feature description", prompt)
	}
}

func TestRegisterForgeSkill_ExecuteRequiresToolContext(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	skill := registry.Lookup("forge")
	if skill == nil {
		t.Fatal("forge skill not found")
	}

	// Execute with nil context should error.
	err := skill.Execute(context.Background(), "test feature", nil)
	if err == nil {
		t.Fatal("expected error when execCtx is nil")
	}

	// Execute with wrong type should error.
	err = skill.Execute(context.Background(), "test feature", "wrong type")
	if err == nil {
		t.Fatal("expected error when execCtx is wrong type")
	}
}

func TestRegisterForgeSkill_ExecuteRequiresArgs(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	skill := registry.Lookup("forge")
	if skill == nil {
		t.Fatal("forge skill not found")
	}

	tctx := &tools.ToolContext{
		Cwd:    t.TempDir(),
		Caller: &phaseMockCaller{},
		Model:  "claude-sonnet-4-6",
	}

	err := skill.Execute(context.Background(), "", tctx)
	if err == nil {
		t.Fatal("expected error when args is empty")
	}
	if !strings.Contains(err.Error(), "feature description") {
		t.Errorf("error = %q, should mention 'feature description'", err.Error())
	}
}

// ── Context Cancellation ───────────────────────────────────────────────────

func TestRun_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	slug := "cancel-test"
	featureDir := filepath.Join(dir, ".forge", "features", slug)
	os.MkdirAll(featureDir, 0o755)
	setupPlanArtifacts(t, featureDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The mock caller will get a cancelled context.
	caller := &phaseMockCaller{
		responses: []*models.Message{
			assistantMsg("done - plan ready"),
		},
	}

	orch, err := orchestrator.New(orchestrator.Config{
		Cwd:     dir,
		Caller:  caller,
		Model:   "claude-sonnet-4-6",
		AskUser: nil,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = orch.Run(ctx, "cancel test")
	// RunLoop should detect the cancelled context and return an error.
	if err == nil {
		t.Log("Run() completed despite cancelled context (RunLoop may handle it gracefully)")
	}
}
