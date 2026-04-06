package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Mock backend for testing ---

type mockBackend struct {
	calls    []string
	results  map[string]string
	cookies  []Cookie
	console  []ConsoleMessage
	failOn   map[string]error
	viewport [2]int
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		results: make(map[string]string),
		failOn:  make(map[string]error),
	}
}

func (m *mockBackend) record(action string, args ...string) {
	m.calls = append(m.calls, action+" "+strings.Join(args, " "))
}

func (m *mockBackend) err(action string) error {
	if e, ok := m.failOn[action]; ok {
		return e
	}
	return nil
}

func (m *mockBackend) Open(_ context.Context, url string) error {
	m.record("open", url)
	return m.err("open")
}

func (m *mockBackend) Snapshot(_ context.Context, opts SnapshotOpts) (string, error) {
	m.record("snapshot")
	if e := m.err("snapshot"); e != nil {
		return "", e
	}
	r := m.results["snapshot"]
	if r == "" {
		r = "- body\n  - h1\n    Welcome"
	}
	return r, nil
}

func (m *mockBackend) Click(_ context.Context, selector string) error {
	m.record("click", selector)
	return m.err("click")
}

func (m *mockBackend) Type(_ context.Context, selector, text string) error {
	m.record("type", selector, text)
	return m.err("type")
}

func (m *mockBackend) Fill(_ context.Context, selector, text string) error {
	m.record("fill", selector, text)
	return m.err("fill")
}

func (m *mockBackend) Press(_ context.Context, key string) error {
	m.record("press", key)
	return m.err("press")
}

func (m *mockBackend) Wait(_ context.Context, target string, _ time.Duration) error {
	m.record("wait", target)
	return m.err("wait")
}

func (m *mockBackend) Get(_ context.Context, what, selector, attrName string) (string, error) {
	m.record("get", what, selector, attrName)
	if e := m.err("get"); e != nil {
		return "", e
	}
	key := "get:" + what
	if r, ok := m.results[key]; ok {
		return r, nil
	}
	return "mock-value", nil
}

func (m *mockBackend) Screenshot(_ context.Context, path string) error {
	m.record("screenshot", path)
	return m.err("screenshot")
}

func (m *mockBackend) Scroll(_ context.Context, direction string, pixels int) error {
	m.record("scroll", direction, fmt.Sprintf("%d", pixels))
	return m.err("scroll")
}

func (m *mockBackend) Navigate(_ context.Context, action string) error {
	m.record("navigate", action)
	return m.err("navigate")
}

func (m *mockBackend) Close(_ context.Context) error {
	m.record("close")
	return m.err("close")
}

func (m *mockBackend) Eval(_ context.Context, js string) (string, error) {
	m.record("eval", js)
	if e := m.err("eval"); e != nil {
		return "", e
	}
	if r, ok := m.results["eval"]; ok {
		return r, nil
	}
	return "eval-result", nil
}

func (m *mockBackend) Upload(_ context.Context, selector, filePath string) error {
	m.record("upload", selector, filePath)
	return m.err("upload")
}

func (m *mockBackend) SetViewport(_ context.Context, width, height int) error {
	m.record("viewport", fmt.Sprintf("%d", width), fmt.Sprintf("%d", height))
	m.viewport = [2]int{width, height}
	return m.err("viewport")
}

func (m *mockBackend) PDF(_ context.Context, path string) error {
	m.record("pdf", path)
	return m.err("pdf")
}

func (m *mockBackend) Cookies(_ context.Context) ([]Cookie, error) {
	m.record("cookies")
	if e := m.err("cookies"); e != nil {
		return nil, e
	}
	return m.cookies, nil
}

func (m *mockBackend) SetCookies(_ context.Context, cookies []Cookie) error {
	m.record("setCookies")
	m.cookies = append(m.cookies, cookies...)
	return m.err("setCookies")
}

func (m *mockBackend) ConsoleMessages() []ConsoleMessage {
	return m.console
}

// --- Verify both backends satisfy the interface at compile time ---

var _ Backend = (*ChromedpBackend)(nil)
var _ Backend = (*CLIBackend)(nil)

// --- Tool tests with mock backend ---

func newToolWithMock(mb *mockBackend) *Tool {
	return &Tool{
		NewBackend: func(_ string) (Backend, error) {
			return mb, nil
		},
	}
}

