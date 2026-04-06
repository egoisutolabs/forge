// Package e2e contains end-to-end tests for the full agent loop.
//
// These tests use a real QueryEngine with real tool implementations but a
// mock API caller, so they exercise the entire stack without hitting the
// live Claude API. Each test creates isolated state using t.TempDir().
//
// Run with: go test ./tests/e2e/...
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/askuser"
	"github.com/egoisutolabs/forge/tools/bash"
	"github.com/egoisutolabs/forge/tools/fileedit"
	"github.com/egoisutolabs/forge/tools/fileread"
	"github.com/egoisutolabs/forge/tools/glob"
	"github.com/egoisutolabs/forge/tools/grep"
	"github.com/egoisutolabs/forge/tools/planmode"
	"github.com/google/uuid"
)

// ── Mock API caller ──────────────────────────────────────────────────────────

// mockCaller replays a sequence of pre-scripted responses. Each call to
// Stream() delivers the next response in the queue.
type mockCaller struct {
	responses []*models.Message
	callCount int
}

func (m *mockCaller) Stream(_ context.Context, _ api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 2)
	go func() {
		defer close(ch)
		if m.callCount >= len(m.responses) {
			ch <- api.StreamEvent{Type: "error", Err: context.DeadlineExceeded}
			return
		}
		msg := m.responses[m.callCount]
		m.callCount++
		for _, b := range msg.Content {
			if b.Type == models.BlockText {
				ch <- api.StreamEvent{Type: "text_delta", Text: b.Text}
			}
		}
		ch <- api.StreamEvent{Type: "message_done", Message: msg}
	}()
	return ch
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// assistantText returns an assistant message containing a single text block.
func assistantText(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
	}
}

// toolUseMsg returns an assistant message with a single tool_use block.
func toolUseMsg(toolName, toolID string, input any) *models.Message {
	data, _ := json.Marshal(input)
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopToolUse,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: toolID, Name: toolName, Input: json.RawMessage(data)},
		},
	}
}

// mustWriteFile writes content to a file, failing the test on error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("mustWriteFile %s: %v", path, err)
	}
}

// ── Scenario 1: Full conversation — FileReadTool ──────────────────────────

// TestE2E_FullConversation_FileRead simulates a user asking the agent to read
// a file. The mock API returns a tool_use block for the Read tool, the real
// FileReadTool executes against a temp file, and the final assistant message
// references the file content.
func TestE2E_FullConversation_FileRead(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "hello.txt")
	mustWriteFile(t, filePath, "hello world\n")

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: agent decides to read the file.
			toolUseMsg("Read", "r1", map[string]any{"file_path": filePath}),
			// Turn 2: agent responds after seeing file content.
			assistantText("The file contains: hello world"),
		},
	}

	qe := engine.New(engine.Config{
		Tools:    []tools.Tool{&fileread.Tool{}},
		Cwd:      tmpDir,
		MaxTurns: 10,
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "Read hello.txt please")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	msgs := qe.Messages()
	// user → assistant(tool_use) → user(tool_result) → assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Tool result must contain the file content.
	toolResultMsg := msgs[2]
	if len(toolResultMsg.Content) == 0 {
		t.Fatal("expected at least 1 tool result block")
	}
	if !strings.Contains(toolResultMsg.Content[0].Content, "hello world") {
		t.Errorf("tool result should contain file content, got: %s", toolResultMsg.Content[0].Content)
	}
	if toolResultMsg.Content[0].IsError {
		t.Errorf("tool result should not be an error, got: %s", toolResultMsg.Content[0].Content)
	}

	// Final assistant message references the file content.
	lastMsg := msgs[len(msgs)-1]
	if !strings.Contains(lastMsg.TextContent(), "hello world") {
		t.Errorf("final message should mention file content, got: %s", lastMsg.TextContent())
	}
}

// ── Scenario 2: Multi-turn — Read then Edit ───────────────────────────────

