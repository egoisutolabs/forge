package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "filewrite_test_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()
	return f.Name()
}

func tctxWithCache() *tools.ToolContext {
	return &tools.ToolContext{
		FileState: tools.NewFileStateCache(100, 25*1024*1024),
	}
}

// populateCache seeds the file state cache exactly as FileReadTool would, using
// the file's current mtime so subsequent writes see a fresh timestamp.
func populateCache(t *testing.T, tctx *tools.ToolContext, path, content string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	off := 1
	lim := 2000
	tctx.FileState.Set(path, tools.FileState{
		Content:   content,
		Timestamp: info.ModTime().UnixMilli(),
		Offset:    &off,
		Limit:     &lim,
	})
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestFileWriteTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

// ─── Metadata ─────────────────────────────────────────────────────────────────

func TestFileWriteTool_Name(t *testing.T) {
	if got := (&Tool{}).Name(); got != "Write" {
		t.Errorf("Name() = %q, want %q", got, "Write")
	}
}

func TestFileWriteTool_IsReadOnly_AlwaysFalse(t *testing.T) {
	if (&Tool{}).IsReadOnly(mustJSON(t, map[string]any{"file_path": "/tmp/f", "content": "x"})) {
		t.Error("Write is never read-only")
	}
}

func TestFileWriteTool_IsConcurrencySafe_AlwaysFalse(t *testing.T) {
	if (&Tool{}).IsConcurrencySafe(mustJSON(t, map[string]any{"file_path": "/tmp/f", "content": "x"})) {
		t.Error("Write is never concurrency-safe")
	}
}

func TestFileWriteTool_CheckPermissions_AlwaysAsk(t *testing.T) {
	bt := &Tool{}
	decision, err := bt.CheckPermissions(mustJSON(t, map[string]any{"file_path": "/tmp/f.go", "content": "package main"}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("CheckPermissions = %v, want PermAsk", decision.Behavior)
	}
}

// ─── ValidateInput ─────────────────────────────────────────────────────────────

func TestFileWriteTool_ValidateInput_MissingPath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"content": "x"})); err == nil {
		t.Error("expected error for missing file_path")
	}
}

func TestFileWriteTool_ValidateInput_EmptyPath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": "", "content": "x"})); err == nil {
		t.Error("expected error for empty file_path")
	}
}

func TestFileWriteTool_ValidateInput_RelativePath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": "relative/path.go", "content": "x"})); err == nil {
		t.Error("expected error for relative file_path")
	}
}

func TestFileWriteTool_ValidateInput_Valid(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": "/tmp/ok.go", "content": "package main"})); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileWriteTool_ValidateInput_EmptyContentAllowed(t *testing.T) {
	// Empty content is valid (creates an empty file)
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": "/tmp/ok.go", "content": ""})); err != nil {
		t.Errorf("empty content should be valid, got: %v", err)
	}
}

// ─── Execute: create new file ─────────────────────────────────────────────────

func TestFileWriteTool_Execute_CreateNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new_file.go")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "package main\n\nfunc main() {}\n",
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// File must exist on disk
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "package main\n\nfunc main() {}\n" {
		t.Errorf("wrong content on disk: %q", string(data))
	}

	// Result must mention creation
	if !strings.Contains(result.Content, "created") && !strings.Contains(result.Content, "Created") {
		t.Errorf("result should mention file creation, got: %s", result.Content)
	}
}

func TestFileWriteTool_Execute_CreateDoesNotRequirePriorRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brand_new.txt")

	// tctx with empty cache — should still succeed for creates
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "hello\n",
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("new file creation should not require a prior read, got error: %s", result.Content)
	}
}

func TestFileWriteTool_Execute_CreateAutoCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.go")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "package c\n",
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected parent dirs to be auto-created, got error: %s", result.Content)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file not created at nested path")
	}
}

func TestFileWriteTool_Execute_CreateUpdatesCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.go")
	tctx := tctxWithCache()

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "content here\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("cache not updated after create")
	}
	if state.Content != "content here\n" {
		t.Errorf("cached content = %q, want %q", state.Content, "content here\n")
	}
	// After write, offset and limit should be nil (full file in cache)
	if state.Offset != nil {
		t.Errorf("cached Offset should be nil after write, got %v", state.Offset)
	}
	if state.Limit != nil {
		t.Errorf("cached Limit should be nil after write, got %v", state.Limit)
	}
}

