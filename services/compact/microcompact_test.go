package compact

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/google/uuid"
)

func toolResultMsg(toolUseID, content string) *models.Message {
	return &models.Message{
		ID:   uuid.NewString(),
		Role: models.RoleUser,
		Content: []models.Block{
			models.NewToolResultBlock(toolUseID, content, false),
		},
	}
}

func assistantToolUseMsg(toolUseID string) *models.Message {
	return &models.Message{
		ID:   uuid.NewString(),
		Role: models.RoleAssistant,
		Content: []models.Block{
			{Type: models.BlockToolUse, ID: toolUseID, Name: "bash"},
		},
	}
}

func TestMicroCompact_EmptyMessages(t *testing.T) {
	r := MicroCompact(nil, 5)
	if len(r.Messages) != 0 {
		t.Errorf("expected nil/empty, got %d messages", len(r.Messages))
	}
	if r.TokensSaved != 0 {
		t.Errorf("TokensSaved = %d, want 0", r.TokensSaved)
	}
}

func TestMicroCompact_FewerThanKeepRecent(t *testing.T) {
	msgs := []*models.Message{
		userMsg("hello"),
		assistantToolUseMsg("t1"),
		toolResultMsg("t1", "output1"),
		assistantToolUseMsg("t2"),
		toolResultMsg("t2", "output2"),
	}
	r := MicroCompact(msgs, 5)
	// Only 2 tool results, keepRecent=5 → nothing cleared.
	if r.TokensSaved != 0 {
		t.Errorf("TokensSaved = %d, want 0", r.TokensSaved)
	}
	// Messages should be the same slice (no copy needed).
	if &r.Messages[0] != &msgs[0] {
		t.Error("expected same slice when nothing cleared")
	}
}

func TestMicroCompact_ClearsOldToolResults(t *testing.T) {
	msgs := []*models.Message{
		userMsg("start"),
		assistantToolUseMsg("t1"),
		toolResultMsg("t1", "old-output-from-tool-1-with-lots-of-content"),
		assistantToolUseMsg("t2"),
		toolResultMsg("t2", "old-output-from-tool-2-with-lots-of-content"),
		assistantToolUseMsg("t3"),
		toolResultMsg("t3", "recent-output-3"),
		assistantToolUseMsg("t4"),
		toolResultMsg("t4", "recent-output-4"),
	}

	r := MicroCompact(msgs, 2)

	// First two tool results (t1, t2) should be cleared.
	tr1 := r.Messages[2].Content[0]
	if !strings.Contains(tr1.Content, "[tool result cleared") {
		t.Errorf("t1 result should be cleared, got: %q", tr1.Content)
	}
	tr2 := r.Messages[4].Content[0]
	if !strings.Contains(tr2.Content, "[tool result cleared") {
		t.Errorf("t2 result should be cleared, got: %q", tr2.Content)
	}

	// Last two (t3, t4) should be untouched.
	tr3 := r.Messages[6].Content[0]
	if tr3.Content != "recent-output-3" {
		t.Errorf("t3 result should be intact, got: %q", tr3.Content)
	}
	tr4 := r.Messages[8].Content[0]
	if tr4.Content != "recent-output-4" {
		t.Errorf("t4 result should be intact, got: %q", tr4.Content)
	}

	if r.TokensSaved <= 0 {
		t.Error("expected positive TokensSaved")
	}
}

func TestMicroCompact_DoesNotMutateOriginal(t *testing.T) {
	original := "original-content-that-should-not-change"
	msgs := []*models.Message{
		assistantToolUseMsg("t1"),
		toolResultMsg("t1", original),
		assistantToolUseMsg("t2"),
		toolResultMsg("t2", "recent"),
	}

	r := MicroCompact(msgs, 1)

	// Original message should be unchanged.
	if msgs[1].Content[0].Content != original {
		t.Errorf("original mutated: got %q", msgs[1].Content[0].Content)
	}

	// Result should have the cleared version.
	if !strings.Contains(r.Messages[1].Content[0].Content, "[tool result cleared") {
		t.Errorf("result should be cleared, got: %q", r.Messages[1].Content[0].Content)
	}
}

func TestMicroCompact_DefaultKeepRecent(t *testing.T) {
	// With keepRecent=0, should use DefaultKeepRecent (5).
	var msgs []*models.Message
	for i := 0; i < 7; i++ {
		id := string(rune('a'+i)) + "1"
		msgs = append(msgs, assistantToolUseMsg(id))
		msgs = append(msgs, toolResultMsg(id, "content-for-"+id))
	}

	r := MicroCompact(msgs, 0)

	// 7 tool results, keep 5 → clear first 2.
	cleared := 0
	for _, msg := range r.Messages {
		for _, b := range msg.Content {
			if b.Type == models.BlockToolResult && strings.Contains(b.Content, "[tool result cleared") {
				cleared++
			}
		}
	}
	if cleared != 2 {
		t.Errorf("expected 2 cleared results, got %d", cleared)
	}
}

func TestMicroCompact_ClearedPlaceholderIncludesByteCount(t *testing.T) {
	content := strings.Repeat("x", 1000)
	msgs := []*models.Message{
		assistantToolUseMsg("t1"),
		toolResultMsg("t1", content),
		assistantToolUseMsg("t2"),
		toolResultMsg("t2", "keep"),
	}

	r := MicroCompact(msgs, 1)
	placeholder := r.Messages[1].Content[0].Content
	if !strings.Contains(placeholder, "1000 bytes") {
		t.Errorf("placeholder should include byte count, got: %q", placeholder)
	}
}

func TestMicroCompact_SkipsEmptyToolResults(t *testing.T) {
	msgs := []*models.Message{
		assistantToolUseMsg("t1"),
		toolResultMsg("t1", ""), // empty content
		assistantToolUseMsg("t2"),
		toolResultMsg("t2", "keep"),
	}

	r := MicroCompact(msgs, 1)
	// Empty tool result should still get the placeholder position but save 0 tokens.
	if r.TokensSaved != 0 {
		t.Errorf("TokensSaved should be 0 for empty content, got %d", r.TokensSaved)
	}
}

func TestMicroCompact_MultipleToolResultsInOneMessage(t *testing.T) {
	// A single user message can have multiple tool_result blocks.
	multiResultMsg := &models.Message{
		ID:   uuid.NewString(),
		Role: models.RoleUser,
		Content: []models.Block{
			models.NewToolResultBlock("t1", "result-1-content", false),
			models.NewToolResultBlock("t2", "result-2-content", false),
		},
	}
	msgs := []*models.Message{
		multiResultMsg,
		assistantToolUseMsg("t3"),
		toolResultMsg("t3", "recent"),
	}

	r := MicroCompact(msgs, 1)

	// t1 and t2 should be cleared, t3 should remain.
	b0 := r.Messages[0].Content[0]
	b1 := r.Messages[0].Content[1]
	if !strings.Contains(b0.Content, "[tool result cleared") {
		t.Errorf("t1 should be cleared, got: %q", b0.Content)
	}
	if !strings.Contains(b1.Content, "[tool result cleared") {
		t.Errorf("t2 should be cleared, got: %q", b1.Content)
	}
	b2 := r.Messages[2].Content[0]
	if b2.Content != "recent" {
		t.Errorf("t3 should be intact, got: %q", b2.Content)
	}
}
