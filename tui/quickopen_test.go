package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		pattern string
		str     string
		match   bool
	}{
		// Empty pattern matches everything
		{"", "anything.go", true},
		{"", "", true},

		// Exact substring
		{"main", "cmd/main.go", true},
		{"app", "tui/app.go", true},

		// Fuzzy characters in order
		{"mg", "cmd/main.go", true},      // m...g in main.go
		{"ag", "tui/app.go", true},       // a...g in app.go
		{"qo", "tui/quickopen.go", true}, // q...o in quickopen

		// No match when chars not in order
		{"zz", "main.go", false},
		{"gm", "main.go", false}, // g appears before m — reversed order

		// Case insensitive
		{"MAIN", "cmd/main.go", true},
		{"Main", "cmd/MAIN.go", true},

		// Path components
		{"tuiapp", "tui/app.go", true},
		{"srcidx", "src/index.ts", true},
	}

	for _, tt := range tests {
		ok, _ := fuzzyMatch(tt.pattern, tt.str)
		if ok != tt.match {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.pattern, tt.str, ok, tt.match)
		}
	}
}

func TestFuzzyMatchScoring(t *testing.T) {
	// Consecutive matches should score higher
	_, consec := fuzzyMatch("app", "tui/app.go")
	_, sparse := fuzzyMatch("app", "tui/a_p_p.go")
	if consec <= sparse {
		t.Errorf("consecutive match (%d) should score higher than sparse (%d)", consec, sparse)
	}

	// Filename match should score higher than deep path match
	_, filename := fuzzyMatch("app", "app.go")
	_, deep := fuzzyMatch("app", "very/deep/path/app.go")
	if filename <= deep {
		t.Errorf("filename match (%d) should score higher than deep path (%d)", filename, deep)
	}

	// Word boundary match should score higher
	_, boundary := fuzzyMatch("a", "tui/app.go") // matches 'a' at word boundary after /
	_, middle := fuzzyMatch("a", "tui/baz.go")   // matches 'a' in middle of 'baz'
	if boundary < middle {
		t.Errorf("boundary match (%d) should score >= middle match (%d)", boundary, middle)
	}
}

func TestScanFiles(t *testing.T) {
	// Create a temp directory with test files
	dir := t.TempDir()

	// Create files
	files := []string{
		"main.go",
		"README.md",
		"src/app.go",
		"src/utils.go",
		"src/deep/nested.go",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("test"), 0o644)
	}

	// Create directories that should be skipped
	skipDirs := []string{
		".git/config",
		"node_modules/pkg/index.js",
		"vendor/lib/main.go",
		"__pycache__/cache.pyc",
	}
	for _, f := range skipDirs {
		path := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("test"), 0o644)
	}

	result := scanFiles(dir)

	// Should find our test files
	if len(result) != len(files) {
		t.Errorf("scanFiles found %d files, want %d", len(result), len(files))
	}

	// Should not include any skipped paths
	for _, r := range result {
		for _, skip := range []string{".git", "node_modules", "vendor", "__pycache__"} {
			if strings.HasPrefix(r, skip+string(filepath.Separator)) || r == skip {
				t.Errorf("scanFiles included skipped path: %s", r)
			}
		}
	}
}

func TestQuickOpenFilterFiles(t *testing.T) {
	d := &QuickOpenDialog{
		allFiles: []string{
			"main.go",
			"tui/app.go",
			"tui/quickopen.go",
			"tui/globalsearch.go",
			"internal/log/log.go",
			"README.md",
		},
		scanned: true,
	}

	// No query — all files
	d.query = ""
	d.filterFiles()
	if len(d.results) != 6 {
		t.Errorf("empty query: got %d results, want 6", len(d.results))
	}

	// Filter for "go" — should match .go files
	d.query = "go"
	d.filterFiles()
	if len(d.results) != 5 { // all .go files
		t.Errorf("'go' query: got %d results, want 5", len(d.results))
	}

	// Filter for "qo" — should match quickopen
	d.query = "qo"
	d.filterFiles()
	found := false
	for _, r := range d.results {
		if r.Path == "tui/quickopen.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'qo' query should match tui/quickopen.go")
	}

	// Filter for "zzz" — no matches
	d.query = "zzz"
	d.filterFiles()
	if len(d.results) != 0 {
		t.Errorf("'zzz' query: got %d results, want 0", len(d.results))
	}
}