// ─── Execute: update existing file ───────────────────────────────────────────

func TestFileWriteTool_Execute_UpdateRequiresPriorRead(t *testing.T) {
	path := writeTempFile(t, "original content\n")
	tctx := tctxWithCache() // empty cache — no prior read

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "new content\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error: updating without prior read should be rejected")
	}
	if !strings.Contains(result.Content, "read") {
		t.Errorf("error should mention need to read first, got: %s", result.Content)
	}
}

func TestFileWriteTool_Execute_UpdateRejectsPartialView(t *testing.T) {
	path := writeTempFile(t, "line1\nline2\nline3\n")
	tctx := tctxWithCache()

	info, _ := os.Stat(path)
	partialView := true
	tctx.FileState.Set(path, tools.FileState{
		Content:       "line1\nline2\nline3\n",
		Timestamp:     info.ModTime().UnixMilli(),
		IsPartialView: &partialView,
	})

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "modified\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error: partial view should be rejected")
	}
}

func TestFileWriteTool_Execute_UpdateSucceeds(t *testing.T) {
	original := "line1\nline2\nline3\n"
	path := writeTempFile(t, original)
	tctx := tctxWithCache()
	populateCache(t, tctx, path, original)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "line1\nline2 updated\nline3\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// File must be updated on disk
	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2 updated\nline3\n" {
		t.Errorf("disk content = %q, want updated content", string(data))
	}
}

func TestFileWriteTool_Execute_UpdateIncludesDiff(t *testing.T) {
	original := "line1\nline2\nline3\n"
	path := writeTempFile(t, original)
	tctx := tctxWithCache()
	populateCache(t, tctx, path, original)

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "line1\nline2 modified\nline3\n",
	}), tctx)

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}

	// Diff should show the change
	if !strings.Contains(result.Content, "-line2") && !strings.Contains(result.Content, "- line2") {
		t.Errorf("diff should show removed line, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "+line2 modified") && !strings.Contains(result.Content, "+ line2 modified") {
		t.Errorf("diff should show added line, got:\n%s", result.Content)
	}
}

func TestFileWriteTool_Execute_UpdateNoDiffForCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.go")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "package main\n",
	}), tctxWithCache())

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}

	// Create should not include a diff (no original content)
	if strings.Contains(result.Content, "@@") {
		t.Errorf("create result should not include diff hunks, got:\n%s", result.Content)
	}
}

func TestFileWriteTool_Execute_UpdateRefreshesCacheWithNilOffsetLimit(t *testing.T) {
	original := "before\n"
	path := writeTempFile(t, original)
	tctx := tctxWithCache()
	populateCache(t, tctx, path, original) // sets non-nil Offset/Limit

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "after\n",
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("cache not updated after write")
	}
	if state.Content != "after\n" {
		t.Errorf("cache has wrong content: %q", state.Content)
	}
	if state.Offset != nil {
		t.Error("cache Offset should be nil after write")
	}
	if state.Limit != nil {
		t.Error("cache Limit should be nil after write")
	}
}

func TestFileWriteTool_Execute_StaleMtime_Rejected(t *testing.T) {
	original := "original\n"
	path := writeTempFile(t, original)

	info, _ := os.Stat(path)
	staleMtime := info.ModTime().UnixMilli() - 1 // 1ms older than file

	tctx := tctxWithCache()
	tctx.FileState.Set(path, tools.FileState{
		Content:   original,
		Timestamp: staleMtime, // cache is stale: file is newer
	})

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "modified\n",
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// Note: we used tctxWithCache() above (fresh cache), so this fails "not in cache"
	// Let's use the tctx with stale timestamp:
	_ = result // above call was wrong context, redo below
	result, err = (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "modified\n",
	}), tctx)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error: stale mtime should be rejected")
	}
	if !strings.Contains(strings.ToLower(result.Content), "modified") {
		t.Errorf("error should mention file was modified, got: %s", result.Content)
	}
}

func TestFileWriteTool_Execute_NoTctx_StillWorks(t *testing.T) {
	// When tctx is nil, creating a new file should still work.
	dir := t.TempDir()
	path := filepath.Join(dir, "nilctx.go")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "package main\n",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("create with nil tctx should succeed, got: %s", result.Content)
	}
}

// ─── diff algorithm unit tests ────────────────────────────────────────────────