// TestE2E_MultiTurn_ReadThenEdit submits two messages to the same QueryEngine.
// The first turn reads a file (populating the FileStateCache). The second turn
// edits it via the real FileEditTool. Verifies history accumulates and the
// file on disk is actually changed.
func TestE2E_MultiTurn_ReadThenEdit(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "editable.txt")
	mustWriteFile(t, filePath, "original content\n")

	caller1 := &mockCaller{
		responses: []*models.Message{
			toolUseMsg("Read", "r1", map[string]any{"file_path": filePath}),
			assistantText("I read the file. It contains 'original content'."),
		},
	}
	caller2 := &mockCaller{
		responses: []*models.Message{
			toolUseMsg("Edit", "e1", map[string]any{
				"file_path":  filePath,
				"old_string": "original content",
				"new_string": "updated content",
			}),
			assistantText("I updated the file."),
		},
	}

	qe := engine.New(engine.Config{
		Tools:            []tools.Tool{&fileread.Tool{}, &fileedit.Tool{}},
		Cwd:              tmpDir,
		MaxTurns:         10,
		PermissionPrompt: func(_ string) bool { return true }, // E2E: auto-approve
	})

	// Turn 1: read.
	if _, err := qe.SubmitMessage(context.Background(), caller1, "Read the file"); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	afterTurn1 := len(qe.Messages())

	// Turn 2: edit (FileStateCache from turn 1 satisfies the cache gate).
	if _, err := qe.SubmitMessage(context.Background(), caller2, "Now update the file"); err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	afterTurn2 := len(qe.Messages())

	// History must grow across turns.
	if afterTurn2 <= afterTurn1 {
		t.Errorf("history should grow: after turn1=%d after turn2=%d", afterTurn1, afterTurn2)
	}

	// Verify the first turn's messages are still present.
	msgs := qe.Messages()
	if !strings.Contains(msgs[0].TextContent(), "Read the file") {
		t.Errorf("first user message preserved incorrectly: %s", msgs[0].TextContent())
	}

	// File on disk must be updated.
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading edited file: %v", err)
	}
	if !strings.Contains(string(raw), "updated content") {
		t.Errorf("file should contain 'updated content', got: %s", string(raw))
	}
}

// ── Scenario 3: Tool chaining — Glob → Grep → Read ────────────────────────

// TestE2E_ToolChaining_GlobGrepRead chains three read-only tools across three
// turns to simulate a codebase exploration workflow. Each tool result is
// recorded in the conversation history and the next tool call uses pre-known
// file paths (determined at test setup time).
//
// Requires the rg (ripgrep) binary; the test is skipped otherwise.
func TestE2E_ToolChaining_GlobGrepRead(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg (ripgrep) not found in PATH; skipping grep chain test")
	}

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "main.go")
	mustWriteFile(t, goFile, "package main\n\nfunc hello() string {\n\treturn \"searchterm\"\n}\n")
	mustWriteFile(t, filepath.Join(tmpDir, "other.txt"), "other file\n")

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: Glob for *.go files.
			toolUseMsg("Glob", "g1", map[string]any{"pattern": "**/*.go", "path": tmpDir}),
			// Turn 2: Grep for "searchterm" in tmpDir.
			toolUseMsg("Grep", "g2", map[string]any{
				"pattern":     "searchterm",
				"path":        tmpDir,
				"output_mode": "files_with_matches",
			}),
			// Turn 3: Read the known Go file.
			toolUseMsg("Read", "r1", map[string]any{"file_path": goFile}),
			// Turn 4: final answer.
			assistantText("I found the function containing 'searchterm' in main.go."),
		},
	}

	qe := engine.New(engine.Config{
		Tools:    []tools.Tool{&glob.Tool{}, &grep.Tool{}, &fileread.Tool{}},
		Cwd:      tmpDir,
		MaxTurns: 10,
	})

	result, err := qe.SubmitMessage(context.Background(), caller,
		"Find Go files, grep for 'searchterm', then read the match")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if result.Turns != 4 {
		t.Errorf("expected 4 turns, got %d", result.Turns)
	}

	// user + 3×(assistant+user) + assistant(final) = 8 messages.
	if len(qe.Messages()) != 8 {
		t.Errorf("expected 8 messages, got %d", len(qe.Messages()))
	}

	// Glob result should mention the Go file.
	globResultMsg := qe.Messages()[2]
	if !strings.Contains(globResultMsg.Content[0].Content, "main.go") {
		t.Errorf("glob result should list main.go, got: %s", globResultMsg.Content[0].Content)
	}

	// Read result should contain the file source.
	readResultMsg := qe.Messages()[6]
	if !strings.Contains(readResultMsg.Content[0].Content, "searchterm") {
		t.Errorf("read result should contain file content, got: %s", readResultMsg.Content[0].Content)
	}
}

// ── Scenario 4: Error recovery — BashTool non-zero exit ───────────────────

