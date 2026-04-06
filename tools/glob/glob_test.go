package glob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// --- helpers ---

func mkTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := []struct {
		rel   string
		mtime time.Time
	}{
		{"main.go", time.Now().Add(-3 * time.Hour)},
		{"main_test.go", time.Now().Add(-2 * time.Hour)},
		{"README.md", time.Now().Add(-1 * time.Hour)},
		{"src/util.go", time.Now().Add(-4 * time.Hour)},
		{"src/util_test.go", time.Now().Add(-5 * time.Hour)},
		{"src/deep/nested.go", time.Now().Add(-6 * time.Hour)},
		{"src/deep/nested_test.go", time.Now().Add(-7 * time.Hour)},
		{"docs/api.md", time.Now().Add(-30 * time.Minute)},
		{"docs/guide.md", time.Now().Add(-10 * time.Minute)},
	}

	for _, f := range files {
		path := filepath.Join(root, f.rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, f.mtime, f.mtime); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func execGlob(t *testing.T, pattern, path string, maxResults int) *models.ToolResult {
	t.Helper()
	tool := &Tool{}
	inp := map[string]any{"pattern": pattern}
	if path != "" {
		inp["path"] = path
	}
	raw, _ := json.Marshal(inp)
	tctx := &tools.ToolContext{
		Cwd:            path,
		GlobMaxResults: maxResults,
	}
	result, err := tool.Execute(context.Background(), json.RawMessage(raw), tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	return result
}

// --- interface compliance ---

func TestGlobTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestGlobTool_Name(t *testing.T) {
	if (&Tool{}).Name() != "Glob" {
		t.Errorf("Name() = %q, want 'Glob'", (&Tool{}).Name())
	}
}

func TestGlobTool_AlwaysReadOnly(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"**/*.go"}`)
	if !(&Tool{}).IsReadOnly(raw) {
		t.Error("IsReadOnly should always be true")
	}
}

func TestGlobTool_AlwaysConcurrencySafe(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"**/*.go"}`)
	if !(&Tool{}).IsConcurrencySafe(raw) {
		t.Error("IsConcurrencySafe should always be true")
	}
}

func TestGlobTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	raw := json.RawMessage(`{"pattern":"**/*.go"}`)
	decision, err := (&Tool{}).CheckPermissions(raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAllow {
		t.Errorf("expected PermAllow, got %v", decision.Behavior)
	}
}

// --- input validation ---

func TestGlobTool_ValidateInput_MissingPattern(t *testing.T) {
	err := (&Tool{}).ValidateInput(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing pattern")
	}
}