func TestComputeDiff_IdenticalContent(t *testing.T) {
	lines := []string{"a", "b", "c"}
	edits := computeDiff(lines, lines)
	for _, e := range edits {
		if e.op != opKeep {
			t.Errorf("identical content should produce only 'keep' edits, got %c", e.op)
		}
	}
}

func TestComputeDiff_EmptyOld(t *testing.T) {
	edits := computeDiff(nil, []string{"x", "y"})
	for _, e := range edits {
		if e.op != opInsert {
			t.Errorf("empty old should produce only inserts, got %c at %q", e.op, e.text)
		}
	}
	if len(edits) != 2 {
		t.Errorf("expected 2 inserts, got %d", len(edits))
	}
}

func TestComputeDiff_EmptyNew(t *testing.T) {
	edits := computeDiff([]string{"x", "y"}, nil)
	for _, e := range edits {
		if e.op != opDelete {
			t.Errorf("empty new should produce only deletes, got %c at %q", e.op, e.text)
		}
	}
}

func TestComputeDiff_Additions(t *testing.T) {
	old := []string{"a", "b"}
	new_ := []string{"a", "inserted", "b"}
	edits := computeDiff(old, new_)

	ops := editOps(edits)
	if !strings.Contains(ops, "+") {
		t.Errorf("expected insert in edits, got %s", ops)
	}

	// Reconstruct new from edits
	got := applyEdits(edits)
	if !stringSlicesEqual(got, new_) {
		t.Errorf("apply(edits) = %v, want %v", got, new_)
	}
}

func TestComputeDiff_Deletions(t *testing.T) {
	old := []string{"a", "removed", "b"}
	new_ := []string{"a", "b"}
	edits := computeDiff(old, new_)

	ops := editOps(edits)
	if !strings.Contains(ops, "-") {
		t.Errorf("expected delete in edits, got %s", ops)
	}

	got := applyEdits(edits)
	if !stringSlicesEqual(got, new_) {
		t.Errorf("apply(edits) = %v, want %v", got, new_)
	}
}

func TestComputeDiff_MixedChanges(t *testing.T) {
	old := []string{"a", "b", "c", "d"}
	new_ := []string{"a", "B", "c", "D", "e"}
	edits := computeDiff(old, new_)

	got := applyEdits(edits)
	if !stringSlicesEqual(got, new_) {
		t.Errorf("apply(edits) = %v, want %v", got, new_)
	}
}

func TestComputeDiff_CompleteReplace(t *testing.T) {
	old := []string{"x", "y", "z"}
	new_ := []string{"1", "2", "3"}
	edits := computeDiff(old, new_)

	got := applyEdits(edits)
	if !stringSlicesEqual(got, new_) {
		t.Errorf("apply(edits) = %v, want %v", got, new_)
	}
}

// ─── unifiedDiff tests ────────────────────────────────────────────────────────

func TestFormatUnifiedDiff_Headers(t *testing.T) {
	old := []string{"line1", "line2"}
	new_ := []string{"line1", "modified"}
	edits := computeDiff(old, new_)
	out := formatUnifiedDiff(edits, "test.go")

	if !strings.Contains(out, "--- test.go") {
		t.Errorf("missing --- header, got:\n%s", out)
	}
	if !strings.Contains(out, "+++ test.go") {
		t.Errorf("missing +++ header, got:\n%s", out)
	}
}

func TestFormatUnifiedDiff_HunkHeader(t *testing.T) {
	old := []string{"a", "b", "c", "d", "e"}
	new_ := []string{"a", "b", "X", "d", "e"}
	edits := computeDiff(old, new_)
	out := formatUnifiedDiff(edits, "f.go")

	if !strings.Contains(out, "@@") {
		t.Errorf("missing @@ hunk header, got:\n%s", out)
	}
}

func TestFormatUnifiedDiff_ContextLines(t *testing.T) {
	old := []string{"ctx1", "ctx2", "ctx3", "changed", "ctx4", "ctx5", "ctx6"}
	new_ := []string{"ctx1", "ctx2", "ctx3", "CHANGED", "ctx4", "ctx5", "ctx6"}
	edits := computeDiff(old, new_)
	out := formatUnifiedDiff(edits, "f.go")

	// 3 lines of context before and after the change
	if !strings.Contains(out, " ctx1") || !strings.Contains(out, " ctx3") {
		t.Errorf("expected context lines before change, got:\n%s", out)
	}
	if !strings.Contains(out, " ctx4") || !strings.Contains(out, " ctx6") {
		t.Errorf("expected context lines after change, got:\n%s", out)
	}
}

