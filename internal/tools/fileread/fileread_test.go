package fileread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/tools"
)

// writeTempFile creates a temporary file with the given content and returns its path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fileread_test_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	f.Close()
	return f.Name()
}

// mustJSON serializes input as a JSON raw message.
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

func tctxWithCache() *tools.ToolContext {
	return &tools.ToolContext{
		FileState: tools.NewFileStateCache(100, 25*1024*1024),
	}
}

// ─── Interface ───────────────────────────────────────────────────────────────

func TestFileReadTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestFileReadTool_Name(t *testing.T) {
	if got := (&Tool{}).Name(); got != "Read" {
		t.Errorf("Name() = %q, want %q", got, "Read")
	}
}

func TestFileReadTool_IsConcurrencySafe(t *testing.T) {
	if !(&Tool{}).IsConcurrencySafe(nil) {
		t.Error("FileReadTool should always be concurrency-safe")
	}
}

func TestFileReadTool_IsReadOnly(t *testing.T) {
	if !(&Tool{}).IsReadOnly(nil) {
		t.Error("FileReadTool should always be read-only")
	}
}

// ─── ValidateInput ────────────────────────────────────────────────────────────

func TestFileReadTool_ValidateInput_MissingPath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{})); err == nil {
		t.Error("expected error for missing file_path")
	}
}

func TestFileReadTool_ValidateInput_EmptyPath(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": ""})); err == nil {
		t.Error("expected error for empty file_path")
	}
}

func TestFileReadTool_ValidateInput_Valid(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"file_path": "/tmp/foo.txt"})); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFileReadTool_ValidateInput_InvalidOffset(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"file_path": "/tmp/foo.txt",
		"offset":    0,
	})); err == nil {
		t.Error("expected error for offset=0 (must be >= 1)")
	}
}

func TestFileReadTool_ValidateInput_InvalidLimit(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{
		"file_path": "/tmp/foo.txt",
		"limit":     0,
	})); err == nil {
		t.Error("expected error for limit=0 (must be >= 1)")
	}
}

// ─── Execute: basic reading ───────────────────────────────────────────────────

func TestFileReadTool_ReadsFile(t *testing.T) {
	path := writeTempFile(t, "alpha\nbeta\ngamma\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected no error result, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "alpha") {
		t.Errorf("content should contain 'alpha', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "beta") {
		t.Errorf("content should contain 'beta', got: %s", result.Content)
	}
}

func TestFileReadTool_LineNumberFormat(t *testing.T) {
	path := writeTempFile(t, "first\nsecond\nthird\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctxWithCache())

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}

	// Expect cat -n format: right-justified line number + tab + content
	if !strings.Contains(result.Content, "1\tfirst") {
		t.Errorf("expected cat -n line number format, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "2\tsecond") {
		t.Errorf("expected line 2 in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "3\tthird") {
		t.Errorf("expected line 3 in output, got:\n%s", result.Content)
	}
}

func TestFileReadTool_Offset_SkipsLines(t *testing.T) {
	// 5-line file; read from line 3
	path := writeTempFile(t, "one\ntwo\nthree\nfour\nfive\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"offset":    3,
	}), tctxWithCache())

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}
	if strings.Contains(result.Content, "one") || strings.Contains(result.Content, "two") {
		t.Errorf("offset=3 should skip lines 1 and 2, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "three") {
		t.Errorf("expected line 'three' (line 3) in output, got:\n%s", result.Content)
	}
	// Line numbers should reflect position in file, not position in output
	if !strings.Contains(result.Content, "3\tthree") {
		t.Errorf("line numbers should be file-relative, got:\n%s", result.Content)
	}
}

func TestFileReadTool_Limit_CapsLines(t *testing.T) {
	path := writeTempFile(t, "a\nb\nc\nd\ne\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"limit":     2,
	}), tctxWithCache())

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}
	if !strings.Contains(result.Content, "a") || !strings.Contains(result.Content, "b") {
		t.Errorf("expected first 2 lines in output, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "c") || strings.Contains(result.Content, "d") {
		t.Errorf("limit=2 should not include lines beyond 2nd, got:\n%s", result.Content)
	}
}