func TestGlobTool_ValidateInput_EmptyPattern(t *testing.T) {
	err := (&Tool{}).ValidateInput(json.RawMessage(`{"pattern":""}`))
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestGlobTool_ValidateInput_ValidPattern(t *testing.T) {
	err := (&Tool{}).ValidateInput(json.RawMessage(`{"pattern":"**/*.go"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGlobTool_ValidateInput_NonExistentPath(t *testing.T) {
	err := (&Tool{}).ValidateInput(json.RawMessage(`{"pattern":"*.go","path":"/nonexistent/path/xyz"}`))
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestGlobTool_ValidateInput_PathIsFile(t *testing.T) {
	f, _ := os.CreateTemp("", "glob-test-*.txt")
	f.Close()
	defer os.Remove(f.Name())

	raw, _ := json.Marshal(map[string]string{"pattern": "*.go", "path": f.Name()})
	err := (&Tool{}).ValidateInput(json.RawMessage(raw))
	if err == nil {
		t.Error("expected error when path is a file, not directory")
	}
}

// --- basic matching ---

func TestGlobTool_MatchGoFiles(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "*.go", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	if len(lines) != 2 {
		t.Errorf("expected 2 .go files in root, got %d: %v", len(lines), lines)
	}
	for _, l := range lines {
		if !strings.HasSuffix(l, ".go") {
			t.Errorf("expected .go file, got %q", l)
		}
	}
}

func TestGlobTool_MatchMarkdownFiles_Recursive(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "**/*.md", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	// README.md, docs/api.md, docs/guide.md = 3
	if len(lines) != 3 {
		t.Errorf("expected 3 .md files, got %d: %v", len(lines), lines)
	}
}

func TestGlobTool_MatchAllGoFiles_Recursive(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "**/*.go", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	// main.go, main_test.go, src/util.go, src/util_test.go,
	// src/deep/nested.go, src/deep/nested_test.go = 6
	if len(lines) != 6 {
		t.Errorf("expected 6 .go files recursively, got %d: %v", len(lines), lines)
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "**/*.rs", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No files found") {
		t.Errorf("expected 'No files found', got %q", result.Content)
	}
}

// --- mtime sort (newest first) ---

func TestGlobTool_SortedByMtimeNewestFirst(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "**/*.md", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	// Newest: docs/guide.md (-10m), docs/api.md (-30m), README.md (-1h)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "guide.md") {
		t.Errorf("first result should be newest (guide.md), got %q", lines[0])
	}
	if !strings.Contains(lines[1], "api.md") {
		t.Errorf("second result should be api.md, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "README.md") {
		t.Errorf("third result should be README.md, got %q", lines[2])
	}
}

// --- limit / truncation ---

func TestGlobTool_TruncatesAtLimit(t *testing.T) {
	root := mkTree(t)
	// Only 2 results
	result := execGlob(t, "**/*.go", root, 2)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Errorf("expected truncation notice, got: %s", result.Content)
	}
}

func TestGlobTool_DefaultLimit100(t *testing.T) {
	// GlobMaxResults=0 means use default (100). With only 6 go files, no truncation.
	root := mkTree(t)
	result := execGlob(t, "**/*.go", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if strings.Contains(result.Content, "truncated") {
		t.Error("should not truncate 6 files at default limit of 100")
	}
}

// --- relative paths ---

func TestGlobTool_ReturnsRelativePaths(t *testing.T) {
	root := mkTree(t)
	result := execGlob(t, "**/*.go", root, 0)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	for _, l := range lines {
		if filepath.IsAbs(l) {
			t.Errorf("expected relative path, got absolute: %q", l)
		}
	}
}

func TestGlobTool_UsesCwdFromToolContext(t *testing.T) {
	root := mkTree(t)
	// Run glob on the `src` subdirectory, cwd = root
	tool := &Tool{}
	inp, _ := json.Marshal(map[string]string{
		"pattern": "**/*.go",
		"path":    filepath.Join(root, "src"),
	})
	tctx := &tools.ToolContext{Cwd: root, GlobMaxResults: 0}
	result, err := tool.Execute(context.Background(), json.RawMessage(inp), tctx)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	// Results should be relative to root cwd, not to `src`
	for _, l := range lines {
		if filepath.IsAbs(l) {
			t.Errorf("expected relative path from cwd, got absolute: %q", l)
		}
		if !strings.HasPrefix(l, "src") {
			t.Errorf("path should start with 'src/' when cwd is root, got %q", l)
		}
	}
}

// --- glob pattern types ---

func TestGlobTool_SingleStarDoesNotCrossDirectories(t *testing.T) {
	root := mkTree(t)
	// Single star: only matches in root dir
	result := execGlob(t, "*.go", root, 0)
	lines := nonEmpty(strings.Split(result.Content, "\n"))
	for _, l := range lines {
		if strings.Contains(l, string(filepath.Separator)) {
			t.Errorf("single star should not cross dirs, got %q", l)
		}
	}
}

func TestGlobTool_InputSchema_Valid(t *testing.T) {
	schema := (&Tool{}).InputSchema()
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema is not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Error("schema type should be 'object'")
	}
}

// --- helper ---

func nonEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ── Path sanitization (security) ─────────────────────────────────────────────

func TestGlob_AbsPathWithDotDot_Normalized(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	_ = os.Mkdir(sub, 0755)
	_ = os.WriteFile(filepath.Join(dir, "file.go"), []byte(""), 0644)

	// Search from "sub/.." which normalizes to dir
	traversalPath := filepath.Join(sub, "..")
	input, _ := json.Marshal(map[string]string{
		"pattern": "*.go",
		"path":    traversalPath,
	})
	tool := &Tool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("normalized dotdot glob path should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "file.go") {
		t.Errorf("expected file.go in results, got: %s", result.Content)
	}
}
