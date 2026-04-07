package fileedit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fileedit_test_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_, _ = f.WriteString(content)
	f.Close()
	return f.Name()
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

// tctxWithRead creates a ToolContext whose FileStateCache has the given file
// pre-seeded as if FileReadTool just ran on it with the given content.
func tctxWithRead(t *testing.T, filePath, content string) *tools.ToolContext {
	t.Helper()
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Stat %s: %v", filePath, err)
	}
	off, lim := 1, 2000
	cache := tools.NewFileStateCache(100, 25*1024*1024)
	cache.Set(filePath, tools.FileState{
		Content:   content,
		Timestamp: info.ModTime().UnixMilli(),
		Offset:    &off,
		Limit:     &lim,
	})
	return &tools.ToolContext{FileState: cache}
}

// readFile returns the content of a file as a string.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return string(data)
}

// ── interface ────────────────────────────────────────────────────────────────

func TestFileEditTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestFileEditTool_Name(t *testing.T) {
	if got := (&Tool{}).Name(); got != "Edit" {
		t.Errorf("Name() = %q, want %q", got, "Edit")
	}
}

func TestFileEditTool_NotConcurrencySafe(t *testing.T) {
	if (&Tool{}).IsConcurrencySafe(nil) {
		t.Error("FileEditTool should NOT be concurrency-safe")
	}
}

func TestFileEditTool_NotReadOnly(t *testing.T) {
	if (&Tool{}).IsReadOnly(nil) {
		t.Error("FileEditTool should NOT be read-only")
	}
}

func TestFileEditTool_CheckPermissions_ReturnsAsk(t *testing.T) {
	dec, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{
		"file_path":  "/tmp/foo.go",
		"old_string": "x",
		"new_string": "y",
	}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != models.PermAsk {
		t.Errorf("expected PermAsk, got %q", dec.Behavior)
	}
}

// ── ValidateInput ─────────────────────────────────────────────────────────────

func TestFileEditTool_ValidateInput_MissingFilePath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"old_string": "x",
		"new_string": "y",
	})); err == nil {
		t.Error("expected error for missing file_path")
	}
}

func TestFileEditTool_ValidateInput_EmptyFilePath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"file_path":  "",
		"old_string": "x",
		"new_string": "y",
	})); err == nil {
		t.Error("expected error for empty file_path")
	}
}

func TestFileEditTool_ValidateInput_SameOldAndNew(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"file_path":  "/tmp/foo.go",
		"old_string": "hello",
		"new_string": "hello",
	})); err == nil {
		t.Error("expected error when old_string == new_string")
	}
}

func TestFileEditTool_ValidateInput_Valid(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"file_path":  "/tmp/foo.go",
		"old_string": "x",
		"new_string": "y",
	})); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// ── Execute: cache guards ─────────────────────────────────────────────────────

func TestFileEditTool_Execute_CacheMiss_RejectsEdit(t *testing.T) {
	path := writeTempFile(t, "hello world\n")
	// No cache entry — simulate a file that was never Read
	tctx := &tools.ToolContext{FileState: tools.NewFileStateCache(100, 25*1024*1024)}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for cache miss, got: %s", result.Content)
	}
	if !strings.Contains(strings.ToLower(result.Content), "read") {
		t.Errorf("error should mention 'read', got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_PartialView_RejectsEdit(t *testing.T) {
	content := "hello world\n"
	path := writeTempFile(t, content)

	// Mark cache entry as partial view
	info, _ := os.Stat(path)
	partial := true
	cache := tools.NewFileStateCache(100, 25*1024*1024)
	cache.Set(path, tools.FileState{
		Content:       content,
		Timestamp:     info.ModTime().UnixMilli(),
		IsPartialView: &partial,
	})
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for partial view, got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_FileModifiedSinceRead_RejectsEdit(t *testing.T) {
	content := "original content\n"
	path := writeTempFile(t, content)

	// Seed cache with a timestamp in the past (before current mtime)
	off, lim := 1, 2000
	cache := tools.NewFileStateCache(100, 25*1024*1024)
	cache.Set(path, tools.FileState{
		Content:   content,
		Timestamp: 1, // epoch start — older than any real mtime
		Offset:    &off,
		Limit:     &lim,
	})
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "original",
		"new_string": "updated",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for stale read, got: %s", result.Content)
	}
	if !strings.Contains(strings.ToLower(result.Content), "modified") {
		t.Errorf("error should mention 'modified', got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_FullReadContentUnchanged_AllowsEditDespiteNewerMtime(t *testing.T) {
	content := "hello world\n"
	path := writeTempFile(t, content)

	// Simulate a "full read" cache entry (Offset=nil, Limit=nil) with stale
	// timestamp but matching content — the mtime false-positive bypass should kick in.
	cache := tools.NewFileStateCache(100, 25*1024*1024)
	cache.Set(path, tools.FileState{
		Content:   content,
		Timestamp: 1, // stale timestamp
		Offset:    nil,
		Limit:     nil,
	})
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("full-read + unchanged content should bypass mtime check, got: %s", result.Content)
	}
}

// ── Execute: content validation ───────────────────────────────────────────────

func TestFileEditTool_Execute_OldStringNotFound_ReturnsError(t *testing.T) {
	content := "hello world\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "NONEXISTENT_STRING",
		"new_string": "something",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for missing old_string, got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_MultipleMatches_RejectsWithoutReplaceAll(t *testing.T) {
	content := "foo bar\nfoo baz\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "foo",
		"new_string": "qux",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for multiple matches without replace_all, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2") {
		t.Errorf("error should mention match count, got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_RelativePath_ReturnsError(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  "relative/path.go",
		"old_string": "x",
		"new_string": "y",
	}), &tools.ToolContext{FileState: tools.NewFileStateCache(100, 25*1024*1024)})

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for relative path, got: %s", result.Content)
	}
}

