package tui

import (
	"testing"
)

func TestTypeahead_HistoryMatch(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("git log --oneline")
	h.Add("go test ./...")

	ta := NewTypeahead(h)

	// "go" should match "go test ./..." (newest match)
	ta.Update("go")
	if ta.Ghost() != " test ./..." {
		t.Fatalf("expected ghost ' test ./...', got %q", ta.Ghost())
	}

	// "git" should match "git log --oneline" (newest match)
	ta.Update("git")
	if ta.Ghost() != " log --oneline" {
		t.Fatalf("expected ghost ' log --oneline', got %q", ta.Ghost())
	}

	// "git s" should match "git status"
	ta.Update("git s")
	if ta.Ghost() != "tatus" {
		t.Fatalf("expected ghost 'tatus', got %q", ta.Ghost())
	}
}

func TestTypeahead_NoMatch(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	ta := NewTypeahead(h)
	ta.Update("xyz")
	if ta.Ghost() != "" {
		t.Fatalf("expected empty ghost for no match, got %q", ta.Ghost())
	}
}

func TestTypeahead_EmptyInput(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	ta := NewTypeahead(h)
	ta.Update("")
	if ta.Ghost() != "" {
		t.Fatalf("expected empty ghost for empty input, got %q", ta.Ghost())
	}
}

func TestTypeahead_ExactMatch(t *testing.T) {
	h := NewHistory(100)
	h.Add("hello")

	ta := NewTypeahead(h)
	// Exact match should not produce ghost text
	ta.Update("hello")
	if ta.Ghost() != "" {
		t.Fatalf("expected empty ghost for exact match, got %q", ta.Ghost())
	}
}

func TestTypeahead_CaseInsensitive(t *testing.T) {
	h := NewHistory(100)
	h.Add("Git Status")

	ta := NewTypeahead(h)
	ta.Update("git")
	if ta.Ghost() != " Status" {
		t.Fatalf("expected ghost ' Status', got %q", ta.Ghost())
	}
}

func TestTypeahead_AcceptWithGhost(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	ta := NewTypeahead(h)
	ta.Update("git")

	full, ok := ta.Accept("git")
	if !ok || full != "git status" {
		t.Fatalf("expected 'git status', got %q ok=%v", full, ok)
	}

	// After accept, ghost should be cleared
	if ta.HasGhost() {
		t.Fatal("expected HasGhost=false after accept")
	}
}

func TestTypeahead_AcceptNoGhost(t *testing.T) {
	h := NewHistory(100)
	ta := NewTypeahead(h)

	_, ok := ta.Accept("something")
	if ok {
		t.Fatal("expected ok=false with no ghost")
	}
}

func TestTypeahead_Clear(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	ta := NewTypeahead(h)
	ta.Update("git")
	if !ta.HasGhost() {
		t.Fatal("expected ghost after update")
	}

	ta.Clear()
	if ta.HasGhost() {
		t.Fatal("expected no ghost after clear")
	}
}

func TestTypeahead_SlashCommandPattern(t *testing.T) {
	h := NewHistory(100)
	ta := NewTypeahead(h)

	// "/he" should match "/help"
	ta.Update("/he")
	if ta.Ghost() != "lp" {
		t.Fatalf("expected ghost 'lp', got %q", ta.Ghost())
	}

	// "/cl" should match "/clear"
	ta.Update("/cl")
	if ta.Ghost() != "ear" {
		t.Fatalf("expected ghost 'ear', got %q", ta.Ghost())
	}
}

func TestTypeahead_HistoryPriorityOverPattern(t *testing.T) {
	h := NewHistory(100)
	h.Add("/help me please")

	ta := NewTypeahead(h)
	// "/he" should match history entry (longer) over built-in "/help"
	ta.Update("/he")
	if ta.Ghost() != "lp me please" {
		t.Fatalf("expected ghost 'lp me please', got %q", ta.Ghost())
	}
}

func TestTypeahead_RenderGhost(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	ta := NewTypeahead(h)
	ta.Update("git")

	theme := ResolveTheme(DarkTheme())
	rendered := ta.RenderGhost(theme)
	if rendered == "" {
		t.Fatal("expected non-empty rendered ghost")
	}
}

func TestTypeahead_RenderGhostEmpty(t *testing.T) {
	h := NewHistory(100)
	ta := NewTypeahead(h)

	theme := ResolveTheme(DarkTheme())
	rendered := ta.RenderGhost(theme)
	if rendered != "" {
		t.Fatalf("expected empty render for no ghost, got %q", rendered)
	}
}

func TestTypeahead_NewestMatchWins(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("git log")
	h.Add("git diff")

	ta := NewTypeahead(h)
	ta.Update("git")
	// Should match "git diff" (newest)
	if ta.Ghost() != " diff" {
		t.Fatalf("expected ghost ' diff', got %q", ta.Ghost())
	}
}
