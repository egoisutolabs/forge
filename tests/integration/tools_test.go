// Package integration contains tests that exercise multiple packages together.
// These use real tool implementations with a mock API caller — no live Claude API needed.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/permissions"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/bash"
	"github.com/egoisutolabs/forge/tools/fileedit"
	"github.com/egoisutolabs/forge/tools/fileread"
	"github.com/egoisutolabs/forge/tools/filewrite"
	"github.com/egoisutolabs/forge/tools/glob"
	"github.com/egoisutolabs/forge/tools/grep"
	"github.com/google/uuid"
)

// ============================================================
// Mock helpers (mirror the pattern from engine/loop_test.go)
// ============================================================

// mockCaller replays a sequence of pre-built responses.
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

// capturingCaller records the StreamParams sent on each call.
type capturingCaller struct {
	responses      []*models.Message
	callCount      int
	capturedParams []api.StreamParams
}

func (m *capturingCaller) Stream(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
	m.capturedParams = append(m.capturedParams, params)
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

// assistantText builds a terminal text response (no tool calls).
func assistantText(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopEndTurn,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
	}
}

// assistantWithUsage builds a terminal text response with token usage attached.
func assistantWithUsage(text string, usage models.Usage) *models.Message {
	msg := assistantText(text)
	msg.Usage = &usage
	return msg
}

// assistantWithToolUse builds an assistant message that calls exactly one tool.
func assistantWithToolUse(toolName, toolInput string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopToolUse,
		Content: []models.Block{
			{
				Type:  models.BlockToolUse,
				ID:    "t_" + uuid.NewString()[:8],
				Name:  toolName,
				Input: json.RawMessage(toolInput),
			},
		},
	}
}

// assistantMaxTokens builds a response truncated by the token limit.
func assistantMaxTokens(text string) *models.Message {
	return &models.Message{
		ID:         uuid.NewString(),
		Role:       models.RoleAssistant,
		StopReason: models.StopMaxTokens,
		Content:    []models.Block{{Type: models.BlockText, Text: text}},
		Usage:      &models.Usage{InputTokens: 100, OutputTokens: 8192},
	}
}

// newToolContext builds a ToolContext for integration tests backed by a temp dir.
func newToolContext(t *testing.T, dir string) *tools.ToolContext {
	t.Helper()
	return &tools.ToolContext{
		Cwd:            dir,
		FileState:      tools.NewFileStateCache(100, 25*1024*1024),
		Permissions:    permissions.NewDefaultContext(dir),
		AbortCtx:       context.Background(),
		GlobMaxResults: 100,
		// Integration tests simulate interactive use: auto-approve all PermAsk.
		PermissionPrompt: func(_ string) bool { return true },
	}
}

// toolResultContent returns the Content field of the first tool_result block in a
// conversation, searching from msg index start.
func toolResultContent(messages []*models.Message, start int) string {
	for i := start; i < len(messages); i++ {
		for _, b := range messages[i].Content {
			if b.Type == models.BlockToolResult {
				return b.Content
			}
		}
	}
	return ""
}

// hasToolResultError returns true if any tool_result block in messages (from start)
// has IsError=true.
func hasToolResultError(messages []*models.Message, start int) bool {
	for i := start; i < len(messages); i++ {
		for _, b := range messages[i].Content {
			if b.Type == models.BlockToolResult && b.IsError {
				return true
			}
		}
	}
	return false
}

// ============================================================
// 1. Engine calls BashTool via mock API
// ============================================================

func TestIntegration_BashTool_ViaLoop(t *testing.T) {
	// The mock API drives one tool call (Bash) then closes the loop.
	// We use a real bash.Tool so the command actually executes.
	dir := t.TempDir()
	tctx := newToolContext(t, dir)

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("Bash", `{"command":"echo integration_test_output"}`),
			assistantText("Done."),
		},
	}

	result, messages, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("run a command")},
		Tools:    []tools.Tool{&bash.Tool{}},
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
	// Verify the Bash output ended up in the conversation.
	output := toolResultContent(messages, 0)
	if !strings.Contains(output, "integration_test_output") {
		t.Errorf("expected 'integration_test_output' in tool result, got: %q", output)
	}
}

// ============================================================
// 2. FileRead → FileEdit cache pipeline
// ============================================================

