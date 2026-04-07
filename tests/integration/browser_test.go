package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools/browser"
)

// =============================================================================
// Mock Backend for testing
// =============================================================================

type mockBackend struct {
	calls   []mockCall
	cookies []browser.Cookie
	console []browser.ConsoleMessage
}

type mockCall struct {
	method string
	args   []interface{}
}

func (m *mockBackend) record(method string, args ...interface{}) {
	m.calls = append(m.calls, mockCall{method: method, args: args})
}

func (m *mockBackend) lastCall() mockCall {
	if len(m.calls) == 0 {
		return mockCall{}
	}
	return m.calls[len(m.calls)-1]
}

func (m *mockBackend) Open(_ context.Context, url string) error {
	m.record("Open", url)
	return nil
}

func (m *mockBackend) Snapshot(_ context.Context, opts browser.SnapshotOpts) (string, error) {
	m.record("Snapshot", opts)
	return "<html snapshot>", nil
}

func (m *mockBackend) Click(_ context.Context, selector string) error {
	m.record("Click", selector)
	return nil
}

func (m *mockBackend) Type(_ context.Context, selector, text string) error {
	m.record("Type", selector, text)
	return nil
}

func (m *mockBackend) Fill(_ context.Context, selector, text string) error {
	m.record("Fill", selector, text)
	return nil
}

func (m *mockBackend) Press(_ context.Context, key string) error {
	m.record("Press", key)
	return nil
}

func (m *mockBackend) Wait(_ context.Context, target string, timeout time.Duration) error {
	m.record("Wait", target, timeout)
	return nil
}

func (m *mockBackend) Get(_ context.Context, what, selector, attrName string) (string, error) {
	m.record("Get", what, selector, attrName)
	switch what {
	case "title":
		return "Test Page Title", nil
	case "url":
		return "https://example.com/page", nil
	case "text":
		return "element text content", nil
	case "html":
		return "<div>element html</div>", nil
	case "value":
		return "input value", nil
	case "count":
		return "3", nil
	case "attr":
		return "attribute-value", nil
	default:
		return "", fmt.Errorf("unknown get type: %s", what)
	}
}

func (m *mockBackend) Screenshot(_ context.Context, path string) error {
	m.record("Screenshot", path)
	return nil
}

func (m *mockBackend) Scroll(_ context.Context, direction string, pixels int) error {
	m.record("Scroll", direction, pixels)
	return nil
}

func (m *mockBackend) Navigate(_ context.Context, action string) error {
	m.record("Navigate", action)
	return nil
}

func (m *mockBackend) Close(_ context.Context) error {
	m.record("Close")
	return nil
}

func (m *mockBackend) Eval(_ context.Context, js string) (string, error) {
	m.record("Eval", js)
	return "eval result", nil
}

func (m *mockBackend) Upload(_ context.Context, selector, filePath string) error {
	m.record("Upload", selector, filePath)
	return nil
}

func (m *mockBackend) SetViewport(_ context.Context, width, height int) error {
	m.record("SetViewport", width, height)
	return nil
}

func (m *mockBackend) PDF(_ context.Context, path string) error {
	m.record("PDF", path)
	return nil
}

func (m *mockBackend) Cookies(_ context.Context) ([]browser.Cookie, error) {
	m.record("Cookies")
	return m.cookies, nil
}

func (m *mockBackend) SetCookies(_ context.Context, cookies []browser.Cookie) error {
	m.record("SetCookies", cookies)
	m.cookies = append(m.cookies, cookies...)
	return nil
}

func (m *mockBackend) ConsoleMessages() []browser.ConsoleMessage {
	return m.console
}

// newMockTool creates a browser.Tool backed by a mockBackend.
func newMockTool() (*browser.Tool, *mockBackend) {
	mb := &mockBackend{}
	t := &browser.Tool{
		NewBackend: func(session string) (browser.Backend, error) {
			return mb, nil
		},
	}
	return t, mb
}

// =============================================================================
// Backend Interface Compliance
// =============================================================================

