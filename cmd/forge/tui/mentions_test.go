package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- MentionPopup tests ----

func TestMentionPopup_InitiallyInactive(t *testing.T) {
	mp := NewMentionPopup()
	if mp.Active() {
		t.Fatal("expected inactive initially")
	}
	if mp.Selected() != nil {
		t.Fatal("expected nil selection when inactive")
	}
}

func TestMentionPopup_ShowAndHide(t *testing.T) {
	src := &staticMentionSource{
		items: []MentionItem{
			{Label: "foo.go", Value: "foo.go", Category: "Files", Icon: "📄"},
		},
	}
	mp := NewMentionPopup(src)

	mp.Show("")
	if !mp.Active() {
		t.Fatal("expected active after Show")
	}
	if mp.FilteredCount() != 1 {
		t.Fatalf("expected 1 item, got %d", mp.FilteredCount())
	}

	mp.Hide()
	if mp.Active() {
		t.Fatal("expected inactive after Hide")
	}
}

func TestMentionPopup_Navigation(t *testing.T) {
	src := &staticMentionSource{
		items: []MentionItem{
			{Label: "a.go", Value: "a.go", Category: "Files"},
			{Label: "b.go", Value: "b.go", Category: "Files"},
			{Label: "c.go", Value: "c.go", Category: "Files"},
		},
	}
	mp := NewMentionPopup(src)
	mp.Show("")

	// Initially at index 0
	sel := mp.Selected()
	if sel == nil || sel.Value != "a.go" {
		t.Fatalf("expected 'a.go', got %v", sel)
	}

	// Next moves to b.go
	mp.Next()
	sel = mp.Selected()
	if sel.Value != "b.go" {
		t.Fatalf("expected 'b.go', got %q", sel.Value)
	}

	// Prev back to a.go
	mp.Prev()
	sel = mp.Selected()
	if sel.Value != "a.go" {
		t.Fatalf("expected 'a.go', got %q", sel.Value)
	}

	// Prev wraps to c.go
	mp.Prev()
	sel = mp.Selected()
	if sel.Value != "c.go" {
		t.Fatalf("expected wrap to 'c.go', got %q", sel.Value)
	}

	// Next wraps to a.go
	mp.Next()
	sel = mp.Selected()
	if sel.Value != "a.go" {
		t.Fatalf("expected wrap to 'a.go', got %q", sel.Value)
	}
}

func TestMentionPopup_Update(t *testing.T) {
	src := &staticMentionSource{
		items: []MentionItem{
			{Label: "main.go", Value: "main.go", Category: "Files"},
			{Label: "app.go", Value: "app.go", Category: "Files"},
		},
	}
	mp := NewMentionPopup(src)
	mp.Show("")
	if mp.FilteredCount() != 2 {
		t.Fatalf("expected 2 items, got %d", mp.FilteredCount())
	}

	// Update with query that filters down
	// staticMentionSource always returns all items, so just check Update works
	mp.Update("main")
	if !mp.Active() {
		t.Fatal("expected active after Update")
	}
}

func TestMentionPopup_EmptySourcesInactive(t *testing.T) {
	src := &staticMentionSource{items: nil}
	mp := NewMentionPopup(src)
	mp.Show("xyz")
	if mp.Active() {
		t.Fatal("expected inactive when no results")
	}
}

func TestMentionPopup_Render(t *testing.T) {
	src := &staticMentionSource{
		items: []MentionItem{
			{Label: "main.go", Value: "main.go", Category: "Files", Icon: "📄"},
		},
	}
	mp := NewMentionPopup(src)
	theme := ResolveTheme(DarkTheme())

	// Inactive — empty render
	out := mp.Render(80, theme)
	if out != "" {
		t.Fatal("expected empty render when inactive")
	}

	// Active
	mp.Show("")
	out = mp.Render(80, theme)
	if out == "" {
		t.Fatal("expected non-empty render when active")
	}
}

func TestMentionPopup_RenderWithCategories(t *testing.T) {
	src := &staticMentionSource{
		items: []MentionItem{
			{Label: "main.go", Value: "main.go", Category: "Files", Icon: "📄"},
			{Label: "/help", Value: "/help", Category: "Skills", Icon: "⚡"},
		},
	}
	mp := NewMentionPopup(src)
	theme := ResolveTheme(DarkTheme())

	mp.Show("")
	out := mp.Render(80, theme)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
}