func TestIntegration_FileRead_FileEdit_CachePipeline(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tctx := newToolContext(t, dir)

	readInput := fmt.Sprintf(`{"file_path":%q}`, filePath)
	editInput := fmt.Sprintf(`{"file_path":%q,"old_string":"hello","new_string":"goodbye"}`, filePath)

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("Read", readInput),
			assistantWithToolUse("Edit", editInput),
			assistantText("File updated."),
		},
	}

	result, messages, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("update the file")},
		Tools:    []tools.Tool{&fileread.Tool{}, &fileedit.Tool{}},
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
	if hasToolResultError(messages, 0) {
		t.Error("unexpected tool error in conversation")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "goodbye") {
		t.Errorf("expected 'goodbye' in file, got: %s", content)
	}
}

// ============================================================
// 3. FileEdit without prior Read → cache guard returns error
// ============================================================

func TestIntegration_FileEdit_NoCacheEntry_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(filePath, []byte("original\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tctx := newToolContext(t, dir) // fresh cache — file has NOT been read

	editBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "t1",
		Name:  "Edit",
		Input: json.RawMessage(fmt.Sprintf(`{"file_path":%q,"old_string":"original","new_string":"replaced"}`, filePath)),
	}

	results := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{editBlock},
		[]tools.Tool{&fileedit.Tool{}},
		tctx,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error from Edit without prior Read")
	}
	if !strings.Contains(results[0].Content, "read") {
		t.Errorf("expected cache-guard message, got: %s", results[0].Content)
	}
}

// ============================================================
// 4. FileRead → FileWrite cache pipeline
// ============================================================

func TestIntegration_FileRead_FileWrite_CachePipeline(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(filePath, []byte("original content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tctx := newToolContext(t, dir)

	readInput := fmt.Sprintf(`{"file_path":%q}`, filePath)
	writeInput := fmt.Sprintf(`{"file_path":%q,"content":"new content\n"}`, filePath)

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithToolUse("Read", readInput),
			assistantWithToolUse("Write", writeInput),
			assistantText("Done."),
		},
	}

	result, messages, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("overwrite the file")},
		Tools:    []tools.Tool{&fileread.Tool{}, &filewrite.Tool{}},
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
	if hasToolResultError(messages, 0) {
		t.Error("unexpected tool error in conversation")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != "new content" {
		t.Errorf("expected 'new content', got: %q", string(content))
	}
}

// ============================================================
// 5. FileWrite creates a new file without requiring prior Read
// ============================================================

func TestIntegration_FileWrite_NewFile_NoPriorRead(t *testing.T) {
	dir := t.TempDir()
	newPath := filepath.Join(dir, "brand_new.txt")

	tctx := newToolContext(t, dir) // file doesn't exist yet → no Read required

	writeBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "w1",
		Name:  "Write",
		Input: json.RawMessage(fmt.Sprintf(`{"file_path":%q,"content":"created!\n"}`, newPath)),
	}

	results := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{writeBlock},
		[]tools.Tool{&filewrite.Tool{}},
		tctx,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].IsError {
		t.Errorf("unexpected error: %s", results[0].Content)
	}

	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("new file was not created: %v", err)
	}
	if !strings.Contains(string(content), "created!") {
		t.Errorf("unexpected content: %s", content)
	}
}

// ============================================================
// 6. Glob finds files → Grep searches their content
// ============================================================

func TestIntegration_Glob_Grep_Pipeline(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep (rg) not installed — skipping grep integration test")
	}

	dir := t.TempDir()
	files := map[string]string{
		"alpha.go":  "package main\n// FINDME marker alpha\n",
		"beta.go":   "package main\n// FINDME marker beta\n",
		"gamma.txt": "not go code\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	tctx := newToolContext(t, dir)

	// Step 1: Glob for *.go files
	globBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "g1",
		Name:  "Glob",
		Input: json.RawMessage(`{"pattern":"*.go"}`),
	}
	globResults := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{globBlock},
		[]tools.Tool{&glob.Tool{}},
		tctx,
	)
	if len(globResults) != 1 || globResults[0].IsError {
		t.Fatalf("Glob failed: %+v", globResults)
	}
	if !strings.Contains(globResults[0].Content, "alpha.go") || !strings.Contains(globResults[0].Content, "beta.go") {
		t.Errorf("Glob should find .go files, got: %s", globResults[0].Content)
	}
	if strings.Contains(globResults[0].Content, "gamma.txt") {
		t.Error("Glob should not include .txt files")
	}

	// Step 2: Grep for FINDME in the directory
	grepBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "r1",
		Name:  "Grep",
		Input: json.RawMessage(fmt.Sprintf(`{"pattern":"FINDME","path":%q}`, dir)),
	}
	grepResults := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{grepBlock},
		[]tools.Tool{&grep.Tool{}},
		tctx,
	)
	if len(grepResults) != 1 || grepResults[0].IsError {
		t.Fatalf("Grep failed: %+v", grepResults)
	}
	if !strings.Contains(grepResults[0].Content, "alpha.go") || !strings.Contains(grepResults[0].Content, "beta.go") {
		t.Errorf("Grep should find FINDME in .go files, got: %s", grepResults[0].Content)
	}
	if strings.Contains(grepResults[0].Content, "gamma.txt") {
		t.Error("Grep should not return gamma.txt (no FINDME)")
	}
}