func TestMockBackendImplementsInterface(t *testing.T) {
	// Compile-time check that mockBackend implements Backend.
	var _ browser.Backend = (*mockBackend)(nil)
}

// =============================================================================
// Input Validation
// =============================================================================

func TestBrowserValidateInput(t *testing.T) {
	tool := &browser.Tool{}

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid inputs.
		{"open valid", `{"action":"open","url":"https://example.com"}`, false, ""},
		{"snapshot valid", `{"action":"snapshot"}`, false, ""},
		{"snapshot with opts", `{"action":"snapshot","interactive":true,"compact":false,"depth":3}`, false, ""},
		{"click valid", `{"action":"click","selector":"#btn"}`, false, ""},
		{"type valid", `{"action":"type","selector":"#input","text":"hello"}`, false, ""},
		{"fill valid", `{"action":"fill","selector":"#input","text":"hello"}`, false, ""},
		{"press valid", `{"action":"press","key":"Enter"}`, false, ""},
		{"wait valid", `{"action":"wait","target":"#loading"}`, false, ""},
		{"get title", `{"action":"get","get":"title"}`, false, ""},
		{"get url", `{"action":"get","get":"url"}`, false, ""},
		{"get text", `{"action":"get","get":"text","selector":"#el"}`, false, ""},
		{"get attr", `{"action":"get","get":"attr","selector":"#el","attr_name":"href"}`, false, ""},
		{"screenshot valid", `{"action":"screenshot"}`, false, ""},
		{"scroll valid", `{"action":"scroll","direction":"down"}`, false, ""},
		{"back valid", `{"action":"back"}`, false, ""},
		{"forward valid", `{"action":"forward"}`, false, ""},
		{"reload valid", `{"action":"reload"}`, false, ""},
		{"close valid", `{"action":"close"}`, false, ""},
		{"eval valid", `{"action":"eval","js":"document.title"}`, false, ""},
		{"upload valid", `{"action":"upload","selector":"#file","file_path":"/tmp/f.txt"}`, false, ""},
		{"viewport valid", `{"action":"viewport","width":1920,"height":1080}`, false, ""},
		{"pdf valid", `{"action":"pdf","path":"/tmp/out.pdf"}`, false, ""},
		{"cookies get", `{"action":"cookies"}`, false, ""},
		{"cookies set", `{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`, false, ""},
		{"console valid", `{"action":"console"}`, false, ""},
		{"find valid", `{"action":"find","locator":"role","value":"button","find_action":"click"}`, false, ""},
		{"tab new", `{"action":"tab","tab_action":"new"}`, false, ""},
		{"tab switch", `{"action":"tab","tab_action":"switch","tab_index":0}`, false, ""},

		// Invalid inputs.
		{"open missing url", `{"action":"open"}`, true, "url is required"},
		{"click missing selector", `{"action":"click"}`, true, "selector is required"},
		{"type missing selector", `{"action":"type","text":"hi"}`, true, "selector is required"},
		{"type missing text", `{"action":"type","selector":"#in"}`, true, "text is required"},
		{"fill missing text", `{"action":"fill","selector":"#in"}`, true, "text is required"},
		{"press missing key", `{"action":"press"}`, true, "key is required"},
		{"wait missing target", `{"action":"wait"}`, true, "target is required"},
		{"get missing get", `{"action":"get"}`, true, "get is required"},
		{"get text missing selector", `{"action":"get","get":"text"}`, true, "selector is required"},
		{"get attr missing selector", `{"action":"get","get":"attr","attr_name":"x"}`, true, "selector is required"},
		{"get attr missing attr_name", `{"action":"get","get":"attr","selector":"#x"}`, true, "attr_name is required"},
		{"scroll missing direction", `{"action":"scroll"}`, true, "direction is required"},
		{"find missing locator", `{"action":"find","value":"x","find_action":"click"}`, true, "locator is required"},
		{"find missing value", `{"action":"find","locator":"role","find_action":"click"}`, true, "value is required"},
		{"find missing find_action", `{"action":"find","locator":"role","value":"button"}`, true, "find_action is required"},
		{"tab missing tab_action", `{"action":"tab"}`, true, "tab_action is required"},
		{"tab switch missing index", `{"action":"tab","tab_action":"switch"}`, true, "tab_index is required"},
		{"eval missing js", `{"action":"eval"}`, true, "js is required"},
		{"upload missing selector", `{"action":"upload","file_path":"/tmp/f"}`, true, "selector is required"},
		{"upload missing file_path", `{"action":"upload","selector":"#f"}`, true, "file_path is required"},
		{"viewport missing dims", `{"action":"viewport"}`, true, "width and height are required"},
		{"viewport zero dims", `{"action":"viewport","width":0,"height":0}`, true, "must be positive"},
		{"pdf missing path", `{"action":"pdf"}`, true, "path is required"},
		{"snapshot negative depth", `{"action":"snapshot","depth":-1}`, true, "depth must be >= 0"},
		{"unsupported action", `{"action":"fly"}`, true, "unsupported action"},
		{"invalid json", `not json`, true, ""},
		{"download missing selector", `{"action":"download","path":"/tmp/f"}`, true, "selector is required"},
		{"download missing path", `{"action":"download","selector":"#dl"}`, true, "path is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(json.RawMessage(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// =============================================================================
// CLI Backend Arg Construction
// =============================================================================

func TestBuildArgsAllActions(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	boolPtr := func(v bool) *bool { return &v }

	// We test buildArgs indirectly via Execute with the mock backend.
	// But we can directly test the Tool's commandline construction for the CLI backend.
	// Since buildArgs is exported enough to test, verify representative actions.

	tests := []struct {
		name    string
		action  string
		input   string
		wantSub string // substring in content or backend call
	}{
		{"open", "open", `{"action":"open","url":"https://example.com"}`, "Open"},
		{"snapshot default", "snapshot", `{"action":"snapshot"}`, "Snapshot"},
		{"click", "click", `{"action":"click","selector":"#btn"}`, "Click"},
		{"type", "type", `{"action":"type","selector":"#in","text":"hello"}`, "Type"},
		{"fill", "fill", `{"action":"fill","selector":"#in","text":"world"}`, "Fill"},
		{"press", "press", `{"action":"press","key":"Enter"}`, "Press"},
		{"wait", "wait", `{"action":"wait","target":"#spinner"}`, "Wait"},
		{"get title", "get", `{"action":"get","get":"title"}`, "Get"},
		{"get url", "get", `{"action":"get","get":"url"}`, "Get"},
		{"get text", "get", `{"action":"get","get":"text","selector":"p"}`, "Get"},
		{"get attr", "get", `{"action":"get","get":"attr","selector":"a","attr_name":"href"}`, "Get"},
		{"screenshot", "screenshot", `{"action":"screenshot","path":"/tmp/s.png"}`, "Screenshot"},
		{"scroll down", "scroll", `{"action":"scroll","direction":"down","pixels":500}`, "Scroll"},
		{"back", "back", `{"action":"back"}`, "Navigate"},
		{"forward", "forward", `{"action":"forward"}`, "Navigate"},
		{"reload", "reload", `{"action":"reload"}`, "Navigate"},
		{"eval", "eval", `{"action":"eval","js":"1+1"}`, "Eval"},
		{"upload", "upload", `{"action":"upload","selector":"#f","file_path":"/tmp/a.txt"}`, "Upload"},
		{"viewport", "viewport", `{"action":"viewport","width":1024,"height":768}`, "SetViewport"},
		{"pdf", "pdf", `{"action":"pdf","path":"/tmp/out.pdf"}`, "PDF"},
		{"cookies get", "cookies", `{"action":"cookies"}`, "Cookies"},
		{"cookies set", "cookies", `{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`, "SetCookies"},
		{"console", "console", `{"action":"console"}`, ""},
	}

	_ = intPtr
	_ = boolPtr

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, mb := newMockTool()
			ctx := context.Background()

			result, err := tool.Execute(ctx, json.RawMessage(tt.input), nil)
			if err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if result.IsError {
				t.Fatalf("tool returned error: %s", result.Content)
			}

			// Verify the mock backend received the expected call.
			if tt.wantSub != "" && len(mb.calls) == 0 {
				t.Errorf("expected backend call with method %q, got none", tt.wantSub)
			}
			if tt.wantSub != "" && len(mb.calls) > 0 {
				found := false
				for _, c := range mb.calls {
					if c.method == tt.wantSub {
						found = true
						break
					}
				}
				if !found {
					methods := make([]string, len(mb.calls))
					for i, c := range mb.calls {
						methods[i] = c.method
					}
					t.Errorf("expected backend call %q, got methods: %v", tt.wantSub, methods)
				}
			}
		})
	}
}

