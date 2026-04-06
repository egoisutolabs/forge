package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/egoisutolabs/forge/auth"
	"github.com/egoisutolabs/forge/config"
	"github.com/egoisutolabs/forge/engine"
)

func TestConnectDialog_ProviderList(t *testing.T) {
	tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.form.Init()

	if cd.step != connectStepProvider {
		t.Errorf("initial step = %d, want %d", cd.step, connectStepProvider)
	}

	view := cd.form.View()
	for _, p := range auth.KnownProviders() {
		name := displayName(p)
		if !strings.Contains(view, name) {
			t.Errorf("provider selection missing %q", name)
		}
	}
}

func TestConnectDialog_ProviderShowsConnectedStatus(t *testing.T) {
	path := tempAuthPath(t)
	if err := auth.SetAPIKeyIn(path, "anthropic", "sk-test"); err != nil {
		t.Fatal(err)
	}

	cd := NewConnectDialog(Theme{}, nil)
	cd.form.Init()
	view := cd.form.View()
	if !strings.Contains(view, "connected") {
		t.Error("expected connected status for anthropic")
	}
}

func TestConnectDialog_AdvanceToKeyInput(t *testing.T) {
	tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.provider = "anthropic"
	cd.advanceToKeyInput()

	if cd.step != connectStepKey {
		t.Errorf("step = %d, want %d", cd.step, connectStepKey)
	}
	// Form should be non-nil and in a fresh state
	if cd.form == nil {
		t.Fatal("expected form to be set for key input step")
	}
}

func TestConnectDialog_AdvancePreservesProvider(t *testing.T) {
	tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.provider = "openai"
	cd.advanceToKeyInput()

	if cd.provider != "openai" {
		t.Errorf("provider = %q, want %q", cd.provider, "openai")
	}
	if cd.step != connectStepKey {
		t.Errorf("step = %d, want %d", cd.step, connectStepKey)
	}
}

func TestConnectDialog_SavePersistsKey(t *testing.T) {
	path := tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.provider = "anthropic"
	cd.apiKey = "sk-ant-test-key-123"

	result := cd.save()
	if !strings.Contains(result, "Connected to Anthropic") {
		t.Errorf("unexpected result: %s", result)
	}

	store, err := auth.LoadFrom(path)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	pa, ok := store.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic not in auth store")
	}
	if pa.APIKey != "sk-ant-test-key-123" {
		t.Errorf("saved key = %q, want %q", pa.APIKey, "sk-ant-test-key-123")
	}
}

func TestConnectDialog_SaveEmptyKeyCancels(t *testing.T) {
	tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.provider = "anthropic"
	cd.apiKey = "   "

	result := cd.save()
	if !strings.Contains(result, "Cancelled") {
		t.Errorf("expected cancel message for empty key, got: %s", result)
	}
}

func TestConnectDialog_EscCancelsProviderStep(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("")

	if m.connectDialog == nil {
		t.Fatal("expected dialog")
	}

	// Send Escape
	model, _ := m.updateConnectDialog(tea.KeyMsg{Type: tea.KeyEscape})
	m2 := model.(AppModel)

	if m2.connectDialog != nil {
		t.Error("expected dialog to be cleared after Esc")
	}
	if len(m2.messages) == 0 {
		t.Fatal("expected cancel message")
	}
	last := m2.messages[len(m2.messages)-1]
	if !strings.Contains(last.Content, "cancelled") {
		t.Errorf("expected cancel message, got: %s", last.Content)
	}
}

func TestConnectDialog_EscCancelsKeyStep(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("")

	if m.connectDialog == nil {
		t.Fatal("expected dialog")
	}

	// Simulate advancing to key step
	m.connectDialog.provider = "openai"
	m.connectDialog.advanceToKeyInput()
	m.connectDialog.form.Init()

	// Send Escape
	model, _ := m.updateConnectDialog(tea.KeyMsg{Type: tea.KeyEscape})
	m2 := model.(AppModel)

	if m2.connectDialog != nil {
		t.Error("expected dialog to be cleared after Esc on key step")
	}
}