func TestFormatUnifiedDiff_IdenticalNoOutput(t *testing.T) {
	lines := []string{"same", "same", "same"}
	edits := computeDiff(lines, lines)
	out := formatUnifiedDiff(edits, "f.go")

	// No hunks when nothing changed
	if strings.Contains(out, "@@") {
		t.Errorf("identical files should produce no hunks, got:\n%s", out)
	}
}

// ─── timestamp handling ───────────────────────────────────────────────────────

func TestFileWriteTool_Execute_CacheTimestampIsPostWrite(t *testing.T) {
	// After write, the cached mtime should reflect the WRITTEN file's mtime,
	// so a second Write call would succeed (not be rejected as stale).
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	tctx := tctxWithCache()

	// First write (create)
	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "v1\n",
	}), tctx)
	if err != nil {
		t.Fatalf("first write error: %v", err)
	}

	// Tiny sleep to allow mtime to advance on coarse-grained filesystems
	time.Sleep(5 * time.Millisecond)

	// Second write (update) — cache from first write should be valid
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"content":   "v2\n",
	}), tctx)
	if err != nil {
		t.Fatalf("second write error: %v", err)
	}
	if result.IsError {
		t.Errorf("second write should succeed using cache from first write, got: %s", result.Content)
	}
}

// ─── diff helpers ─────────────────────────────────────────────────────────────

// editOps returns the sequence of op characters as a string, e.g. "=+-=".
func editOps(edits []editEntry) string {
	var b strings.Builder
	for _, e := range edits {
		b.WriteByte(byte(e.op))
	}
	return b.String()
}

// applyEdits reconstructs the "new" file from an edit script.
func applyEdits(edits []editEntry) []string {
	var result []string
	for _, e := range edits {
		if e.op == opKeep || e.op == opInsert {
			result = append(result, e.text)
		}
	}
	return result
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

// BenchmarkLcsEdits measures lcsEdits performance with ~1000-line inputs and
// ~10% of lines changed — a realistic code-file update scenario.
func BenchmarkLcsEdits_1000Lines(b *testing.B) {
	const size = 1000
	make1000 := func(prefix string) []string {
		lines := make([]string, size)
		for i := range lines {
			lines[i] = fmt.Sprintf("%s line %d: some representative source code content here", prefix, i)
		}
		return lines
	}
	old := make1000("old")
	newLines := make1000("old")
	for i := 9; i < size; i += 10 { // change every 10th line
		newLines[i] = fmt.Sprintf("new line %d: modified content replaces the original", i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lcsEdits(old, newLines)
	}
}

// BenchmarkLcsEdits_WorstCase exercises the maximum allowed input (1000+1000=2000
// lines) with completely disjoint content — worst-case for the DP table.
func BenchmarkLcsEdits_WorstCase(b *testing.B) {
	const half = 1000
	makeLines := func(prefix string) []string {
		lines := make([]string, half)
		for i := range lines {
			lines[i] = fmt.Sprintf("%s-unique-%d", prefix, i)
		}
		return lines
	}
	old := makeLines("alpha")
	newLines := makeLines("beta")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lcsEdits(old, newLines)
	}
}

// ── Path sanitization (security) ─────────────────────────────────────────────

func TestFileWrite_RelativePath_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"relative/file.txt","content":"data"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative path should be rejected by FileWrite")
	}
}

func TestFileWrite_DotDotRelative_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"../../etc/passwd","content":"data"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative dotdot path should be rejected by FileWrite")
	}
}

func TestFileWrite_AbsWithDotDot_NormalizesAndWrites(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	_ = os.Mkdir(sub, 0755)

	// Write via a path with ".." that resolves to dir/out.txt
	traversalPath := filepath.Join(sub, "..", "out.txt")
	input, _ := json.Marshal(map[string]string{
		"file_path": traversalPath,
		"content":   "written",
	})

	tctx := tctxWithCache()
	tool := &Tool{}
	result, _ := tool.Execute(context.Background(), json.RawMessage(input), tctx)
	if result.IsError {
		t.Errorf("normalized abs dotdot path should succeed: %s", result.Content)
	}

	canonical := filepath.Join(dir, "out.txt")
	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("file not written to canonical path: %v", err)
	}
	if string(data) != "written" {
		t.Errorf("content = %q, want written", string(data))
	}
}