func TestFileReadTool_DefaultLimit_CapsAt2000Lines(t *testing.T) {
	// Create a file with 2500 lines
	var sb strings.Builder
	for i := 1; i <= 2500; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	path := writeTempFile(t, sb.String())

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctxWithCache())

	if err != nil || result.IsError {
		t.Fatalf("unexpected error: %v / %s", err, result.Content)
	}
	// Should contain line 2000 but not line 2001
	if !strings.Contains(result.Content, "line 2000") {
		t.Error("should contain line 2000")
	}
	if strings.Contains(result.Content, "line 2001") {
		t.Error("should not contain line 2001 (beyond default limit)")
	}
}

// ─── Execute: error cases ─────────────────────────────────────────────────────

func TestFileReadTool_RelativePath_ReturnsError(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": "relative/path.txt",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for relative path, got content: %s", result.Content)
	}
}

func TestFileReadTool_BinaryExtension_ReturnsError(t *testing.T) {
	// Use a .exe extension — doesn't need to exist on disk
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": "/tmp/fake_binary.exe",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for binary file, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "binary") {
		t.Errorf("error message should mention binary, got: %s", result.Content)
	}
}

func TestFileReadTool_FileNotFound_ReturnsError(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": "/tmp/no_such_file_exists_forge_test.txt",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for missing file, got: %s", result.Content)
	}
}

func TestFileReadTool_EmptyFile(t *testing.T) {
	path := writeTempFile(t, "")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty file should return a warning, not an error
	if result.IsError {
		t.Errorf("empty file should not be an error result, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "empty") {
		t.Errorf("should warn about empty file, got: %s", result.Content)
	}
}

func TestFileReadTool_OffsetBeyondFile(t *testing.T) {
	path := writeTempFile(t, "only one line\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"offset":    100,
	}), tctxWithCache())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("offset beyond EOF should not be an error result, got: %s", result.Content)
	}
	// Should warn that offset is beyond file length
	if !strings.Contains(strings.ToLower(result.Content), "shorter") &&
		!strings.Contains(strings.ToLower(result.Content), "offset") {
		t.Errorf("should mention offset/shorter, got: %s", result.Content)
	}
}

// ─── Execute: FileStateCache ──────────────────────────────────────────────────

func TestFileReadTool_UpdatesFileStateCache(t *testing.T) {
	content := "hello\nworld\n"
	path := writeTempFile(t, content)
	tctx := tctxWithCache()

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("expected FileStateCache to be updated after read")
	}
	if !strings.Contains(state.Content, "hello") {
		t.Errorf("cached content should contain 'hello', got: %q", state.Content)
	}
	if state.Timestamp == 0 {
		t.Error("cached timestamp should be non-zero")
	}
	if state.Offset == nil || *state.Offset != 1 {
		t.Errorf("cached offset should be 1 (default), got: %v", state.Offset)
	}
}

func TestFileReadTool_CacheStoresOffsetAndLimit(t *testing.T) {
	path := writeTempFile(t, "a\nb\nc\nd\ne\n")
	tctx := tctxWithCache()

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
		"offset":    2,
		"limit":     3,
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, ok := tctx.FileState.Get(path)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if state.Offset == nil || *state.Offset != 2 {
		t.Errorf("cached offset = %v, want 2", state.Offset)
	}
	if state.Limit == nil || *state.Limit != 3 {
		t.Errorf("cached limit = %v, want 3", state.Limit)
	}
}