func TestExecute_Eval(t *testing.T) {
	mb := newMockBackend()
	mb.results["eval"] = "42"
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"eval","js":"1+1"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if result.Content != "42" {
		t.Fatalf("got %q, want %q", result.Content, "42")
	}
	if len(mb.calls) != 1 || !strings.HasPrefix(mb.calls[0], "eval") {
		t.Fatalf("unexpected calls: %v", mb.calls)
	}
}

func TestExecute_Viewport(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"viewport","width":1920,"height":1080}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1920x1080") {
		t.Fatalf("unexpected content: %s", result.Content)
	}
	if mb.viewport != [2]int{1920, 1080} {
		t.Fatalf("viewport not set: %v", mb.viewport)
	}
}

func TestExecute_CookiesGet(t *testing.T) {
	mb := newMockBackend()
	mb.cookies = []Cookie{
		{Name: "session", Value: "abc123", Domain: "example.com"},
	}
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"cookies"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "session") || !strings.Contains(result.Content, "abc123") {
		t.Fatalf("cookie data missing from output: %s", result.Content)
	}
}

func TestExecute_CookiesSet(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	input := `{"action":"cookies","cookies":[{"name":"token","value":"xyz"}]}`
	result, err := tool.Execute(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Set 1 cookie") {
		t.Fatalf("unexpected content: %s", result.Content)
	}
	if len(mb.cookies) != 1 || mb.cookies[0].Name != "token" {
		t.Fatalf("cookie not set: %v", mb.cookies)
	}
}

func TestExecute_Console(t *testing.T) {
	mb := newMockBackend()
	mb.console = []ConsoleMessage{
		{Level: "log", Text: "hello world"},
		{Level: "error", Text: "something broke"},
	}
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"console"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello world") || !strings.Contains(result.Content, "something broke") {
		t.Fatalf("console messages missing: %s", result.Content)
	}
}

func TestExecute_ConsoleEmpty(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"console"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "No console messages captured." {
		t.Fatalf("unexpected content: %s", result.Content)
	}
}

func TestExecute_PDF(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"pdf","path":"/tmp/out.pdf"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "/tmp/out.pdf") {
		t.Fatalf("unexpected content: %s", result.Content)
	}
}

func TestExecute_Upload(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"upload","selector":"#file","file_path":"/tmp/test.txt"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if len(mb.calls) != 1 || !strings.Contains(mb.calls[0], "upload") {
		t.Fatalf("upload not called: %v", mb.calls)
	}
}

func TestExecute_BackendError(t *testing.T) {
	mb := newMockBackend()
	mb.failOn["eval"] = fmt.Errorf("js error: syntax error")
	tool := newToolWithMock(mb)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"eval","js":"invalid("}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}
	if !strings.Contains(result.Content, "js error") {
		t.Fatalf("unexpected error content: %s", result.Content)
	}
}

func TestExecute_OpenClick_BackendCalls(t *testing.T) {
	mb := newMockBackend()
	tool := newToolWithMock(mb)

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"open","url":"https://example.com"}`), nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"action":"click","selector":"#btn"}`), nil)
	if err != nil {
		t.Fatalf("click: %v", err)
	}

	if len(mb.calls) < 2 {
		t.Fatalf("expected 2+ calls, got %d: %v", len(mb.calls), mb.calls)
	}
	if !strings.HasPrefix(mb.calls[0], "open") {
		t.Fatalf("first call should be open, got: %s", mb.calls[0])
	}
	if !strings.HasPrefix(mb.calls[1], "click") {
		t.Fatalf("second call should be click, got: %s", mb.calls[1])
	}
}

