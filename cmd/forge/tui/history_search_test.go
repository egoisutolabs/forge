package tui

import (
	"strings"
	"testing"
)

func TestHistorySearch_OpenClose(t *testing.T) {
	h := NewHistory(100)
	hs := NewHistorySearch(h)

	if hs.Active() {
		t.Fatal("expected not active initially")
	}

	hs.Open()
	if !hs.Active() {
		t.Fatal("expected active after Open")
	}

	hs.Close()
	if hs.Active() {
		t.Fatal("expected not active after Close")
	}
}

func TestHistorySearch_FilterAll(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("go test ./...")
	h.Add("git log")

	hs := NewHistorySearch(h)
	hs.Open()

	// Empty query shows all (newest first)
	if hs.ResultCount() != 3 {
		t.Fatalf("expected 3 results, got %d", hs.ResultCount())
	}
	if hs.Selected() != "git log" {
		t.Fatalf("expected 'git log' first (newest), got %q", hs.Selected())
	}
}

func TestHistorySearch_FilterByQuery(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("go test ./...")
	h.Add("git log")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("git")

	if hs.ResultCount() != 2 {
		t.Fatalf("expected 2 results for 'git', got %d", hs.ResultCount())
	}
	// Both results should contain "git"
	for i := 0; i < hs.ResultCount(); i++ {
		// Navigate to check
	}
	if hs.Selected() != "git log" {
		t.Fatalf("expected 'git log' first, got %q", hs.Selected())
	}
}

func TestHistorySearch_FuzzyMatch(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("gist create")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("gst") // fuzzy: g...s...t

	// "git status" matches: g-i-t- -s-t-a-t-u-s (g, s, t in order)
	// "gist create" matches: g-i-s-t (g, s, t in order)
	if hs.ResultCount() != 2 {
		t.Fatalf("expected 2 fuzzy matches, got %d", hs.ResultCount())
	}
}

func TestHistorySearch_NoMatch(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("xyz")

	if hs.ResultCount() != 0 {
		t.Fatalf("expected 0 results, got %d", hs.ResultCount())
	}
	if hs.Selected() != "" {
		t.Fatalf("expected empty selected, got %q", hs.Selected())
	}
}

func TestHistorySearch_TypeChar(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("go test")

	hs := NewHistorySearch(h)
	hs.Open()

	hs.TypeChar('g')
	if hs.Query() != "g" {
		t.Fatalf("expected query 'g', got %q", hs.Query())
	}
	if hs.ResultCount() != 2 {
		t.Fatalf("expected 2 results, got %d", hs.ResultCount())
	}

	hs.TypeChar('o')
	if hs.Query() != "go" {
		t.Fatalf("expected query 'go', got %q", hs.Query())
	}
	if hs.ResultCount() != 1 {
		t.Fatalf("expected 1 result for 'go', got %d", hs.ResultCount())
	}
}

func TestHistorySearch_Backspace(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("git")

	hs.Backspace()
	if hs.Query() != "gi" {
		t.Fatalf("expected query 'gi', got %q", hs.Query())
	}
}

func TestHistorySearch_BackspaceEmpty(t *testing.T) {
	h := NewHistory(100)
	hs := NewHistorySearch(h)
	hs.Open()

	// Should not panic on empty query
	hs.Backspace()
	if hs.Query() != "" {
		t.Fatalf("expected empty query, got %q", hs.Query())
	}
}

func TestHistorySearch_Navigation(t *testing.T) {
	h := NewHistory(100)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	hs := NewHistorySearch(h)
	hs.Open()

	// Initial cursor at 0 (newest first: "third")
	if hs.Selected() != "third" {
		t.Fatalf("expected 'third', got %q", hs.Selected())
	}

	hs.Next()
	if hs.Selected() != "second" {
		t.Fatalf("expected 'second', got %q", hs.Selected())
	}

	hs.Next()
	if hs.Selected() != "first" {
		t.Fatalf("expected 'first', got %q", hs.Selected())
	}

	// Wrap around
	hs.Next()
	if hs.Selected() != "third" {
		t.Fatalf("expected 'third' after wrap, got %q", hs.Selected())
	}

	// Prev wraps backward
	hs.Prev()
	if hs.Selected() != "first" {
		t.Fatalf("expected 'first' after prev wrap, got %q", hs.Selected())
	}
}

func TestHistorySearch_MaxResults(t *testing.T) {
	h := NewHistory(200)
	for i := 0; i < 50; i++ {
		h.Add("entry " + string(rune('a'+i%26)))
	}

	hs := NewHistorySearch(h)
	hs.Open()

	if hs.ResultCount() > MaxSearchResults {
		t.Fatalf("expected at most %d results, got %d", MaxSearchResults, hs.ResultCount())
	}
}

func TestHistorySearch_Render(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")
	h.Add("go test")

	hs := NewHistorySearch(h)
	theme := ResolveTheme(DarkTheme())

	// Not active — empty render
	out := hs.Render(80, theme)
	if out != "" {
		t.Fatal("expected empty render when not active")
	}

	// Active
	hs.Open()
	out = hs.Render(80, theme)
	if out == "" {
		t.Fatal("expected non-empty render when active")
	}
	if !strings.Contains(out, "bck-i-search") {
		t.Fatal("expected 'bck-i-search' in render")
	}
}

func TestHistorySearch_RenderWithQuery(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("git")

	theme := ResolveTheme(DarkTheme())
	out := hs.Render(80, theme)
	if !strings.Contains(out, "git") {
		t.Fatal("expected query 'git' in render")
	}
}

func TestHistorySearch_RenderNoMatches(t *testing.T) {
	h := NewHistory(100)
	h.Add("git status")

	hs := NewHistorySearch(h)
	hs.Open()
	hs.SetQuery("xyz")

	theme := ResolveTheme(DarkTheme())
	out := hs.Render(80, theme)
	if !strings.Contains(out, "no matches") {
		t.Fatal("expected 'no matches' in render")
	}
}

func TestHistorySearch_EmptyHistory(t *testing.T) {
	h := NewHistory(100)
	hs := NewHistorySearch(h)
	hs.Open()

	if hs.ResultCount() != 0 {
		t.Fatalf("expected 0 results for empty history, got %d", hs.ResultCount())
	}
}

func TestHistorySearch_NavigationOnEmpty(t *testing.T) {
	h := NewHistory(100)
	hs := NewHistorySearch(h)
	hs.Open()

	// Should not panic
	hs.Next()
	hs.Prev()
	if hs.Selected() != "" {
		t.Fatalf("expected empty selected, got %q", hs.Selected())
	}
}

// ---- fuzzyContains tests ----

func TestFuzzyContains(t *testing.T) {
	cases := []struct {
		haystack, needle string
		want             bool
	}{
		{"git status", "gst", true},
		{"git status", "git", true},
		{"git status", "status", true},
		{"git status", "gs", true},
		{"git status", "xyz", false},
		{"abc", "abcd", false},
		{"hello world", "", true},
		{"", "a", false},
		{"", "", true},
	}
	for _, c := range cases {
		got := fuzzyContains(c.haystack, c.needle)
		if got != c.want {
			t.Errorf("fuzzyContains(%q, %q) = %v, want %v", c.haystack, c.needle, got, c.want)
		}
	}
}