// ---- FileMentionSource tests ----

func TestFileMentionSource_Search(t *testing.T) {
	dir := t.TempDir()
	// Create some test files
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "util.go"), []byte("package sub"), 0o644)

	src := &FileMentionSource{Cwd: dir}

	// Empty query returns files
	results := src.Search("")
	if len(results) == 0 {
		t.Fatal("expected results for empty query")
	}
	for _, r := range results {
		if r.Category != "Files" {
			t.Fatalf("expected category 'Files', got %q", r.Category)
		}
		if r.Icon == "" {
			t.Fatal("expected non-empty icon")
		}
	}

	// Query for "main" should return main.go
	results = src.Search("main")
	found := false
	for _, r := range results {
		if r.Value == "main.go" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'main.go' in results for query 'main'")
	}

	// Query for "util" should find sub/util.go
	results = src.Search("util")
	found = false
	for _, r := range results {
		if r.Value == filepath.Join("sub", "util.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'sub/util.go' in results for query 'util', got %v", results)
	}
}

func TestFileMentionSource_SkipsHiddenAndVendor(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(""), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "index.js"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir, "visible.go"), []byte(""), 0o644)

	src := &FileMentionSource{Cwd: dir}
	results := src.Search("")

	for _, r := range results {
		if r.Value == filepath.Join(".git", "config") {
			t.Fatal("should not include .git files")
		}
		if r.Value == filepath.Join("node_modules", "pkg", "index.js") {
			t.Fatal("should not include node_modules files")
		}
	}
}

func TestFileMentionSource_Limit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(dir, filepath.Join("file"+string(rune('a'+i))+".go")), []byte(""), 0o644)
	}

	src := &FileMentionSource{Cwd: dir}
	results := src.Search("")
	if len(results) > 10 {
		t.Fatalf("expected at most 10 results, got %d", len(results))
	}
}

// ---- SkillMentionSource tests ----

func TestSkillMentionSource_Search(t *testing.T) {
	reg := NewCommandRegistry()
	src := &SkillMentionSource{Registry: reg}

	// Empty query returns all non-hidden commands
	results := src.Search("")
	if len(results) == 0 {
		t.Fatal("expected results for empty query")
	}
	for _, r := range results {
		if r.Category != "Skills" {
			t.Fatalf("expected category 'Skills', got %q", r.Category)
		}
		if r.Icon != "⚡" {
			t.Fatalf("expected icon '⚡', got %q", r.Icon)
		}
	}

	// Query "he" should match "help"
	results = src.Search("he")
	found := false
	for _, r := range results {
		if r.Value == "/help" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected '/help' in results for query 'he'")
	}

	// Query "zzz" should match nothing
	results = src.Search("zzz")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for 'zzz', got %d", len(results))
	}
}

func TestSkillMentionSource_NilRegistry(t *testing.T) {
	src := &SkillMentionSource{Registry: nil}
	results := src.Search("")
	if results != nil {
		t.Fatal("expected nil results for nil registry")
	}
}

// ---- AgentMentionSource tests ----

func TestAgentMentionSource_Search(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("# Reviewer agent"), 0o644)
	os.WriteFile(filepath.Join(dir, "test-runner.md"), []byte("# test-runner agent"), 0o644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755) // should be skipped

	src := &AgentMentionSource{Dirs: []string{dir}}

	// Empty query returns all agents
	results := src.Search("")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Category != "Agents" {
			t.Fatalf("expected category 'Agents', got %q", r.Category)
		}
		if r.Icon != "🤖" {
			t.Fatalf("expected icon '🤖', got %q", r.Icon)
		}
	}

	// Query for "review" should match "reviewer"
	results = src.Search("review")
	found := false
	for _, r := range results {
		if r.Value == "reviewer" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'reviewer' in results for query 'review'")
	}
}

func TestAgentMentionSource_NonexistentDir(t *testing.T) {
	src := &AgentMentionSource{Dirs: []string{"/nonexistent/path"}}
	results := src.Search("")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for nonexistent dir, got %d", len(results))
	}
}

func TestAgentMentionSource_Dedup(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "agent.md"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(dir2, "agent.yaml"), []byte(""), 0o644)

	src := &AgentMentionSource{Dirs: []string{dir1, dir2}}
	results := src.Search("")
	if len(results) != 1 {
		t.Fatalf("expected 1 result after dedup, got %d", len(results))
	}
}