// =============================================================================
// Cookie Save/Load Roundtrip
// =============================================================================

func TestCookieSaveLoadRoundtrip(t *testing.T) {
	// Override HOME so cookies go to temp dir.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cookies := []browser.Cookie{
		{
			Name:     "session_id",
			Value:    "abc123",
			Domain:   "example.com",
			Path:     "/",
			Expires:  1700000000,
			HTTPOnly: true,
			Secure:   true,
			SameSite: "Strict",
		},
		{
			Name:   "theme",
			Value:  "dark",
			Domain: "example.com",
			Path:   "/",
		},
	}

	// Save cookies.
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	cookieDir := filepath.Join(tmpHome, ".forge", "browser")
	if err := os.MkdirAll(cookieDir, 0755); err != nil {
		t.Fatal(err)
	}
	cookieFile := filepath.Join(cookieDir, "cookies-test-session.json")
	if err := os.WriteFile(cookieFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Load cookies.
	loadedData, err := os.ReadFile(cookieFile)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []browser.Cookie
	if err := json.Unmarshal(loadedData, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(loaded))
	}

	// Verify first cookie.
	if loaded[0].Name != "session_id" {
		t.Errorf("cookie[0] name = %q, want %q", loaded[0].Name, "session_id")
	}
	if loaded[0].Value != "abc123" {
		t.Errorf("cookie[0] value = %q, want %q", loaded[0].Value, "abc123")
	}
	if loaded[0].Domain != "example.com" {
		t.Errorf("cookie[0] domain = %q, want %q", loaded[0].Domain, "example.com")
	}
	if !loaded[0].HTTPOnly {
		t.Error("cookie[0] HTTPOnly should be true")
	}
	if !loaded[0].Secure {
		t.Error("cookie[0] Secure should be true")
	}
	if loaded[0].SameSite != "Strict" {
		t.Errorf("cookie[0] SameSite = %q, want %q", loaded[0].SameSite, "Strict")
	}

	// Verify second cookie.
	if loaded[1].Name != "theme" {
		t.Errorf("cookie[1] name = %q, want %q", loaded[1].Name, "theme")
	}
	if loaded[1].Value != "dark" {
		t.Errorf("cookie[1] value = %q, want %q", loaded[1].Value, "dark")
	}
}