// ============================================================
// 7. PreToolUse hook denies BashTool → error returned
// ============================================================

func TestIntegration_PreToolUseHook_DeniesExecution(t *testing.T) {
	dir := t.TempDir()
	tctx := newToolContext(t, dir)

	// Hook outputs a JSON deny decision.
	denyHook := hooks.HookConfig{
		Command: `printf '{"continue":false,"decision":"deny","reason":"blocked by policy"}'`,
	}
	tctx.Hooks = hooks.HooksSettings{
		hooks.HookEventPreToolUse: []hooks.HookMatcher{
			{Matcher: "Bash", Hooks: []hooks.HookConfig{denyHook}},
		},
	}

	bashBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "b1",
		Name:  "Bash",
		Input: json.RawMessage(`{"command":"echo should_not_run"}`),
	}

	results := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{bashBlock},
		[]tools.Tool{&bash.Tool{}},
		tctx,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error from hook denial")
	}
	if !strings.Contains(results[0].Content, "Hook denied") {
		t.Errorf("expected 'Hook denied' in result, got: %s", results[0].Content)
	}
	if !strings.Contains(results[0].Content, "blocked by policy") {
		t.Errorf("expected denial reason in result, got: %s", results[0].Content)
	}
}

// ============================================================
// 8. PostToolUse hook is invoked after execution
// ============================================================

func TestIntegration_PostToolUseHook_IsCalled(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(dir, "hook_was_called")

	tctx := newToolContext(t, dir)
	postHook := hooks.HookConfig{
		// Write a file to prove the hook ran.
		Command: fmt.Sprintf("touch %s", markerPath),
	}
	tctx.Hooks = hooks.HooksSettings{
		hooks.HookEventPostToolUse: []hooks.HookMatcher{
			{Matcher: "Bash", Hooks: []hooks.HookConfig{postHook}},
		},
	}

	bashBlock := models.Block{
		Type:  models.BlockToolUse,
		ID:    "b1",
		Name:  "Bash",
		Input: json.RawMessage(`{"command":"echo hello_post_hook"}`),
	}

	results := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{bashBlock},
		[]tools.Tool{&bash.Tool{}},
		tctx,
	)

	if len(results) != 1 || results[0].IsError {
		t.Fatalf("unexpected tool error: %+v", results)
	}
	if !strings.Contains(results[0].Content, "hello_post_hook") {
		t.Errorf("expected bash output, got: %s", results[0].Content)
	}

	// Give the hook a moment to complete (it runs asynchronously via best-effort).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(markerPath); err == nil {
			return // marker exists — hook was called
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Error("PostToolUse hook was not called (marker file not created)")
}

// ============================================================
// 9. Permission flow: read-only tools auto-approved
// ============================================================

func TestIntegration_Permission_ReadOnly_AutoApproved(t *testing.T) {
	dir := t.TempDir()
	tctx := newToolContext(t, dir) // default mode

	// Glob and Read are read-only — their CheckPermissions should return PermAllow.
	for _, tool := range []tools.Tool{&glob.Tool{}, &fileread.Tool{}} {
		dec, err := tool.CheckPermissions(json.RawMessage(`{"pattern":"*"}`), tctx)
		if err != nil {
			t.Errorf("%s.CheckPermissions error: %v", tool.Name(), err)
			continue
		}
		if dec.Behavior != models.PermAllow {
			t.Errorf("%s: expected PermAllow for read-only tool, got %v", tool.Name(), dec.Behavior)
		}
		if !tool.IsReadOnly(nil) {
			t.Errorf("%s: expected IsReadOnly=true", tool.Name())
		}
	}
}

// ============================================================
// 10. Permission flow: write tools return PermAsk
// ============================================================

