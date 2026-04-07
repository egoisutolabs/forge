package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ── BuildForkDirective ─────────────────────────────────────────────────────────

func TestBuildForkDirective_ContainsBoilerplateTag(t *testing.T) {
	msg := BuildForkDirective("do the thing")
	if msg == nil {
		t.Fatal("BuildForkDirective returned nil")
	}
	if msg.Role != models.RoleUser {
		t.Errorf("role = %q, want %q", msg.Role, models.RoleUser)
	}

	text := msg.TextContent()
	openTag := "<" + ForkBoilerplateTag + ">"
	closeTag := "</" + ForkBoilerplateTag + ">"
	if !strings.Contains(text, openTag) {
		t.Errorf("directive missing opening tag %q", openTag)
	}
	if !strings.Contains(text, closeTag) {
		t.Errorf("directive missing closing tag %q", closeTag)
	}
}

func TestBuildForkDirective_ContainsDirective(t *testing.T) {
	prompt := "analyse all the things"
	msg := BuildForkDirective(prompt)
	text := msg.TextContent()
	if !strings.Contains(text, prompt) {
		t.Errorf("directive text does not contain prompt %q", prompt)
	}
}

func TestBuildForkDirective_ContainsDirectivePrefix(t *testing.T) {
	msg := BuildForkDirective("some task")
	text := msg.TextContent()
	if !strings.Contains(text, ForkDirectivePrefix) {
		t.Errorf("directive text missing prefix %q", ForkDirectivePrefix)
	}
}

func TestBuildForkDirective_ContainsNoSubagentsRule(t *testing.T) {
	msg := BuildForkDirective("work")
	text := msg.TextContent()
	// Rule 1 must tell the child not to spawn sub-agents.
	if !strings.Contains(text, "sub-agents") {
		t.Error("fork directive should mention 'sub-agents' in the rules")
	}
}

// ── IsForkChild ────────────────────────────────────────────────────────────────

func TestIsForkChild_EmptyMessages_ReturnsFalse(t *testing.T) {
	if IsForkChild(nil) {
		t.Error("IsForkChild(nil) should return false")
	}
	if IsForkChild([]*models.Message{}) {
		t.Error("IsForkChild([]) should return false")
	}
}

func TestIsForkChild_MessagesWithoutBoilerplate_ReturnsFalse(t *testing.T) {
	msgs := []*models.Message{
		models.NewUserMessage("ordinary user message"),
		{
			Role: models.RoleAssistant,
			Content: []models.Block{
				{Type: models.BlockText, Text: "assistant reply"},
			},
		},
	}
	if IsForkChild(msgs) {
		t.Error("messages without boilerplate should not be detected as fork child")
	}
}

func TestIsForkChild_MessageWithBoilerplateTag_ReturnsTrue(t *testing.T) {
	directive := BuildForkDirective("my task")
	msgs := []*models.Message{
		models.NewUserMessage("initial context"),
		directive,
	}
	if !IsForkChild(msgs) {
		t.Error("messages containing fork directive should be detected as fork child")
	}
}

func TestIsForkChild_BoilerplateInAssistantMessage_ReturnsFalse(t *testing.T) {
	// Boilerplate only counts when it's in a user message.
	tag := "<" + ForkBoilerplateTag + ">some content</" + ForkBoilerplateTag + ">"
	msgs := []*models.Message{
		{
			Role:    models.RoleAssistant,
			Content: []models.Block{{Type: models.BlockText, Text: tag}},
		},
	}
	if IsForkChild(msgs) {
		t.Error("boilerplate in assistant message should not trigger IsForkChild")
	}
}

// ── StartFork ─────────────────────────────────────────────────────────────────

func TestStartFork_RecursionPrevented(t *testing.T) {
	// Build a message history that already contains fork boilerplate.
	directive := BuildForkDirective("prior task")
	msgs := []*models.Message{
		models.NewUserMessage("context"),
		directive,
	}

	_, err := StartFork(context.Background(), msgs, "sys", nil, nil)
	if err == nil {
		t.Error("StartFork inside a fork child should return an error")
	}
	if !strings.Contains(err.Error(), "recursive fork") {
		t.Errorf("error should mention 'recursive fork', got: %v", err)
	}
}