func TestValidateInput_NewActions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"eval ok", `{"action":"eval","js":"document.title"}`, false},
		{"eval no js", `{"action":"eval"}`, true},
		{"upload ok", `{"action":"upload","selector":"#f","file_path":"/tmp/x"}`, false},
		{"upload no selector", `{"action":"upload","file_path":"/tmp/x"}`, true},
		{"upload no path", `{"action":"upload","selector":"#f"}`, true},
		{"viewport ok", `{"action":"viewport","width":800,"height":600}`, false},
		{"viewport no height", `{"action":"viewport","width":800}`, true},
		{"viewport zero", `{"action":"viewport","width":0,"height":600}`, true},
		{"pdf ok", `{"action":"pdf","path":"/tmp/out.pdf"}`, false},
		{"pdf no path", `{"action":"pdf"}`, true},
		{"cookies get", `{"action":"cookies"}`, false},
		{"cookies set", `{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`, false},
		{"console", `{"action":"console"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newTool().ValidateInput(json.RawMessage(tt.input))
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckPermissions_NewActions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // "allow" or "ask"
	}{
		{"console", `{"action":"console"}`, "allow"},
		{"cookies get", `{"action":"cookies"}`, "allow"},
		{"cookies set", `{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`, "ask"},
		{"eval", `{"action":"eval","js":"1+1"}`, "ask"},
		{"upload", `{"action":"upload","selector":"#f","file_path":"/tmp/x"}`, "ask"},
		{"viewport", `{"action":"viewport","width":800,"height":600}`, "ask"},
		{"pdf", `{"action":"pdf","path":"/tmp/out.pdf"}`, "ask"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec, err := newTool().CheckPermissions(json.RawMessage(tt.input), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := string(dec.Behavior)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsReadOnly_NewActions(t *testing.T) {
	tool := newTool()
	if !tool.IsReadOnly(json.RawMessage(`{"action":"console"}`)) {
		t.Fatal("console should be read-only")
	}
	if !tool.IsReadOnly(json.RawMessage(`{"action":"cookies"}`)) {
		t.Fatal("cookies get should be read-only")
	}
	if tool.IsReadOnly(json.RawMessage(`{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`)) {
		t.Fatal("cookies set should not be read-only")
	}
	if tool.IsReadOnly(json.RawMessage(`{"action":"eval","js":"1"}`)) {
		t.Fatal("eval should not be read-only")
	}
}

// --- Cookie persistence tests ---

func TestCookieSaveLoadRoundtrip(t *testing.T) {
	// Use a temp dir so we don't touch the user's home.
	tmpDir := t.TempDir()
	session := "test-roundtrip"
	path := filepath.Join(tmpDir, fmt.Sprintf("cookies-%s.json", session))

	cookies := []Cookie{
		{Name: "session_id", Value: "s3cr3t", Domain: ".example.com", Path: "/", Secure: true},
		{Name: "prefs", Value: "dark_mode=1", Domain: ".example.com", Path: "/"},
	}

	// Write cookies manually.
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Read them back.
	var loaded []Cookie
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(loaded))
	}
	if loaded[0].Name != "session_id" || loaded[0].Value != "s3cr3t" {
		t.Fatalf("first cookie mismatch: %+v", loaded[0])
	}
	if !loaded[0].Secure {
		t.Fatal("expected Secure=true")
	}
	if loaded[1].Name != "prefs" {
		t.Fatalf("second cookie mismatch: %+v", loaded[1])
	}
}

// --- CLI Backend unsupported tests ---

func TestCLIBackend_UnsupportedActions(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, _ ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
	}
	cli := NewCLIBackend("agent-browser", "test-session", runner)

	ctx := context.Background()

	if _, err := cli.Eval(ctx, "1+1"); err == nil {
		t.Fatal("Eval should return error for CLI backend")
	}
	if err := cli.Upload(ctx, "#f", "/tmp/x"); err == nil {
		t.Fatal("Upload should return error for CLI backend")
	}
	if err := cli.SetViewport(ctx, 800, 600); err == nil {
		t.Fatal("SetViewport should return error for CLI backend")
	}
	if err := cli.PDF(ctx, "/tmp/out.pdf"); err == nil {
		t.Fatal("PDF should return error for CLI backend")
	}
	if _, err := cli.Cookies(ctx); err == nil {
		t.Fatal("Cookies should return error for CLI backend")
	}
	if err := cli.SetCookies(ctx, []Cookie{{Name: "a", Value: "b"}}); err == nil {
		t.Fatal("SetCookies should return error for CLI backend")
	}
	if msgs := cli.ConsoleMessages(); msgs != nil {
		t.Fatalf("expected nil console messages, got %v", msgs)
	}
}

func TestCLIBackend_BasicActions(t *testing.T) {
	runner := &stubRunner{
		run: func(_ string, args ...string) ([]byte, error) {
			return []byte("cli-output"), nil
		},
	}
	cli := NewCLIBackend("agent-browser", "sess1", runner)
	ctx := context.Background()

	if err := cli.Open(ctx, "https://example.com"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := cli.Snapshot(ctx, SnapshotOpts{Interactive: true, Compact: true}); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := cli.Click(ctx, "#btn"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if err := cli.Type(ctx, "#input", "hello"); err != nil {
		t.Fatalf("Type: %v", err)
	}
	if err := cli.Fill(ctx, "#input", "hello"); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if err := cli.Press(ctx, "Enter"); err != nil {
		t.Fatalf("Press: %v", err)
	}
	if err := cli.Wait(ctx, "#el", 0); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if _, err := cli.Get(ctx, "title", "", ""); err != nil {
		t.Fatalf("Get title: %v", err)
	}
	if _, err := cli.Get(ctx, "url", "", ""); err != nil {
		t.Fatalf("Get url: %v", err)
	}
	if _, err := cli.Get(ctx, "text", "#el", ""); err != nil {
		t.Fatalf("Get text: %v", err)
	}
	if _, err := cli.Get(ctx, "attr", "#el", "href"); err != nil {
		t.Fatalf("Get attr: %v", err)
	}
	if err := cli.Screenshot(ctx, "/tmp/shot.png"); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if err := cli.Scroll(ctx, "down", 300); err != nil {
		t.Fatalf("Scroll: %v", err)
	}
	if err := cli.Navigate(ctx, "back"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := cli.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify we made the right number of calls.
	if len(runner.calls) != 15 {
		t.Fatalf("expected 15 CLI calls, got %d", len(runner.calls))
	}
}

// --- Backend selection tests ---

func TestBackendSelection_FallbackToCLI(t *testing.T) {
	// When NewBackend is nil and no Chrome, should create a CLIBackend.
	tool := &Tool{Command: "agent-browser"}
	// We can't easily test chromeAvailable() here, but we can verify
	// the NewBackend override works.
	var created bool
	tool.NewBackend = func(session string) (Backend, error) {
		created = true
		return newMockBackend(), nil
	}

	_, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("NewBackend was not called")
	}
}

func TestBackendSelection_ReusesSameSession(t *testing.T) {
	createCount := 0
	mb := newMockBackend()
	tool := &Tool{
		NewBackend: func(_ string) (Backend, error) {
			createCount++
			return mb, nil
		},
	}

	// Two calls with same implicit session should reuse backend.
	_, _ = tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`), nil)
	_, _ = tool.Execute(context.Background(), json.RawMessage(`{"action":"snapshot"}`), nil)

	if createCount != 1 {
		t.Fatalf("expected 1 backend creation, got %d", createCount)
	}
}