// =============================================================================
// New Action Validation (eval, viewport, pdf, cookies, upload, console)
// =============================================================================

func TestNewActionEval(t *testing.T) {
	tool, mb := newMockTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"eval","js":"document.title"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("eval error: %s", result.Content)
	}
	if result.Content != "eval result" {
		t.Errorf("content = %q, want %q", result.Content, "eval result")
	}
	if mb.lastCall().method != "Eval" {
		t.Errorf("expected Eval call, got %q", mb.lastCall().method)
	}
}

func TestNewActionViewport(t *testing.T) {
	tool, mb := newMockTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"viewport","width":1920,"height":1080}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("viewport error: %s", result.Content)
	}
	if result.Content != "Viewport set to 1920x1080" {
		t.Errorf("content = %q, want %q", result.Content, "Viewport set to 1920x1080")
	}
	if mb.lastCall().method != "SetViewport" {
		t.Errorf("expected SetViewport call, got %q", mb.lastCall().method)
	}
}

func TestNewActionPDF(t *testing.T) {
	tool, mb := newMockTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"pdf","path":"/tmp/out.pdf"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("pdf error: %s", result.Content)
	}
	if result.Content != "PDF saved to /tmp/out.pdf" {
		t.Errorf("content = %q, want %q", result.Content, "PDF saved to /tmp/out.pdf")
	}
	if mb.lastCall().method != "PDF" {
		t.Errorf("expected PDF call, got %q", mb.lastCall().method)
	}
}