// TestE2E_ErrorRecovery_BashNonZeroExit verifies that when BashTool exits with
// a non-zero code the tool result block carries IsError=true and the loop
// continues normally so the agent can respond to the failure.
func TestE2E_ErrorRecovery_BashNonZeroExit(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: run a command that exits with code 1.
			toolUseMsg("Bash", "b1", map[string]any{"command": "exit 1"}),
			// Turn 2: agent acknowledges the failure.
			assistantText("The command failed with exit code 1. I will try a different approach."),
		},
	}

	qe := engine.New(engine.Config{
		Tools:            []tools.Tool{&bash.Tool{}},
		MaxTurns:         10,
		PermissionPrompt: func(_ string) bool { return true }, // E2E: auto-approve
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "Run exit 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	msgs := qe.Messages()
	// user → assistant(bash) → user(tool_result) → assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Tool result must be an error.
	toolResultMsg := msgs[2]
	if len(toolResultMsg.Content) == 0 {
		t.Fatal("expected tool result block")
	}
	if !toolResultMsg.Content[0].IsError {
		t.Errorf("expected IsError=true for non-zero exit, got content: %s",
			toolResultMsg.Content[0].Content)
	}
	if !strings.Contains(toolResultMsg.Content[0].Content, "exit code 1") {
		t.Errorf("tool result should mention exit code 1, got: %s",
			toolResultMsg.Content[0].Content)
	}

	// Loop should reach completion (agent handled the error).
	if !strings.Contains(msgs[3].TextContent(), "failed") {
		t.Errorf("final message should mention failure, got: %s", msgs[3].TextContent())
	}
}

// ── Scenario 5: Session persistence — history accumulates across turns ────

// TestE2E_SessionPersistence calls SubmitMessage twice on the same QueryEngine
// and verifies that messages from both turns are present in the history.
func TestE2E_SessionPersistence(t *testing.T) {
	caller := &mockCaller{
		responses: []*models.Message{
			assistantText("Hello there!"),  // reply to first message
			assistantText("Goodbye then!"), // reply to second message
		},
	}

	qe := engine.New(engine.Config{MaxTurns: 10})

	// First submission.
	if _, err := qe.SubmitMessage(context.Background(), caller, "Hello"); err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	// user + assistant = 2 messages.
	if len(qe.Messages()) != 2 {
		t.Errorf("expected 2 messages after turn 1, got %d", len(qe.Messages()))
	}

	// Second submission.
	if _, err := qe.SubmitMessage(context.Background(), caller, "Goodbye"); err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	// user + assistant + user + assistant = 4 messages.
	if len(qe.Messages()) != 4 {
		t.Errorf("expected 4 messages after turn 2, got %d", len(qe.Messages()))
	}

	msgs := qe.Messages()

	// Turn 1 user message is still present at index 0.
	if !strings.Contains(msgs[0].TextContent(), "Hello") {
		t.Errorf("msgs[0] should contain 'Hello', got: %s", msgs[0].TextContent())
	}
	// Turn 1 assistant reply is at index 1.
	if !strings.Contains(msgs[1].TextContent(), "Hello there") {
		t.Errorf("msgs[1] should contain reply to Hello, got: %s", msgs[1].TextContent())
	}
	// Turn 2 user message is at index 2.
	if !strings.Contains(msgs[2].TextContent(), "Goodbye") {
		t.Errorf("msgs[2] should contain 'Goodbye', got: %s", msgs[2].TextContent())
	}
}

// ── Scenario 6: AskUser flow — mock UserPrompt callback ───────────────────

// TestE2E_AskUserFlow calls RunLoop directly with a ToolContext that carries a
// mock UserPrompt callback. Verifies the callback is invoked with the expected
// questions and the answer appears in the tool result.
func TestE2E_AskUserFlow(t *testing.T) {
	const question = "Which approach do you prefer?"

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: agent asks a question via AskUserQuestionTool.
			toolUseMsg("AskUserQuestion", "ask1", map[string]any{
				"questions": []map[string]any{
					{
						"question": question,
						"header":   "Approach",
						"options": []map[string]any{
							{"label": "Option A", "description": "First approach"},
							{"label": "Option B", "description": "Second approach"},
						},
					},
				},
			}),
			// Turn 2: agent acknowledges the answer.
			assistantText("You chose Option A. Great choice!"),
		},
	}

	var promptCalled bool
	tctx := &tools.ToolContext{
		Tools: []tools.Tool{&askuser.Tool{}},
		UserPrompt: func(questions []tools.AskQuestion) (map[string]string, error) {
			promptCalled = true
			answers := make(map[string]string, len(questions))
			for _, q := range questions {
				// Always pick the first option.
				answers[q.Question] = q.Options[0].Label
			}
			return answers, nil
		},
		// AskUserQuestionTool returns PermAsk; approve it so the tool can execute.
		PermissionPrompt: func(_ string) bool { return true },
	}

	result, msgs, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("Help me decide on an approach")},
		Tools:    []tools.Tool{&askuser.Tool{}},
		Model:    "test-model",
		MaxTurns: 10,
		ToolCtx:  tctx,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if !promptCalled {
		t.Error("expected UserPrompt callback to be invoked")
	}

	// user → assistant(ask) → user(tool_result) → assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Tool result should contain the chosen answer.
	toolResultMsg := msgs[2]
	if !strings.Contains(toolResultMsg.Content[0].Content, "Option A") {
		t.Errorf("tool result should contain answer 'Option A', got: %s",
			toolResultMsg.Content[0].Content)
	}
	if toolResultMsg.Content[0].IsError {
		t.Errorf("tool result should not be an error, got: %s", toolResultMsg.Content[0].Content)
	}
}