func TestIntegration_Permission_WriteTools_ReturnPermAsk(t *testing.T) {
	dir := t.TempDir()
	tctx := newToolContext(t, dir) // default mode

	// FileWrite and FileEdit are not read-only and return PermAsk in default mode.
	for _, tc := range []struct {
		tool  tools.Tool
		input string
	}{
		{&filewrite.Tool{}, fmt.Sprintf(`{"file_path":%q,"content":"x"}`, filepath.Join(dir, "f.txt"))},
		{&fileedit.Tool{}, fmt.Sprintf(`{"file_path":%q,"old_string":"a","new_string":"b"}`, filepath.Join(dir, "f.txt"))},
	} {
		dec, err := tc.tool.CheckPermissions(json.RawMessage(tc.input), tctx)
		if err != nil {
			t.Errorf("%s.CheckPermissions error: %v", tc.tool.Name(), err)
			continue
		}
		if dec.Behavior != models.PermAsk {
			t.Errorf("%s: expected PermAsk for write tool, got %v", tc.tool.Name(), dec.Behavior)
		}
		if tc.tool.IsReadOnly(nil) {
			t.Errorf("%s: expected IsReadOnly=false", tc.tool.Name())
		}
	}
}

// ============================================================
// 11. Permission flow: PermDeny stops execution
// ============================================================

func TestIntegration_Permission_PermDeny_BlocksExecution(t *testing.T) {
	tctx := &tools.ToolContext{
		Cwd:         t.TempDir(),
		FileState:   tools.NewFileStateCache(10, 1024),
		Permissions: permissions.NewDefaultContext(t.TempDir()),
		AbortCtx:    context.Background(),
	}

	// A tool that always returns PermDeny.
	denyTool := &alwaysDenyTool{}

	results := tools.ExecuteToolBlocks(
		context.Background(),
		[]models.Block{
			{Type: models.BlockToolUse, ID: "d1", Name: "Deny", Input: json.RawMessage(`{}`)},
		},
		[]tools.Tool{denyTool},
		tctx,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error block for PermDeny tool")
	}
	if !strings.Contains(results[0].Content, "Permission denied") {
		t.Errorf("expected 'Permission denied' in result, got: %s", results[0].Content)
	}
}

// alwaysDenyTool is a minimal tool that always returns PermDeny.
type alwaysDenyTool struct{}

func (d *alwaysDenyTool) Name() string        { return "Deny" }
func (d *alwaysDenyTool) Description() string { return "always denied" }
func (d *alwaysDenyTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (d *alwaysDenyTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "should not reach here"}, nil
}
func (d *alwaysDenyTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermDeny, Message: "always denied"}, nil
}
func (d *alwaysDenyTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (d *alwaysDenyTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (d *alwaysDenyTool) IsReadOnly(_ json.RawMessage) bool        { return false }

// ============================================================
// 12. StreamingExecutor: concurrent tools run in parallel
// ============================================================

func TestIntegration_StreamingExecutor_ConcurrentTools_Parallel(t *testing.T) {
	var inFlight atomic.Int32
	var maxInFlight atomic.Int32

	slowTool := &slowConcTool{
		delay: 40 * time.Millisecond,
		fn: func() string {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := maxInFlight.Load()
				if cur > old {
					if maxInFlight.CompareAndSwap(old, cur) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(40 * time.Millisecond)
			return "ok"
		},
	}

	caller := &mockCaller{
		responses: []*models.Message{
			{
				ID:         uuid.NewString(),
				Role:       models.RoleAssistant,
				StopReason: models.StopToolUse,
				Content: []models.Block{
					{Type: models.BlockToolUse, ID: "t1", Name: "Slow", Input: json.RawMessage(`{}`)},
					{Type: models.BlockToolUse, ID: "t2", Name: "Slow", Input: json.RawMessage(`{}`)},
					{Type: models.BlockToolUse, ID: "t3", Name: "Slow", Input: json.RawMessage(`{}`)},
				},
			},
			assistantText("all done"),
		},
	}

	result, _, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("go parallel")},
		Tools:    []tools.Tool{slowTool},
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}
	if maxInFlight.Load() < 2 {
		t.Errorf("expected concurrent execution (max in-flight >= 2), got %d", maxInFlight.Load())
	}
}

// ============================================================
// 13. StreamingExecutor: non-concurrent tools run serially
// ============================================================

func TestIntegration_StreamingExecutor_NonConcurrentTools_Serial(t *testing.T) {
	var inFlight atomic.Int32
	var maxObserved atomic.Int32

	serialTool := &serialOnlyTool{
		fn: func() string {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := maxObserved.Load()
				if cur > old {
					if maxObserved.CompareAndSwap(old, cur) {
						break
					}
				} else {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			return "serial"
		},
	}

	// Two serial tools — must not overlap.
	results := make(chan models.Block, 4)
	ctx := context.Background()
	se := tools.NewStreamingExecutor(ctx, []tools.Tool{serialTool}, nil)
	se.AddTool(models.Block{Type: models.BlockToolUse, ID: "s1", Name: "Serial", Input: json.RawMessage(`{}`)})
	se.AddTool(models.Block{Type: models.BlockToolUse, ID: "s2", Name: "Serial", Input: json.RawMessage(`{}`)})
	se.Done()

	for r := range se.Results() {
		results <- r
	}
	close(results)

	if maxObserved.Load() > 1 {
		t.Errorf("serial tool should never have >1 in-flight, got %d", maxObserved.Load())
	}

	var got []models.Block
	for b := range results {
		got = append(got, b)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %d", len(got))
	}
}

// ============================================================
// 14. Budget enforcement: loop stops when MaxBudgetUSD exceeded
// ============================================================

func TestIntegration_BudgetEnforcement_StopsAtLimit(t *testing.T) {
	// $0.001 budget; response has 1M input tokens → cost ≈ $3 (Sonnet tier).
	budget := 0.001

	caller := &mockCaller{
		responses: []*models.Message{
			assistantWithUsage("result", models.Usage{
				InputTokens:  1_000_000,
				OutputTokens: 0,
			}),
		},
	}

	result, _, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:       caller,
		Messages:     []*models.Message{models.NewUserMessage("do something")},
		Tools:        nil,
		Model:        "claude-sonnet-4-6",
		MaxTurns:     20,
		MaxBudgetUSD: &budget,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopBudgetExceeded {
		t.Errorf("expected budget_exceeded, got %v", result.Reason)
	}
	if result.TotalCostUSD < budget {
		t.Errorf("expected cost >= budget (%.6f), got %.6f", budget, result.TotalCostUSD)
	}
}

// ============================================================
// 15. Max output tokens: first hit escalates token limit, then recovers
// ============================================================

func TestIntegration_MaxOutputTokens_Escalation_Recovery(t *testing.T) {
	caller := &capturingCaller{
		responses: []*models.Message{
			assistantMaxTokens("truncated..."),
			assistantText("full response after escalation"),
		},
	}

	result, _, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("long task")},
		Tools:    nil,
		Model:    "test-model",
		MaxTurns: 10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed after escalation, got %v", result.Reason)
	}
	if len(caller.capturedParams) < 2 {
		t.Fatalf("expected at least 2 API calls, got %d", len(caller.capturedParams))
	}
	// First call uses default 8192 tokens.
	if caller.capturedParams[0].MaxTokens != 8192 {
		t.Errorf("first call: expected MaxTokens=8192, got %d", caller.capturedParams[0].MaxTokens)
	}
	// Second call (after escalation) uses 64000.
	if caller.capturedParams[1].MaxTokens != 64000 {
		t.Errorf("second call: expected MaxTokens=64000 (escalated), got %d", caller.capturedParams[1].MaxTokens)
	}
	// Escalation should not count as a full turn.
	if result.Turns != 1 {
		t.Errorf("expected 1 real turn (escalation is free), got %d", result.Turns)
	}
}