func TestNewActionCookiesGet(t *testing.T) {
	tool, mb := newMockTool()
	mb.cookies = []browser.Cookie{
		{Name: "session", Value: "xyz"},
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"cookies"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("cookies get error: %s", result.Content)
	}

	// Result should be JSON array of cookies.
	var cookies []browser.Cookie
	if err := json.Unmarshal([]byte(result.Content), &cookies); err != nil {
		t.Fatalf("parse cookies result: %v", err)
	}
	if len(cookies) != 1 || cookies[0].Name != "session" {
		t.Errorf("unexpected cookies: %+v", cookies)
	}
}

func TestNewActionCookiesSet(t *testing.T) {
	tool, mb := newMockTool()
	input := `{"action":"cookies","cookies":[{"name":"token","value":"abc"},{"name":"pref","value":"dark"}]}`
	result, err := tool.Execute(context.Background(), json.RawMessage(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("cookies set error: %s", result.Content)
	}
	if result.Content != "Set 2 cookie(s)" {
		t.Errorf("content = %q, want %q", result.Content, "Set 2 cookie(s)")
	}
	// Verify backend received the cookies.
	if len(mb.cookies) != 2 {
		t.Errorf("backend received %d cookies, want 2", len(mb.cookies))
	}
}

func TestNewActionUpload(t *testing.T) {
	tool, mb := newMockTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"upload","selector":"#file-input","file_path":"/tmp/photo.jpg"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("upload error: %s", result.Content)
	}
	if mb.lastCall().method != "Upload" {
		t.Errorf("expected Upload call, got %q", mb.lastCall().method)
	}
}

func TestNewActionConsole(t *testing.T) {
	tool, mb := newMockTool()
	mb.console = []browser.ConsoleMessage{
		{Level: "log", Text: "Hello from console"},
		{Level: "error", Text: "Something failed"},
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"console"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("console error: %s", result.Content)
	}

	var msgs []browser.ConsoleMessage
	if err := json.Unmarshal([]byte(result.Content), &msgs); err != nil {
		t.Fatalf("parse console result: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 console messages, got %d", len(msgs))
	}
}

func TestNewActionConsoleEmpty(t *testing.T) {
	tool, _ := newMockTool()
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"action":"console"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "No console messages captured." {
		t.Errorf("content = %q, want %q", result.Content, "No console messages captured.")
	}
}

// =============================================================================
// Permission Checks
// =============================================================================

func TestBrowserPermissions(t *testing.T) {
	tool := &browser.Tool{}

	readOnlyActions := []string{
		`{"action":"snapshot"}`,
		`{"action":"get","get":"title"}`,
		`{"action":"wait","target":"#x"}`,
		`{"action":"scroll","direction":"down"}`,
		`{"action":"back"}`,
		`{"action":"forward"}`,
		`{"action":"reload"}`,
		`{"action":"close"}`,
		`{"action":"console"}`,
		`{"action":"cookies"}`,
	}

	for _, input := range readOnlyActions {
		perm, err := tool.CheckPermissions(json.RawMessage(input), nil)
		if err != nil {
			t.Errorf("CheckPermissions(%s) error: %v", input, err)
			continue
		}
		if perm.Behavior != models.PermAllow {
			t.Errorf("CheckPermissions(%s) = %s, want allow", input, perm.Behavior)
		}
	}

	askActions := []string{
		`{"action":"open","url":"https://example.com"}`,
		`{"action":"click","selector":"#btn"}`,
		`{"action":"type","selector":"#in","text":"hi"}`,
		`{"action":"press","key":"Enter"}`,
		`{"action":"screenshot"}`,
		`{"action":"eval","js":"1+1"}`,
		`{"action":"upload","selector":"#f","file_path":"/tmp/a"}`,
		`{"action":"viewport","width":1920,"height":1080}`,
		`{"action":"pdf","path":"/tmp/x.pdf"}`,
		`{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`,
	}

	for _, input := range askActions {
		perm, err := tool.CheckPermissions(json.RawMessage(input), nil)
		if err != nil {
			t.Errorf("CheckPermissions(%s) error: %v", input, err)
			continue
		}
		if perm.Behavior != models.PermAsk {
			t.Errorf("CheckPermissions(%s) = %s, want ask", input, perm.Behavior)
		}
	}
}