// ── Scenario 7: Plan mode — Enter → read-only work → Exit ─────────────────

// TestE2E_PlanMode_EnterAndExit verifies that:
//  1. EnterPlanMode transitions the permission context to ModePlan.
//  2. FileReadTool still executes successfully in plan mode (read-only).
//  3. ExitPlanMode restores the previous mode and writes the plan to disk.
func TestE2E_PlanMode_EnterAndExit(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")

	// A file to read while in plan mode.
	planFile := filepath.Join(tmpDir, "overview.txt")
	mustWriteFile(t, planFile, "project overview\n")

	exitTool := &planmode.ExitTool{PlansDir: plansDir}

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: enter plan mode.
			toolUseMsg("EnterPlanMode", "p1", map[string]any{}),
			// Turn 2: read a file (allowed in plan mode).
			toolUseMsg("Read", "r1", map[string]any{"file_path": planFile}),
			// Turn 3: exit plan mode and save a plan.
			toolUseMsg("ExitPlanMode", "p2", map[string]any{
				"plan": "Step 1: implement X\nStep 2: implement Y",
			}),
			// Turn 4: final text response.
			assistantText("I have reviewed the codebase and created a plan."),
		},
	}

	qe := engine.New(engine.Config{
		Tools:            []tools.Tool{&planmode.EnterTool{}, exitTool, &fileread.Tool{}},
		Cwd:              tmpDir,
		MaxTurns:         10,
		PermissionPrompt: func(_ string) bool { return true }, // auto-approve ExitPlanMode
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "Please enter plan mode and explore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	// After ExitPlanMode, permissions should be restored to the default mode.
	if qe.Permissions().Mode != models.ModeDefault {
		t.Errorf("expected ModeDefault after exit, got %v", qe.Permissions().Mode)
	}

	// ExitPlanMode should have written a plan file to plansDir.
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		t.Fatalf("reading plans dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected a plan file to be written to plansDir")
	}

	// Verify the plan content was written.
	planData, err := os.ReadFile(filepath.Join(plansDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading plan file: %v", err)
	}
	if !strings.Contains(string(planData), "Step 1") {
		t.Errorf("plan file should contain plan text, got: %s", string(planData))
	}

	// Verify all turns executed (8 messages: user + 3*(assistant+user) + assistant).
	if len(qe.Messages()) != 8 {
		t.Errorf("expected 8 messages, got %d", len(qe.Messages()))
	}

	// Read tool result should not be an error (read-only allowed in plan mode).
	readResultMsg := qe.Messages()[4]
	if len(readResultMsg.Content) == 0 {
		t.Fatal("expected read tool result block")
	}
	if readResultMsg.Content[0].IsError {
		t.Errorf("read in plan mode should succeed, got error: %s",
			readResultMsg.Content[0].Content)
	}
}

// ── Scenario 8: Large output — persisted to disk, preview returned ─────────

// TestE2E_LargeOutput_PersistedToDisk creates a file larger than BashTool's
// MaxResultSizeChars threshold and has the agent cat it. Verifies that the
// tool result contains the <persisted-output> wrapper (full content saved to
// disk, preview returned inline).
func TestE2E_LargeOutput_PersistedToDisk(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a file just over the persistence threshold.
	bigContent := strings.Repeat("x", bash.MaxResultSizeChars+100)
	bigFile := filepath.Join(tmpDir, "bigfile.txt")
	mustWriteFile(t, bigFile, bigContent)

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: cat the large file.
			toolUseMsg("Bash", "b1", map[string]any{
				"command": fmt.Sprintf("cat '%s'", bigFile),
			}),
			// Turn 2: agent acknowledges large output was persisted.
			assistantText("The output was large and has been saved to disk."),
		},
	}

	qe := engine.New(engine.Config{
		Tools:    []tools.Tool{&bash.Tool{}},
		Cwd:      tmpDir,
		MaxTurns: 10,
	})

	result, err := qe.SubmitMessage(context.Background(), caller, "Print the big file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	msgs := qe.Messages()
	// user → assistant(bash) → user(tool_result) → assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Tool result must use the persisted-output wrapper.
	toolResultMsg := msgs[2]
	if len(toolResultMsg.Content) == 0 {
		t.Fatal("expected tool result block")
	}
	content := toolResultMsg.Content[0].Content
	if !strings.Contains(content, "persisted-output") {
		t.Errorf("large output should produce <persisted-output> wrapper, got: %.200s", content)
	}
	if !strings.Contains(content, "Full output saved to:") {
		t.Errorf("persisted-output should include file path, got: %.200s", content)
	}
	// Should not be flagged as an error — large output is not a failure.
	if toolResultMsg.Content[0].IsError {
		t.Errorf("large output should not be an error, got: %s", content)
	}
}