// --- Smart wait helper tests ---

func TestIsDurationTarget(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1000", true},
		{"500ms", true},
		{"2s", true},
		{"#element", false},
		{".class", false},
		{"div", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isDurationTarget(tt.input)
		if got != tt.want {
			t.Errorf("isDurationTarget(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"1000", 1000 * time.Millisecond},
		{"500ms", 500 * time.Millisecond},
		{"2s", 2 * time.Second},
	}
	for _, tt := range tests {
		got, err := parseDuration(tt.input)
		if err != nil {
			t.Errorf("parseDuration(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsStaleError(t *testing.T) {
	if isStaleError(nil) {
		t.Fatal("nil should not be stale")
	}
	if !isStaleError(fmt.Errorf("stale element reference")) {
		t.Fatal("expected stale")
	}
	if !isStaleError(fmt.Errorf("node not found")) {
		t.Fatal("expected stale for 'node not found'")
	}
	if isStaleError(fmt.Errorf("timeout")) {
		t.Fatal("timeout should not be stale")
	}
}

// --- Security: cookie session name path traversal ---

func TestSanitizeSession_Valid(t *testing.T) {
	for _, s := range []string{"forge-browser-abc123", "test-session", ""} {
		if err := sanitizeSession(s); err != nil {
			t.Errorf("sanitizeSession(%q) = %v, want nil", s, err)
		}
	}
}

func TestSanitizeSession_Rejects(t *testing.T) {
	for _, s := range []string{
		"../../../tmp/evil",
		"foo/bar",
		"foo\\bar",
		"session..hack",
	} {
		if err := sanitizeSession(s); err == nil {
			t.Errorf("sanitizeSession(%q) = nil, want error", s)
		}
	}
}

func TestCookiePath_RejectsTraversal(t *testing.T) {
	_, err := cookiePath("../../etc/passwd")
	if err == nil {
		t.Fatal("cookiePath should reject path traversal in session name")
	}
}

func TestSaveCookies_RejectsTraversal(t *testing.T) {
	cookies := []Cookie{{Name: "a", Value: "b"}}
	err := saveCookies("../../etc/evil", cookies)
	if err == nil {
		t.Fatal("saveCookies should reject path traversal in session name")
	}
}

func TestLoadCookies_RejectsTraversal(t *testing.T) {
	_, err := loadCookies("../../etc/evil")
	if err == nil {
		t.Fatal("loadCookies should reject path traversal in session name")
	}
}