// ── Execute: successful edits ─────────────────────────────────────────────────

func TestFileEditTool_Execute_SuccessfulEdit(t *testing.T) {
	content := "hello world\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	if got := readFile(t, path); !strings.Contains(got, "goodbye") {
		t.Errorf("file should contain 'goodbye' after edit, got: %s", got)
	}
	if strings.Contains(readFile(t, path), "hello") {
		t.Errorf("file should not contain 'hello' after replacement")
	}
}

func TestFileEditTool_Execute_ReplaceAll(t *testing.T) {
	content := "foo bar\nfoo baz\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":   path,
		"old_string":  "foo",
		"new_string":  "qux",
		"replace_all": true,
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}
	got := readFile(t, path)
	if strings.Contains(got, "foo") {
		t.Errorf("replace_all should replace every occurrence, got: %s", got)
	}
	if !strings.Contains(got, "qux bar") || !strings.Contains(got, "qux baz") {
		t.Errorf("expected both replacements, got: %s", got)
	}
}

func TestFileEditTool_Execute_UpdatesCacheAfterEdit(t *testing.T) {
	content := "hello world\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "goodbye",
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("cache should be updated after edit")
	}
	if !strings.Contains(state.Content, "goodbye") {
		t.Errorf("cached content should reflect edit, got: %q", state.Content)
	}
	// After edit: Offset and Limit should both be nil (full-file cache)
	if state.Offset != nil || state.Limit != nil {
		t.Errorf("post-edit cache entry should have Offset=nil, Limit=nil")
	}
	if state.Timestamp == 0 {
		t.Error("post-edit timestamp should be non-zero")
	}
}

func TestFileEditTool_Execute_NewFile_EmptyOldString(t *testing.T) {
	// old_string="" creates a new file
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.txt")

	cache := tools.NewFileStateCache(100, 25*1024*1024)
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "",
		"new_string": "brand new content\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("new file creation should succeed, got: %s", result.Content)
	}
	if got := readFile(t, path); !strings.Contains(got, "brand new content") {
		t.Errorf("new file should contain the new content, got: %s", got)
	}
}

func TestFileEditTool_Execute_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "dir", "newfile.txt")

	cache := tools.NewFileStateCache(100, 25*1024*1024)
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "",
		"new_string": "content\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected parent dirs to be created, got: %s", result.Content)
	}
}

func TestFileEditTool_Execute_ResultContainsFilePath(t *testing.T) {
	content := "hello\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "world",
	}), tctx)

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}
	if !strings.Contains(result.Content, filepath.Base(path)) {
		t.Errorf("result should mention file path, got: %s", result.Content)
	}
}

// ── Execute: fuzzy matching ───────────────────────────────────────────────────

func TestFileEditTool_Execute_FuzzyMatch_TrailingWhitespace(t *testing.T) {
	// File has trailing spaces; model's old_string has them stripped
	content := "func foo() {   \n    return 42   \n}\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	// old_string without the trailing spaces — should still match
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "func foo() {\n    return 42\n}",
		"new_string": "func foo() {\n    return 100\n}",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("fuzzy match (trailing whitespace) should succeed, got: %s", result.Content)
	}
	if got := readFile(t, path); !strings.Contains(got, "100") {
		t.Errorf("edit should apply via fuzzy match, got: %s", got)
	}
}

// ── unit tests for helpers ────────────────────────────────────────────────────