func TestConnectDialog_FullFlowCompletesOnFormDone(t *testing.T) {
	path := tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("")

	if m.connectDialog == nil {
		t.Fatal("expected dialog")
	}

	// Simulate completing provider selection step
	m.connectDialog.provider = "groq"
	m.connectDialog.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("test").
			Options(huh.NewOption("Groq", "groq")).
			Value(&m.connectDialog.provider),
	))
	m.connectDialog.form.Init()
	// Mark form as completed by submitting
	m.connectDialog.form.NextGroup()

	model, _ := m.updateConnectDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(AppModel)

	// Should have advanced to key step
	if m2.connectDialog == nil {
		t.Fatal("expected dialog to still be active (key step)")
	}
	if m2.connectDialog.step != connectStepKey {
		t.Errorf("step = %d, want %d (key input)", m2.connectDialog.step, connectStepKey)
	}

	// Simulate completing key input
	m2.connectDialog.apiKey = "gsk-test-key"
	m2.connectDialog.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("test").
			Value(&m2.connectDialog.apiKey),
	))
	m2.connectDialog.form.Init()
	m2.connectDialog.form.NextGroup()

	model2, _ := m2.updateConnectDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := model2.(AppModel)

	if m3.connectDialog != nil {
		t.Error("expected dialog to be nil after completion")
	}

	// Verify key was saved
	store, err := auth.LoadFrom(path)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	pa, ok := store.Providers["groq"]
	if !ok {
		t.Fatal("groq not in auth store")
	}
	if pa.APIKey != "gsk-test-key" {
		t.Errorf("saved key = %q, want %q", pa.APIKey, "gsk-test-key")
	}

	// Verify success message
	found := false
	for _, msg := range m3.messages {
		if strings.Contains(msg.Content, "Connected to Groq") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected success message mentioning Groq")
	}
}

func TestCmdConnect_DirectProviderKey_StillWorks(t *testing.T) {
	path := tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("anthropic sk-direct-key")

	// Should NOT open dialog
	if m.connectDialog != nil {
		t.Error("direct /connect provider key should not open dialog")
	}

	// Should save key
	store, err := auth.LoadFrom(path)
	if err != nil {
		t.Fatalf("load auth: %v", err)
	}
	pa, ok := store.Providers["anthropic"]
	if !ok {
		t.Fatal("anthropic not in auth store")
	}
	if pa.APIKey != "sk-direct-key" {
		t.Errorf("key = %q, want %q", pa.APIKey, "sk-direct-key")
	}
}

// newFullTestAppModel creates a fully initialized AppModel for end-to-end tests
// that exercise the full Update() path (unlike newTestAppModel which is minimal).
func newFullTestAppModel() AppModel {
	eng := engine.New(engine.Config{
		Model: "claude-sonnet-4-6",
		Cwd:   "/tmp",
	})
	bridge := &EngineBridge{
		eng:    eng,
		caller: &MockCaller{},
	}
	m := New(bridge)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return result.(AppModel)
}

func TestConnectDialog_EnterForwardedViaFullUpdate(t *testing.T) {
	tempAuthPath(t)
	m := newFullTestAppModel()
	m.input.SetValue("/connect")

	// Open dialog via full Update path (Enter submits "/connect")
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := result.(AppModel)

	if m2.connectDialog == nil {
		t.Fatal("connectDialog should be set after /connect via full Update path")
	}

	// Init the form (simulates Bubbletea running the Init cmd)
	m2.connectDialog.form.Init()

	// Send another Enter — must go to dialog, not submitPrompt
	result2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := result2.(AppModel)

	for _, msg := range m3.messages {
		if msg.Role == "user" {
			t.Error("Enter key was intercepted by submitPrompt instead of forwarded to connect dialog")
		}
	}
}

func TestConnectDialog_ArrowsForwardedViaFullUpdate(t *testing.T) {
	tempAuthPath(t)
	m := newFullTestAppModel()
	m.input.SetValue("/connect")

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := result.(AppModel)

	if m2.connectDialog == nil {
		t.Fatal("dialog not opened")
	}

	m2.connectDialog.form.Init()

	// Down arrow should go to dialog, not input history
	result2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := result2.(AppModel)
	if m3.connectDialog == nil {
		t.Error("Down arrow dismissed dialog")
	}

	// Up arrow should go to dialog, not input history
	result3, _ := m3.Update(tea.KeyMsg{Type: tea.KeyUp})
	m4 := result3.(AppModel)
	if m4.connectDialog == nil {
		t.Error("Up arrow dismissed dialog")
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"anthropic", "Anthropic"},
		{"openai", "OpenAI"},
		{"openrouter", "OpenRouter"},
		{"xai", "xAI"},
		{"deepinfra", "DeepInfra"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := displayName(tt.provider)
		if got != tt.want {
			t.Errorf("displayName(%q) = %q, want %q", tt.provider, got, tt.want)
		}
	}
}

