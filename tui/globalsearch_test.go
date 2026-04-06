package tui

import (
	"sync/atomic"
	"testing"
)

func TestParseRgLine(t *testing.T) {
	tests := []struct {
		line    string
		ok      bool
		file    string
		lineNum int
		content string
	}{
		{
			line:    "src/main.go:42:func main() {",
			ok:      true,
			file:    "src/main.go",
			lineNum: 42,
			content: "func main() {",
		},
		{
			line:    "README.md:1:# Title",
			ok:      true,
			file:    "README.md",
			lineNum: 1,
			content: "# Title",
		},
		{
			line:    "file.go:10:line with:colons:in:content",
			ok:      true,
			file:    "file.go",
			lineNum: 10,
			content: "line with:colons:in:content",
		},
		{
			line: "no-colon-at-all",
			ok:   false,
		},
		{
			line: "file.go:notanumber:content",
			ok:   false,
		},
	}

	for _, tt := range tests {
		result, ok := parseRgLine(tt.line)
		if ok != tt.ok {
			t.Errorf("parseRgLine(%q) ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if result.File != tt.file {
			t.Errorf("parseRgLine(%q).File = %q, want %q", tt.line, result.File, tt.file)
		}
		if result.Line != tt.lineNum {
			t.Errorf("parseRgLine(%q).Line = %d, want %d", tt.line, result.Line, tt.lineNum)
		}
		if result.Content != tt.content {
			t.Errorf("parseRgLine(%q).Content = %q, want %q", tt.line, result.Content, tt.content)
		}
	}
}

func TestGlobalSearchNavigation(t *testing.T) {
	d := &GlobalSearchDialog{
		results: []SearchResult{
			{File: "a.go", Line: 1, Content: "aaa"},
			{File: "b.go", Line: 2, Content: "bbb"},
			{File: "c.go", Line: 3, Content: "ccc"},
		},
	}

	if d.selected != 0 {
		t.Fatalf("initial selection = %d, want 0", d.selected)
	}

	d.Next()
	if d.selected != 1 {
		t.Errorf("after Next: selected = %d, want 1", d.selected)
	}

	d.Next()
	d.Next() // wraps
	if d.selected != 0 {
		t.Errorf("after wrap: selected = %d, want 0", d.selected)
	}

	d.Prev() // wraps backward
	if d.selected != 2 {
		t.Errorf("after Prev from 0: selected = %d, want 2", d.selected)
	}
}

func TestGlobalSearchSelectedPath(t *testing.T) {
	d := &GlobalSearchDialog{
		results: []SearchResult{
			{File: "main.go", Line: 10, Content: "func main()"},
			{File: "app.go", Line: 20, Content: "func app()"},
		},
	}

	if p := d.SelectedPath(); p != "main.go" {
		t.Errorf("SelectedPath() = %q, want %q", p, "main.go")
	}

	d.Next()
	if p := d.SelectedPath(); p != "app.go" {
		t.Errorf("SelectedPath() after Next = %q, want %q", p, "app.go")
	}

	// Empty results
	d2 := &GlobalSearchDialog{}
	if p := d2.SelectedPath(); p != "" {
		t.Errorf("empty dialog SelectedPath() = %q, want empty", p)
	}
}

func TestGlobalSearchSelectedResult(t *testing.T) {
	d := &GlobalSearchDialog{
		results: []SearchResult{
			{File: "a.go", Line: 5, Content: "hello"},
		},
	}

	r := d.SelectedResult()
	if r == nil {
		t.Fatal("SelectedResult() should not be nil")
	}
	if r.File != "a.go" || r.Line != 5 {
		t.Errorf("got %+v, want file=a.go line=5", r)
	}

	// Empty
	d2 := &GlobalSearchDialog{}
	if r2 := d2.SelectedResult(); r2 != nil {
		t.Error("empty dialog SelectedResult() should be nil")
	}
}

func TestGlobalSearchOpenClose(t *testing.T) {
	d := NewGlobalSearchDialog("/tmp")

	d.Open()
	if d.query != "" {
		t.Error("after Open, query should be empty")
	}
	if d.searching {
		t.Error("after Open, searching should be false")
	}

	// Simulate some state
	d.query = "test"
	d.results = []SearchResult{{File: "a.go", Line: 1}}
	d.selected = 1
	d.searching = true

	d.Close()
	if d.query != "" {
		t.Error("after Close, query should be empty")
	}
	if d.results != nil {
		t.Error("after Close, results should be nil")
	}
	if d.selected != 0 {
		t.Error("after Close, selected should be 0")
	}
	if d.searching {
		t.Error("after Close, searching should be false")
	}
}

func TestGlobalSearchRaceProtection(t *testing.T) {
	d := NewGlobalSearchDialog("/tmp")
	d.Open()

	// Simulate a stale result with old generation
	currentGen := atomic.LoadInt64(&globalSearchGeneration)
	staleMsg := globalSearchDoneMsg{
		results: []SearchResult{
			{File: "stale.go", Line: 1, Content: "stale"},
		},
		total:      1,
		generation: currentGen - 1,
	}
	d.HandleSearchDone(staleMsg)

	// Stale result should be ignored
	if len(d.results) != 0 {
		t.Error("stale search result should be ignored")
	}

	// Valid result with current generation
	validMsg := globalSearchDoneMsg{
		results: []SearchResult{
			{File: "valid.go", Line: 1, Content: "valid"},
		},
		total:      1,
		generation: currentGen,
	}
	d.HandleSearchDone(validMsg)
	if len(d.results) != 1 || d.results[0].File != "valid.go" {
		t.Error("valid search result should be applied")
	}
}

func TestGlobalSearchTypeAndBackspace(t *testing.T) {
	d := NewGlobalSearchDialog("/tmp")
	d.Open()

	// TypeRune returns a debounce command
	cmd := d.TypeRune('h')
	if d.query != "h" {
		t.Errorf("after TypeRune('h'): query = %q", d.query)
	}
	if cmd == nil {
		t.Error("TypeRune should return a debounce command")
	}

	d.TypeRune('i')
	if d.query != "hi" {
		t.Errorf("after TypeRune('i'): query = %q", d.query)
	}

	// Backspace
	d.Backspace()
	if d.query != "h" {
		t.Errorf("after Backspace: query = %q", d.query)
	}

	// Backspace to empty clears results
	d.results = []SearchResult{{File: "a.go"}}
	d.Backspace()
	if d.query != "" {
		t.Error("query should be empty after clearing")
	}
	if d.results != nil {
		t.Error("results should be nil after clearing query")
	}

	// Backspace on empty is no-op
	cmd = d.Backspace()
	if cmd != nil {
		t.Error("Backspace on empty should return nil")
	}
}

func TestGlobalSearchDebounce(t *testing.T) {
	d := NewGlobalSearchDialog("/tmp")
	d.Open()

	// scheduleSearch should return a command
	cmd := d.scheduleSearch()
	if cmd == nil {
		t.Error("scheduleSearch should return a non-nil command")
	}
}

func TestGlobalSearchRender(t *testing.T) {
	d := &GlobalSearchDialog{
		query: "test",
		results: []SearchResult{
			{File: "main.go", Line: 10, Content: "func test()"},
			{File: "app.go", Line: 20, Content: "var test = 1"},
		},
		totalFound: 2,
		cwd:        "/tmp",
	}

	theme := InitTheme()
	rendered := d.Render(80, 20, theme)

	if rendered == "" {
		t.Error("Render should produce non-empty output")
	}
}