// ============================================================
// 16. Max output tokens: too many retries → StopOutputTruncated
// ============================================================

func TestIntegration_MaxOutputTokens_TooManyRetries(t *testing.T) {
	// 5 consecutive max_tokens hits → output_truncated
	responses := make([]*models.Message, 5)
	for i := range responses {
		responses[i] = assistantMaxTokens("cut")
	}

	caller := &mockCaller{responses: responses}

	result, _, err := engine.RunLoop(context.Background(), engine.LoopParams{
		Caller:   caller,
		Messages: []*models.Message{models.NewUserMessage("hi")},
		Model:    "test-model",
		MaxTurns: 20,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reason != models.StopOutputTruncated {
		t.Errorf("expected output_truncated, got %v", result.Reason)
	}
}

// ============================================================
// Test tool helpers
// ============================================================

// slowConcTool is a concurrent-safe tool that runs a custom function.
type slowConcTool struct {
	delay time.Duration
	fn    func() string
}

func (s *slowConcTool) Name() string        { return "Slow" }
func (s *slowConcTool) Description() string { return "slow concurrent tool" }
func (s *slowConcTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (s *slowConcTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: s.fn()}, nil
}
func (s *slowConcTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *slowConcTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (s *slowConcTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *slowConcTool) IsReadOnly(_ json.RawMessage) bool        { return true }

// serialOnlyTool is a non-concurrent-safe tool that runs a custom function.
type serialOnlyTool struct {
	fn func() string
}

func (s *serialOnlyTool) Name() string        { return "Serial" }
func (s *serialOnlyTool) Description() string { return "serial tool" }
func (s *serialOnlyTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (s *serialOnlyTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: s.fn()}, nil
}
func (s *serialOnlyTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *serialOnlyTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (s *serialOnlyTool) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (s *serialOnlyTool) IsReadOnly(_ json.RawMessage) bool        { return false }