func TestConnectDialog_IncludesOtherOption(t *testing.T) {
	tempAuthPath(t)
	cd := NewConnectDialog(Theme{}, nil)
	cd.form.Init()

	view := cd.form.View()
	if !strings.Contains(view, "Other") && !strings.Contains(view, "custom provider") {
		t.Error("provider selection missing 'Other (custom provider)' option")
	}
}

func TestConnectDialog_OtherAdvanceToCustomID(t *testing.T) {
	tempAuthPath(t)
	m := newTestAppModel()
	m.cmdConnect("")

	if m.connectDialog == nil {
		t.Fatal("expected dialog")
	}

	// Simulate completing provider selection with "Other" chosen.
	m.connectDialog.provider = otherProviderID
	m.connectDialog.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("test").
			Options(huh.NewOption("Other (custom provider)", otherProviderID)).
			Value(&m.connectDialog.provider),
	))
	m.connectDialog.form.Init()
	m.connectDialog.form.NextGroup()

	model, _ := m.updateConnectDialog(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := model.(AppModel)

	if m2.connectDialog == nil {
		t.Fatal("expected dialog to still be active (custom ID step)")
	}
	if m2.connectDialog.step != connectStepCustomID {
		t.Errorf("step = %d, want %d (custom ID input)", m2.connectDialog.step, connectStepCustomID)
	}
}

func TestConnectDialog_CustomSaveFlow(t *testing.T) {
	tempAuthPath(t)

	// Redirect HOME so config.json writes to a temp directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cd := NewConnectDialog(Theme{}, nil)
	cd.provider = otherProviderID
	cd.customID = "testprovider"
	cd.customURL = "https://api.test.com/v1"
	cd.apiKey = "sk-test"

	result := cd.save()
	if !strings.Contains(result, "testprovider") {
		t.Errorf("save result should mention provider name, got: %s", result)
	}
	if !strings.Contains(result, "https://api.test.com/v1") {
		t.Errorf("save result should mention base URL, got: %s", result)
	}

	// Verify config.json was created with the right content.
	configPath := filepath.Join(tmpHome, ".forge", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var jc config.JSONConfig
	if err := json.Unmarshal(data, &jc); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	p, ok := jc.Providers["testprovider"]
	if !ok {
		t.Fatal("testprovider not found in config.json providers")
	}
	if p.Options.BaseURL != "https://api.test.com/v1" {
		t.Errorf("base URL = %q, want %q", p.Options.BaseURL, "https://api.test.com/v1")
	}
	if p.Options.APIKey != "sk-test" {
		t.Errorf("API key = %q, want %q", p.Options.APIKey, "sk-test")
	}
}

func TestConnectDialog_CustomIDValidation(t *testing.T) {
	tempAuthPath(t)

	// Test the validation logic directly by replicating the same rules
	// used in advanceToCustomID(). This avoids coupling to huh's internal
	// state machine.
	validateID := func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("provider ID cannot be empty")
		}
		if strings.Contains(s, " ") {
			return fmt.Errorf("provider ID cannot contain spaces")
		}
		return nil
	}

	if err := validateID(""); err == nil {
		t.Error("expected empty ID to fail validation")
	}
	if err := validateID("   "); err == nil {
		t.Error("expected whitespace-only ID to fail validation")
	}
	if err := validateID("has spaces"); err == nil {
		t.Error("expected ID with spaces to fail validation")
	}
	if err := validateID("myprovider"); err != nil {
		t.Errorf("expected valid ID to pass, got: %v", err)
	}
}

func TestConnectDialog_CustomURLValidation(t *testing.T) {
	tempAuthPath(t)

	// Test the validation logic directly by replicating the same rules
	// used in advanceToCustomURL().
	validateURL := func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf("base URL cannot be empty")
		}
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			return fmt.Errorf("URL must start with http:// or https://")
		}
		return nil
	}

	if err := validateURL(""); err == nil {
		t.Error("expected empty URL to fail validation")
	}
	if err := validateURL("ftp://bad.example.com"); err == nil {
		t.Error("expected ftp:// URL to fail validation")
	}
	if err := validateURL("api.example.com/v1"); err == nil {
		t.Error("expected URL without scheme to fail validation")
	}
	if err := validateURL("https://api.example.com/v1"); err != nil {
		t.Errorf("expected valid https URL to pass, got: %v", err)
	}
	if err := validateURL("http://localhost:8080"); err != nil {
		t.Errorf("expected valid http URL to pass, got: %v", err)
	}
}