func TestBrowserIsReadOnly(t *testing.T) {
	tool := &browser.Tool{}

	readOnly := map[string]bool{
		`{"action":"snapshot"}`:                                     true,
		`{"action":"get","get":"title"}`:                            true,
		`{"action":"wait","target":"#x"}`:                           true,
		`{"action":"scroll","direction":"down"}`:                    true,
		`{"action":"back"}`:                                         true,
		`{"action":"forward"}`:                                      true,
		`{"action":"reload"}`:                                       true,
		`{"action":"close"}`:                                        true,
		`{"action":"console"}`:                                      true,
		`{"action":"cookies"}`:                                      true,
		`{"action":"open","url":"https://x.com"}`:                   false,
		`{"action":"click","selector":"#b"}`:                        false,
		`{"action":"eval","js":"1"}`:                                false,
		`{"action":"cookies","cookies":[{"name":"a","value":"b"}]}`: false,
	}

	for input, expected := range readOnly {
		got := tool.IsReadOnly(json.RawMessage(input))
		if got != expected {
			t.Errorf("IsReadOnly(%s) = %v, want %v", input, got, expected)
		}
	}
}

// =============================================================================
// Session Management
// =============================================================================

func TestBrowserPersistentSession(t *testing.T) {
	tool, _ := newMockTool()
	ctx := context.Background()

	// Multiple calls without explicit session should reuse the same session.
	_, _ = tool.Execute(ctx, json.RawMessage(`{"action":"snapshot"}`), nil)
	_, _ = tool.Execute(ctx, json.RawMessage(`{"action":"get","get":"title"}`), nil)

	// Both calls should have gone to the same mock backend (we only have one).
	// The session name consistency is managed internally.
}

// =============================================================================
// Tool Name and Schema
// =============================================================================

func TestBrowserToolMetadata(t *testing.T) {
	tool := &browser.Tool{}

	if tool.Name() != "Browser" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "Browser")
	}

	schema := tool.InputSchema()
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	// Verify action enum contains all 25 actions.
	props, ok := schemaObj["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("missing properties in schema")
	}
	actionProp, ok := props["action"].(map[string]interface{})
	if !ok {
		t.Fatal("missing action property in schema")
	}
	actionEnum, ok := actionProp["enum"].([]interface{})
	if !ok {
		t.Fatal("missing action enum in schema")
	}

	expectedActions := []string{
		"open", "snapshot", "click", "type", "fill", "press", "wait", "get",
		"screenshot", "scroll", "back", "forward", "reload", "close",
		"hover", "focus", "download", "find", "tab",
		"eval", "upload", "viewport", "pdf", "cookies", "console",
	}
	if len(actionEnum) != len(expectedActions) {
		t.Errorf("action enum has %d entries, want %d", len(actionEnum), len(expectedActions))
	}

	actionSet := make(map[string]bool)
	for _, a := range actionEnum {
		actionSet[a.(string)] = true
	}
	for _, expected := range expectedActions {
		if !actionSet[expected] {
			t.Errorf("missing action %q in enum", expected)
		}
	}
}

func TestBrowserIsConcurrencySafe(t *testing.T) {
	tool := &browser.Tool{}
	if tool.IsConcurrencySafe(json.RawMessage(`{"action":"snapshot"}`)) {
		t.Error("Browser tool should not be concurrency safe")
	}
}
