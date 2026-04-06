package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/auth"
	"github.com/egoisutolabs/forge/engine"
)

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0K"},
		{1200, "1.2K"},
		{12400, "12.4K"},
		{999999, "1000.0K"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHandleSlashCommand_Known(t *testing.T) {
	m := newTestAppModel()

	known := []struct {
		name    string
		handled bool
	}{
		{"help", true},
		{"h", true},
		{"?", true},
		{"cost", true},
		{"clear", true},
		{"cls", true},
		{"model", true},
		{"compact", true},
		{"history", true},
		{"quit", true},
		{"exit", true},
		{"q", true},
		{"unknown", false},
		{"commit", false}, // handled by engine via skill, not TUI
	}

	for _, tt := range known {
		_, ok := m.handleSlashCommand(tt.name, "")
		if ok != tt.handled {
			t.Errorf("handleSlashCommand(%q): handled=%v, want %v", tt.name, ok, tt.handled)
		}
	}
}

func TestCmdClear(t *testing.T) {
	m := newTestAppModel()
	m.messages = []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	m.cmdClear()
	if len(m.messages) != 0 {
		t.Errorf("after /clear: %d messages, want 0", len(m.messages))
	}
}

func TestCmdModel_ShowCurrent(t *testing.T) {
	m := newTestAppModel()
	m.cmdModel("")
	if len(m.messages) == 0 {
		t.Fatal("expected system message with current model")
	}
	if m.messages[len(m.messages)-1].Role != "system" {
		t.Error("expected system role for model display")
	}
}

func TestCmdModel_Switch(t *testing.T) {
	m := newTestAppModel()
	m.cmdModel("claude-opus-4-6")
	if m.bridge.Model() != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q", m.bridge.Model(), "claude-opus-4-6")
	}
	if m.status.Model != "claude-opus-4-6" {
		t.Errorf("status.Model = %q, want %q", m.status.Model, "claude-opus-4-6")
	}
}

func TestCmdCost(t *testing.T) {
	m := newTestAppModel()
	m.cmdCost()
	if len(m.messages) == 0 {
		t.Fatal("expected system message with cost info")
	}
	msg := m.messages[len(m.messages)-1]
	if msg.Role != "system" {
		t.Error("expected system role for cost display")
	}
}

func TestCmdHelp(t *testing.T) {
	m := newTestAppModel()
	m.cmdHelp()
	if len(m.messages) == 0 {
		t.Fatal("expected system message with help text")
	}
	msg := m.messages[len(m.messages)-1]
	if msg.Role != "system" {
		t.Error("expected system role for help display")
	}
}

// newTestAppModel creates a minimal AppModel for command tests.
func newTestAppModel() *AppModel {
	eng := engine.New(engine.Config{
		Model: "claude-sonnet-4-6",
		Cwd:   "/tmp",
	})
	bridge := &EngineBridge{
		eng:    eng,
		caller: &MockCaller{},
	}
	m := &AppModel{
		bridge:  bridge,
		status:  StatusInfo{Model: "claude-sonnet-4-6"},
		history: NewHistory(100),
	}
	return m
}

// tempAuthPath sets up a temp auth.json and returns cleanup func.
func tempAuthPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	auth.SetPath(path)
	t.Cleanup(func() { auth.SetPath("") })
	return path
}

func TestHandleSlashCommand_ConnectAndProviders(t *testing.T) {
	m := newTestAppModel()

	cases := []struct {
		name    string
		handled bool
	}{
		{"connect", true},
		{"providers", true},
	}
	for _, tt := range cases {
		_, ok := m.handleSlashCommand(tt.name, "")
		if ok != tt.handled {
			t.Errorf("handleSlashCommand(%q): handled=%v, want %v", tt.name, ok, tt.handled)
		}
	}
}

func TestCmdConnect_NoArgs_OpensDialog(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("")

	if m.connectDialog == nil {
		t.Fatal("expected connect dialog to be opened")
	}
	if m.connectDialog.step != connectStepProvider {
		t.Errorf("step = %d, want %d (provider selection)", m.connectDialog.step, connectStepProvider)
	}

	// Dialog should render provider options
	view := m.connectDialog.form.View()
	if !strings.Contains(view, "Anthropic") {
		t.Error("expected provider selection to include Anthropic")
	}
	if !strings.Contains(view, "OpenAI") {
		t.Error("expected provider selection to include OpenAI")
	}
}

func TestCmdConnect_ProviderOnly_ShowsUsage(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("anthropic")

	if len(m.messages) == 0 {
		t.Fatal("expected system message")
	}
	msg := m.messages[len(m.messages)-1]
	if !strings.Contains(msg.Content, "/connect anthropic <api-key>") {
		t.Errorf("expected usage hint for anthropic, got: %s", msg.Content)
	}
}

func TestCmdConnect_UnknownProvider_ShowsError(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("fakeprovider sk-key-123")

	if len(m.messages) == 0 {
		t.Fatal("expected system message")
	}
	msg := m.messages[len(m.messages)-1]
	if !strings.Contains(msg.Content, "Unknown provider") {
		t.Errorf("expected unknown provider error, got: %s", msg.Content)
	}
}

func TestCmdConnect_ProviderAndKey_Saves(t *testing.T) {
	path := tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("anthropic sk-ant-test-key-123")

	// Verify system message confirms save
	if len(m.messages) == 0 {
		t.Fatal("expected system message")
	}
	msg := m.messages[len(m.messages)-1]
	if !strings.Contains(msg.Content, "anthropic") {
		t.Errorf("expected confirmation mentioning anthropic, got: %s", msg.Content)
	}

	// Verify key was actually saved to auth store
	store, err := auth.LoadFrom(path)
	if err != nil {
		t.Fatalf("failed to load auth store: %v", err)
	}
	pa, ok := store.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic not found in auth store")
	}
	if pa.APIKey != "sk-ant-test-key-123" {
		t.Errorf("saved key = %q, want %q", pa.APIKey, "sk-ant-test-key-123")
	}
}

func TestCmdProviders_ShowsAllProviders(t *testing.T) {
	path := tempAuthPath(t)
	// Pre-save a key for anthropic
	if err := auth.SetAPIKeyIn(path, "anthropic", "sk-test"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	m := newTestAppModel()
	m.cmdProviders()

	if len(m.messages) == 0 {
		t.Fatal("expected system message")
	}
	msg := m.messages[len(m.messages)-1]
	if msg.Role != "system" {
		t.Error("expected system role")
	}

	// Should list all 8 known providers
	for _, p := range auth.KnownProviders() {
		if !strings.Contains(msg.Content, p) {
			t.Errorf("expected provider %q in output", p)
		}
	}

	// anthropic should show as connected
	if !strings.Contains(msg.Content, "✓") {
		t.Error("expected ✓ for connected provider")
	}
	// at least one should show as not configured
	if !strings.Contains(msg.Content, "✗") {
		t.Error("expected ✗ for unconfigured provider")
	}
}

func TestCmdProviders_EnvVarDetection(t *testing.T) {
	tempAuthPath(t)
	// Set an env var for openai
	os.Setenv("OPENAI_API_KEY", "sk-env-test")
	t.Cleanup(func() { os.Unsetenv("OPENAI_API_KEY") })

	m := newTestAppModel()
	m.cmdProviders()

	if len(m.messages) == 0 {
		t.Fatal("expected system message")
	}
	msg := m.messages[len(m.messages)-1]
	// openai should show as connected via env var
	if !strings.Contains(msg.Content, "OPENAI_API_KEY") {
		t.Errorf("expected env var name in output, got: %s", msg.Content)
	}
}