func TestQuickOpenResultLimiting(t *testing.T) {
	// Create a dialog with more files than maxQuickOpenResults
	files := make([]string, 200)
	for i := range files {
		files[i] = filepath.Join("src", "file"+string(rune('a'+i%26))+".go")
	}

	d := &QuickOpenDialog{
		allFiles: files,
		scanned:  true,
	}

	d.query = ""
	d.filterFiles()
	if len(d.results) > maxQuickOpenResults {
		t.Errorf("results exceed limit: got %d, max %d", len(d.results), maxQuickOpenResults)
	}
	if d.totalFound != 200 {
		t.Errorf("totalFound = %d, want 200", d.totalFound)
	}
}

func TestQuickOpenNavigation(t *testing.T) {
	d := &QuickOpenDialog{
		allFiles: []string{"a.go", "b.go", "c.go"},
		scanned:  true,
	}
	d.filterFiles()

	if d.selected != 0 {
		t.Fatalf("initial selection should be 0, got %d", d.selected)
	}

	d.Next()
	if d.selected != 1 {
		t.Errorf("after Next: selected = %d, want 1", d.selected)
	}

	d.Next()
	if d.selected != 2 {
		t.Errorf("after Next*2: selected = %d, want 2", d.selected)
	}

	// Wrap around
	d.Next()
	if d.selected != 0 {
		t.Errorf("after wrap: selected = %d, want 0", d.selected)
	}

	// Prev wraps backward
	d.Prev()
	if d.selected != 2 {
		t.Errorf("after Prev from 0: selected = %d, want 2", d.selected)
	}
}

func TestQuickOpenTypeAndBackspace(t *testing.T) {
	d := &QuickOpenDialog{
		allFiles: []string{"main.go", "test.go", "readme.md"},
		scanned:  true,
	}
	d.filterFiles()

	d.TypeRune('m')
	if d.query != "m" {
		t.Errorf("after TypeRune('m'): query = %q", d.query)
	}
	// Should filter to main.go and readme.md
	if len(d.results) < 1 {
		t.Error("should have results after typing 'm'")
	}

	d.Backspace()
	if d.query != "" {
		t.Errorf("after Backspace: query = %q", d.query)
	}
	if len(d.results) != 3 {
		t.Errorf("after clearing query: got %d results, want 3", len(d.results))
	}

	// Backspace on empty query is no-op
	d.Backspace()
	if d.query != "" {
		t.Error("backspace on empty should be no-op")
	}
}

func TestQuickOpenSelectedPath(t *testing.T) {
	d := &QuickOpenDialog{
		allFiles: []string{"a.go", "b.go"},
		scanned:  true,
	}
	d.filterFiles()

	if p := d.SelectedPath(); p != "a.go" {
		t.Errorf("SelectedPath() = %q, want %q", p, "a.go")
	}

	d.Next()
	if p := d.SelectedPath(); p != "b.go" {
		t.Errorf("SelectedPath() after Next = %q, want %q", p, "b.go")
	}

	// Empty results
	d2 := &QuickOpenDialog{scanned: true}
	if p := d2.SelectedPath(); p != "" {
		t.Errorf("empty dialog SelectedPath() = %q, want empty", p)
	}
}

func TestQuickOpenRaceProtection(t *testing.T) {
	d := NewQuickOpenDialog("/tmp")

	// Simulate stale scan result with old generation
	currentGen := atomic.LoadInt64(&quickOpenGeneration)
	d.HandleScanDone(quickOpenScanDoneMsg{
		files:      []string{"stale.go"},
		generation: currentGen - 1,
	})

	// Should not have been applied
	if d.scanned {
		t.Error("stale scan result should not mark dialog as scanned")
	}
	if len(d.allFiles) != 0 {
		t.Error("stale scan result should not set allFiles")
	}
}

func TestQuickOpenRender(t *testing.T) {
	d := &QuickOpenDialog{
		allFiles: []string{"main.go", "app.go"},
		scanned:  true,
		cwd:      "/tmp",
	}
	d.filterFiles()

	theme := InitTheme()
	rendered := d.Render(80, 20, theme)

	if rendered == "" {
		t.Error("Render should produce non-empty output")
	}

	// Should contain header
	if !containsStr(rendered, "Quick Open") {
		t.Error("render should contain 'Quick Open' header")
	}
}

// containsStr is a test helper to check substring presence.
func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}

func TestQuickOpenDialogLifecycle(t *testing.T) {
	d := NewQuickOpenDialog("/tmp")

	// Open returns a command
	cmd := d.Open()
	if cmd == nil {
		t.Error("Open should return a non-nil command")
	}
	if d.query != "" {
		t.Error("after Open, query should be empty")
	}
}