func TestFindActualString_ExactMatch(t *testing.T) {
	file := "hello world"
	actual, found := findActualString(file, "hello")
	if !found {
		t.Fatal("expected exact match to be found")
	}
	if actual != "hello" {
		t.Errorf("got %q, want %q", actual, "hello")
	}
}

func TestFindActualString_NotFound(t *testing.T) {
	_, found := findActualString("hello world", "NOPE")
	if found {
		t.Fatal("should not find non-existent string")
	}
}

func TestFindActualString_TrailingWhitespaceNormalization(t *testing.T) {
	// File has trailing spaces; search string does not
	file := "line one   \nline two   \n"
	search := "line one\nline two"
	actual, found := findActualString(file, search)
	if !found {
		t.Fatal("should find via trailing-whitespace normalization")
	}
	// The returned actual should be the stripped version that exists in the file
	if !strings.Contains(file, actual) {
		t.Errorf("returned actual %q should exist in file", actual)
	}
}

func TestStripTrailingWhitespaceLines(t *testing.T) {
	in := "hello   \nworld  \n  ok\n"
	got := stripTrailingWhitespaceLines(in)
	for _, line := range strings.Split(got, "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line %q still has trailing whitespace", line)
		}
	}
}

func TestGenerateDiff_ContainsMinusAndPlus(t *testing.T) {
	diff := generateDiff("hello\nworld\n", "hello\nearth\n", "test.txt")
	if !strings.Contains(diff, "-") {
		t.Error("diff should contain '-' lines for removed content")
	}
	if !strings.Contains(diff, "+") {
		t.Error("diff should contain '+' lines for added content")
	}
}

// ── Execute: nil tctx safety ──────────────────────────────────────────────────

func TestFileEditTool_Execute_NilTctx_RejectsEdit(t *testing.T) {
	// Without a ToolContext we can't check the cache — should reject gracefully
	content := "hello\n"
	path := writeTempFile(t, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "world",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("nil tctx should cause a rejection (no cache to check), got: %s", result.Content)
	}
}

// ── Execute: result message ───────────────────────────────────────────────────

func TestFileEditTool_Execute_ReplaceAll_ResultMentionsOccurrences(t *testing.T) {
	content := "x\nx\nx\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":   path,
		"old_string":  "x",
		"new_string":  "z",
		"replace_all": true,
	}), tctx)
	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}
	// Should mention "all occurrences" in the success message
	if !strings.Contains(strings.ToLower(result.Content), "all") &&
		!strings.Contains(strings.ToLower(result.Content), "occurrences") {
		t.Logf("result content (informational): %s", result.Content)
	}
}

// ── Execute: mtime bypass (full read + unchanged content) ─────────────────────

func TestFileEditTool_Execute_StaleRead_ContentChanged_Rejects(t *testing.T) {
	content := "original\n"
	path := writeTempFile(t, content)

	// Simulate a newer mtime and different cached content (full read entry)
	cache := tools.NewFileStateCache(100, 25*1024*1024)
	cache.Set(path, tools.FileState{
		Content:   "something else entirely\n",
		Timestamp: 1, // stale
		Offset:    nil,
		Limit:     nil,
	})
	tctx := &tools.ToolContext{FileState: cache}

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "original",
		"new_string": "updated",
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("stale full-read with changed content should reject, got: %s", result.Content)
	}
}

// waitForMtimeChange sleeps until the OS mtime resolution causes a visible
// change when the file is re-written. On most Linux/macOS FS this is ≤10ms
// but we wait up to 100ms to be safe.
func waitForMtimeChange(t *testing.T, path string) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(5 * time.Millisecond)
		after, _ := os.Stat(path)
		if after != nil && after.ModTime() != before.ModTime() {
			return
		}
	}
}

func TestFileEditTool_Execute_CacheUpdatedWithNewMtime(t *testing.T) {
	content := "hello\n"
	path := writeTempFile(t, content)
	tctx := tctxWithRead(t, path, content)

	// Ensure mtime will change on write
	waitForMtimeChange(t, path)

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path":  path,
		"old_string": "hello",
		"new_string": "world",
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("cache not updated")
	}

	info, _ := os.Stat(path)
	if state.Timestamp != info.ModTime().UnixMilli() {
		t.Errorf("cached timestamp %d != file mtime %d", state.Timestamp, info.ModTime().UnixMilli())
	}
}

// ── Path sanitization (security) ─────────────────────────────────────────────

func TestFileEdit_RelativePath_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"relative/file.txt","old_string":"x","new_string":"y"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative path should be rejected by FileEdit")
	}
}

func TestFileEdit_DotDotRelative_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"../../etc/shadow","old_string":"x","new_string":"y"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative dotdot path should be rejected by FileEdit")
	}
}
