package e2e

import (
	"context"
	"testing"

	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/services/session"
	"github.com/egoisutolabs/forge/tools"
	"github.com/egoisutolabs/forge/tools/fileread"
)

// TestSession_SaveAndLoad creates a session, saves it to a temp directory,
// loads it back, and verifies messages are preserved.
func TestSession_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	s := session.New("claude-sonnet-4-6", "/tmp/test")
	s.Messages = []*models.Message{
		models.NewUserMessage("hello"),
	}
	s.TotalUsage = models.Usage{InputTokens: 100, OutputTokens: 50}

	if err := session.Save(s, dir); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := session.Load(s.ID, dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.ID != s.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want \"claude-sonnet-4-6\"", loaded.Model)
	}
	if len(loaded.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].TextContent() != "hello" {
		t.Errorf("message content = %q, want \"hello\"", loaded.Messages[0].TextContent())
	}
	if loaded.TotalUsage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", loaded.TotalUsage.InputTokens)
	}
}

// TestSession_AutoSave verifies AutoSave updates messages and usage.
func TestSession_AutoSave(t *testing.T) {
	dir := t.TempDir()

	s := session.New("test-model", "/tmp/work")

	msgs := []*models.Message{
		models.NewUserMessage("turn 1"),
		{Role: models.RoleAssistant, Content: []models.Block{{Type: models.BlockText, Text: "reply 1"}}},
		models.NewUserMessage("turn 2"),
	}
	usage := models.Usage{InputTokens: 200, OutputTokens: 100}

	if err := session.AutoSave(s, msgs, usage, dir); err != nil {
		t.Fatalf("AutoSave error: %v", err)
	}

	loaded, err := session.Load(s.ID, dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(loaded.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded.Messages))
	}
	if loaded.TotalUsage.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", loaded.TotalUsage.OutputTokens)
	}
}

// TestSession_List verifies List returns sessions sorted by most recent first.
func TestSession_List(t *testing.T) {
	dir := t.TempDir()

	s1 := session.New("model-a", "/a")
	s1.Messages = []*models.Message{models.NewUserMessage("first")}
	if err := session.Save(s1, dir); err != nil {
		t.Fatal(err)
	}

	s2 := session.New("model-b", "/b")
	s2.Messages = []*models.Message{models.NewUserMessage("second")}
	if err := session.Save(s2, dir); err != nil {
		t.Fatal(err)
	}

	sessions, err := session.List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Most recent (s2) should be first.
	if sessions[0].ID != s2.ID {
		t.Errorf("expected most recent session first, got ID %q (want %q)", sessions[0].ID, s2.ID)
	}
}

// TestSession_ListEmpty verifies List returns nil for an empty directory.
func TestSession_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	sessions, err := session.List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

// TestSession_EngineWithMockCaller exercises the engine with a mock caller,
// verifying session-like message accumulation across two SubmitMessage calls.
func TestSession_EngineWithMockCaller(t *testing.T) {
	tmpDir := t.TempDir()
	mustWriteFile(t, tmpDir+"/test.txt", "session test content\n")

	caller := &mockCaller{
		responses: []*models.Message{
			// Turn 1: read a file.
			toolUseMsg("Read", "r1", map[string]any{"file_path": tmpDir + "/test.txt"}),
			// Turn 1 continues: text response.
			assistantText("I read the file."),
			// Turn 2: another text response.
			assistantText("Anything else?"),
		},
	}

	qe := engine.New(engine.Config{
		Tools:    []tools.Tool{&fileread.Tool{}},
		Cwd:      tmpDir,
		MaxTurns: 10,
	})

	// Turn 1.
	result, err := qe.SubmitMessage(context.Background(), caller, "Read the file")
	if err != nil {
		t.Fatalf("turn 1 error: %v", err)
	}
	if result.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result.Reason)
	}

	msgsAfterTurn1 := len(qe.Messages())
	if msgsAfterTurn1 < 2 {
		t.Fatalf("expected at least 2 messages after turn 1, got %d", msgsAfterTurn1)
	}

	// Turn 2.
	result2, err := qe.SubmitMessage(context.Background(), caller, "What else?")
	if err != nil {
		t.Fatalf("turn 2 error: %v", err)
	}
	if result2.Reason != models.StopCompleted {
		t.Errorf("expected completed, got %v", result2.Reason)
	}

	msgsAfterTurn2 := len(qe.Messages())
	if msgsAfterTurn2 <= msgsAfterTurn1 {
		t.Errorf("messages should grow across turns: after turn1=%d, after turn2=%d",
			msgsAfterTurn1, msgsAfterTurn2)
	}

	// Verify first user message is preserved.
	first := qe.Messages()[0]
	if first.TextContent() != "Read the file" {
		t.Errorf("first message should be \"Read the file\", got %q", first.TextContent())
	}
}