// ---- extractMentionQuery tests ----

func TestExtractMentionQuery(t *testing.T) {
	cases := []struct {
		input     string
		wantQuery string
		wantOK    bool
	}{
		{"@", "", true},
		{"@main", "main", true},
		{"@main.go", "main.go", true},
		{"tell me about @foo", "foo", true},
		{"hello @", "", true},
		{"hello @ ", "", false}, // space after @ closes it
		{"@foo bar", "", false}, // space in query closes it
		{"no mention here", "", false},
		{"email@example.com", "", false}, // @ not preceded by space
		{"", "", false},
	}
	for _, c := range cases {
		q, ok := extractMentionQuery(c.input)
		if ok != c.wantOK || q != c.wantQuery {
			t.Errorf("extractMentionQuery(%q) = (%q, %v), want (%q, %v)",
				c.input, q, ok, c.wantQuery, c.wantOK)
		}
	}
}

// ---- App integration tests ----

func TestApp_MentionPopupOpensOnAt(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Type "@" — the KeyRunes handler passes it to textarea, then updateMentions
	next, _ := m.Update(runesMsg("@"))
	m = next.(AppModel)

	if !m.mentions.Active() {
		t.Fatal("expected mention popup to activate on @")
	}
}

func TestApp_MentionPopupClosesOnEsc(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Activate mentions
	m.mentions.Show("")

	next, _ := m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.mentions.Active() {
		t.Fatal("expected mention popup to close on esc")
	}
}

func TestApp_MentionSelectionInsertsValue(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Manually set up mention popup with a known item
	m.mentions = NewMentionPopup(&staticMentionSource{
		items: []MentionItem{
			{Label: "main.go", Value: "main.go", Category: "Files"},
		},
	})

	// Type "@" to trigger
	m.input.SetValue("@")
	m.mentions.Show("")

	// Press enter to select
	next, _ := m.Update(keyMsg("enter"))
	m = next.(AppModel)

	val := m.input.Value()
	if val != "@main.go " {
		t.Fatalf("expected '@main.go ' after selection, got %q", val)
	}
	if m.mentions.Active() {
		t.Fatal("expected mention popup to close after selection")
	}
}

func TestApp_MentionTabNavigation(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	m.mentions = NewMentionPopup(&staticMentionSource{
		items: []MentionItem{
			{Label: "a.go", Value: "a.go", Category: "Files"},
			{Label: "b.go", Value: "b.go", Category: "Files"},
		},
	})
	m.mentions.Show("")

	// Tab moves to next
	next, _ := m.Update(keyMsg("tab"))
	m = next.(AppModel)
	sel := m.mentions.Selected()
	if sel == nil || sel.Value != "b.go" {
		t.Fatalf("expected 'b.go' after tab, got %v", sel)
	}

	// Shift+Tab moves back
	next, _ = m.Update(keyMsg("shift+tab"))
	m = next.(AppModel)
	sel = m.mentions.Selected()
	if sel == nil || sel.Value != "a.go" {
		t.Fatalf("expected 'a.go' after shift+tab, got %v", sel)
	}
}

// ---- shouldSkipPath tests ----

func TestShouldSkipPath(t *testing.T) {
	cases := []struct {
		path string
		skip bool
	}{
		{"main.go", false},
		{"src/app.go", false},
		{".git/config", true},
		{".hidden/file", true},
		{"node_modules/pkg/index.js", true},
		{"vendor/lib/lib.go", true},
		{"__pycache__/mod.pyc", true},
		{"dist/bundle.js", true},
		{"build/output.js", true},
	}
	for _, c := range cases {
		got := shouldSkipPath(c.path)
		if got != c.skip {
			t.Errorf("shouldSkipPath(%q) = %v, want %v", c.path, got, c.skip)
		}
	}
}

// ---- helpers ----

// staticMentionSource is a test helper that returns fixed items.
type staticMentionSource struct {
	items []MentionItem
}

func (s *staticMentionSource) Search(query string) []MentionItem {
	return s.items
}

func (s *staticMentionSource) Category() string {
	return "Test"
}

// runesMsg creates a tea.KeyMsg with KeyRunes type for the given string.
func runesMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