func TestFileReadTool_NilTctx_DoesNotCrash(t *testing.T) {
	path := writeTempFile(t, "hello\n")

	result, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), nil)

	if err != nil {
		t.Fatalf("unexpected error with nil tctx: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}
}

func TestFileReadTool_CacheUsesAbsolutePath(t *testing.T) {
	path := writeTempFile(t, "content\n")
	tctx := tctxWithCache()

	_, err := (&Tool{}).Execute(context.Background(), mustJSON(t, map[string]any{
		"file_path": path,
	}), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be accessible via the absolute path
	absPath, _ := filepath.Abs(path)
	if _, ok := tctx.FileState.Get(absPath); !ok {
		t.Errorf("cache should be keyed by absolute path %s", absPath)
	}
}

// ─── CheckPermissions ────────────────────────────────────────────────────────

func TestFileReadTool_CheckPermissions_NormalFile_Allow(t *testing.T) {
	decision, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{
		"file_path": "/tmp/any.txt",
	}), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != "allow" {
		t.Errorf("expected Behavior=allow, got %q", decision.Behavior)
	}
}

// ── Sensitive file protection (SEC-3) ────────────────────────────────────────

func TestFileReadTool_CheckPermissions_DotEnv_PermAsk(t *testing.T) {
	for _, p := range []string{"/project/.env", "/project/.env.local", "/project/.env.production"} {
		d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if d.Behavior != "ask" {
			t.Errorf("%s: expected PermAsk, got %q", p, d.Behavior)
		}
	}
}

func TestFileReadTool_CheckPermissions_PemKey_PermAsk(t *testing.T) {
	for _, p := range []string{"/certs/server.pem", "/keys/id_rsa.key"} {
		d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if d.Behavior != "ask" {
			t.Errorf("%s: expected PermAsk, got %q", p, d.Behavior)
		}
	}
}

func TestFileReadTool_CheckPermissions_Credentials_PermAsk(t *testing.T) {
	for _, p := range []string{"/home/user/.aws/credentials", "/app/db_credentials.json"} {
		d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if d.Behavior != "ask" {
			t.Errorf("%s: expected PermAsk, got %q", p, d.Behavior)
		}
	}
}

func TestFileReadTool_CheckPermissions_SshDir_PermAsk(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	for _, p := range []string{
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "known_hosts"),
	} {
		d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if d.Behavior != "ask" {
			t.Errorf("%s: expected PermAsk, got %q", p, d.Behavior)
		}
	}
}

func TestFileReadTool_CheckPermissions_AwsDir_PermAsk(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	p := filepath.Join(home, ".aws", "credentials")
	d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Behavior != "ask" {
		t.Errorf("expected PermAsk for %s, got %q", p, d.Behavior)
	}
}

func TestFileReadTool_CheckPermissions_SecretFile_PermAsk(t *testing.T) {
	p := "/app/config/api_secret.json"
	d, err := (&Tool{}).CheckPermissions(mustJSON(t, map[string]any{"file_path": p}), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Behavior != "ask" {
		t.Errorf("expected PermAsk for %s, got %q", p, d.Behavior)
	}
}

func TestFileReadTool_CheckPermissions_NilInput_Allow(t *testing.T) {
	d, err := (&Tool{}).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Behavior != "allow" {
		t.Errorf("nil input should default to allow, got %q", d.Behavior)
	}
}

// ── Path sanitization (security) ─────────────────────────────────────────────

func TestFileRead_RelativePath_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"relative/path.txt"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative path should be rejected")
	}
}

func TestFileRead_DotDotRelative_Rejected(t *testing.T) {
	tool := &Tool{}
	input := json.RawMessage(`{"file_path":"../../../etc/passwd"}`)
	result, _ := tool.Execute(context.Background(), input, nil)
	if !result.IsError {
		t.Error("relative dotdot path should be rejected")
	}
}

func TestFileRead_AbsWithDotDot_NormalizesAndReads(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	_ = os.Mkdir(sub, 0755)
	target := filepath.Join(dir, "target.txt")
	_ = os.WriteFile(target, []byte("hello world"), 0644)

	// Path with ".." that resolves to target.txt
	traversalPath := filepath.Join(sub, "..", "target.txt")

	tool := &Tool{}
	input, _ := json.Marshal(map[string]string{"file_path": traversalPath})
	result, _ := tool.Execute(context.Background(), json.RawMessage(input), nil)
	if result.IsError {
		t.Errorf("normalized abs dotdot path should succeed, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello world") {
		t.Errorf("expected file content, got: %s", result.Content)
	}
}