func TestStartFork_ReturnsAgentIDImmediately(t *testing.T) {
	origRunner := AgentLoopRunner
	defer func() { AgentLoopRunner = origRunner }()

	// Slow runner — StartFork must return before this completes.
	started := make(chan struct{})
	AgentLoopRunner = func(_ context.Context, _ api.Caller, _ []*models.Message, _ string, _ []tools.Tool) {
		close(started)
		time.Sleep(10 * time.Second) // should never block the caller
	}

	done := make(chan string, 1)
	go func() {
		id, err := StartFork(context.Background(), nil, "sys", nil, nil)
		if err != nil {
			t.Errorf("StartFork: %v", err)
		}
		done <- id
	}()

	select {
	case id := <-done:
		if id == "" {
			t.Error("StartFork should return a non-empty agentID")
		}
	case <-time.After(time.Second):
		t.Fatal("StartFork did not return within 1 second — it must not block on the loop")
	}

	// Goroutine should be running.
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background goroutine did not start within 1 second")
	}
}

func TestStartFork_BoilerplateInjectedInChildMessages(t *testing.T) {
	origRunner := AgentLoopRunner
	defer func() { AgentLoopRunner = origRunner }()

	captured := make(chan []*models.Message, 1)
	AgentLoopRunner = func(_ context.Context, _ api.Caller, msgs []*models.Message, _ string, _ []tools.Tool) {
		cp := make([]*models.Message, len(msgs))
		copy(cp, msgs)
		captured <- cp
	}

	parentMsgs := []*models.Message{
		models.NewUserMessage("hello from parent"),
	}

	_, err := StartFork(context.Background(), parentMsgs, "sys", nil, nil)
	if err != nil {
		t.Fatalf("StartFork: %v", err)
	}

	var childMsgs []*models.Message
	select {
	case childMsgs = <-captured:
	case <-time.After(time.Second):
		t.Fatal("loop runner was not called within 1 second")
	}

	// Child messages must include a message with the boilerplate tag.
	if !IsForkChild(childMsgs) {
		t.Error("child messages should contain fork boilerplate (IsForkChild should be true)")
	}
}

func TestStartFork_InheritsParentMessages(t *testing.T) {
	origRunner := AgentLoopRunner
	defer func() { AgentLoopRunner = origRunner }()

	captured := make(chan []*models.Message, 1)
	AgentLoopRunner = func(_ context.Context, _ api.Caller, msgs []*models.Message, _ string, _ []tools.Tool) {
		cp := make([]*models.Message, len(msgs))
		copy(cp, msgs)
		captured <- cp
	}

	parent1 := models.NewUserMessage("first parent turn")
	parent2 := models.NewUserMessage("second parent turn")

	_, err := StartFork(context.Background(), []*models.Message{parent1, parent2}, "sys", nil, nil)
	if err != nil {
		t.Fatalf("StartFork: %v", err)
	}

	var childMsgs []*models.Message
	select {
	case childMsgs = <-captured:
	case <-time.After(time.Second):
		t.Fatal("loop runner was not called within 1 second")
	}

	// The first two messages should be the parent's verbatim.
	if len(childMsgs) < 2 {
		t.Fatalf("expected at least 2 messages (parent + directive), got %d", len(childMsgs))
	}
	if childMsgs[0].ID != parent1.ID {
		t.Errorf("first child message ID = %q, want parent's %q", childMsgs[0].ID, parent1.ID)
	}
	if childMsgs[1].ID != parent2.ID {
		t.Errorf("second child message ID = %q, want parent's %q", childMsgs[1].ID, parent2.ID)
	}
}

func TestStartFork_UniqueAgentIDEachCall(t *testing.T) {
	origRunner := AgentLoopRunner
	defer func() { AgentLoopRunner = origRunner }()
	AgentLoopRunner = func(_ context.Context, _ api.Caller, _ []*models.Message, _ string, _ []tools.Tool) {}

	id1, err := StartFork(context.Background(), nil, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := StartFork(context.Background(), nil, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if id1 == id2 {
		t.Error("consecutive StartFork calls should return distinct agentIDs")
	}
	if id1 == "" || id2 == "" {
		t.Error("agentIDs must not be empty")
	}
}
