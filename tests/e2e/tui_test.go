package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/cmd/forge/tui"
)

// --- Theme Tests ---

// TestTUI_DarkTheme verifies DarkTheme returns a valid theme config.
func TestTUI_DarkTheme(t *testing.T) {
	cfg := tui.DarkTheme()
	if cfg.Name != "dark" {
		t.Errorf("DarkTheme().Name = %q, want dark", cfg.Name)
	}
	assertThemeNonEmpty(t, cfg, "DarkTheme")
}

// TestTUI_LightTheme verifies LightTheme returns a valid theme config.
func TestTUI_LightTheme(t *testing.T) {
	cfg := tui.LightTheme()
	if cfg.Name != "light" {
		t.Errorf("LightTheme().Name = %q, want light", cfg.Name)
	}
	assertThemeNonEmpty(t, cfg, "LightTheme")
}

// TestTUI_MinimalTheme verifies MinimalTheme returns a valid theme config.
func TestTUI_MinimalTheme(t *testing.T) {
	cfg := tui.MinimalTheme()
	if cfg.Name != "minimal" {
		t.Errorf("MinimalTheme().Name = %q, want minimal", cfg.Name)
	}
	assertThemeNonEmpty(t, cfg, "MinimalTheme")
}

// TestTUI_ResolveTheme verifies that ResolveTheme produces a Theme with non-zero styles.
func TestTUI_ResolveTheme(t *testing.T) {
	for _, constructor := range []struct {
		name string
		fn   func() tui.ThemeConfig
	}{
		{"dark", tui.DarkTheme},
		{"light", tui.LightTheme},
		{"minimal", tui.MinimalTheme},
	} {
		t.Run(constructor.name, func(t *testing.T) {
			theme := tui.ResolveTheme(constructor.fn())

			// Verify Config is set.
			if theme.Config.Name != constructor.name {
				t.Errorf("Config.Name = %q, want %q", theme.Config.Name, constructor.name)
			}

			// Verify styles are initialized (lipgloss styles render non-empty for non-empty text).
			rendered := theme.UserStyle.Render("test")
			if rendered == "" {
				t.Error("UserStyle.Render produced empty string")
			}
			rendered = theme.ErrorStyle.Render("error")
			if rendered == "" {
				t.Error("ErrorStyle.Render produced empty string")
			}
		})
	}
}

// TestTUI_LoadThemeFromFile verifies loading a theme from a YAML file.
func TestTUI_LoadThemeFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	themePath := filepath.Join(tmpDir, "theme.yaml")

	yamlContent := `
name: custom
user_color: "#ff0000"
assistant_color: "#00ff00"
tool_color: "#0000ff"
error_color: "9"
border_color: "8"
accent_color: "12"
dim_color: "8"
success_color: "10"
warning_color: "11"
spinner_color: "5"
status_bg_color: "0"
status_text_color: "8"
code_bg_color: "235"
`
	if err := os.WriteFile(themePath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := tui.LoadThemeFromFile(themePath)
	if err != nil {
		t.Fatalf("LoadThemeFromFile: %v", err)
	}
	if cfg.Name != "custom" {
		t.Errorf("Name = %q, want custom", cfg.Name)
	}
	if cfg.UserColor != "#ff0000" {
		t.Errorf("UserColor = %q, want #ff0000", cfg.UserColor)
	}
	if cfg.AssistantColor != "#00ff00" {
		t.Errorf("AssistantColor = %q, want #00ff00", cfg.AssistantColor)
	}
}

// TestTUI_LoadThemeFromFile_Missing verifies error on missing file.
func TestTUI_LoadThemeFromFile_Missing(t *testing.T) {
	_, err := tui.LoadThemeFromFile("/nonexistent/theme.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- History Tests ---

// TestTUI_History_AddAndNavigate verifies the add/navigate roundtrip.
func TestTUI_History_AddAndNavigate(t *testing.T) {
	h := tui.NewHistory(100)

	// Empty history.
	if h.Len() != 0 {
		t.Errorf("initial Len() = %d, want 0", h.Len())
	}
	if _, ok := h.Up(""); ok {
		t.Error("Up on empty history should return false")
	}

	// Add entries.
	h.Add("first")
	h.Add("second")
	h.Add("third")

	if h.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", h.Len())
	}

	// Entries returns oldest first.
	entries := h.Entries()
	if entries[0] != "first" || entries[1] != "second" || entries[2] != "third" {
		t.Errorf("Entries = %v", entries)
	}

	// Up navigates from newest to oldest.
	text, ok := h.Up("current-draft")
	if !ok {
		t.Fatal("Up should succeed")
	}
	if text != "third" {
		t.Errorf("Up(1) = %q, want third", text)
	}
	if !h.Browsing() {
		t.Error("should be browsing after Up")
	}

	text, ok = h.Up("")
	if !ok {
		t.Fatal("Up should succeed")
	}
	if text != "second" {
		t.Errorf("Up(2) = %q, want second", text)
	}

	text, ok = h.Up("")
	if !ok {
		t.Fatal("Up should succeed")
	}
	if text != "first" {
		t.Errorf("Up(3) = %q, want first", text)
	}

	// At oldest — Up should return same entry.
	text, ok = h.Up("")
	if ok {
		t.Error("Up at oldest should return false")
	}
	if text != "first" {
		t.Errorf("Up at oldest = %q, want first", text)
	}

	// Down navigates back to newer entries.
	text, ok = h.Down()
	if !ok {
		t.Fatal("Down should succeed")
	}
	if text != "second" {
		t.Errorf("Down(1) = %q, want second", text)
	}

	text, ok = h.Down()
	if !ok {
		t.Fatal("Down should succeed")
	}
	if text != "third" {
		t.Errorf("Down(2) = %q, want third", text)
	}

	// Down past newest restores draft.
	text, ok = h.Down()
	if !ok {
		t.Fatal("Down should succeed to restore draft")
	}
	if text != "current-draft" {
		t.Errorf("Down restoring draft = %q, want current-draft", text)
	}
	if h.Browsing() {
		t.Error("should not be browsing after returning to draft")
	}
}

// TestTUI_History_DeduplicateConsecutive verifies consecutive duplicates are skipped.
func TestTUI_History_DeduplicateConsecutive(t *testing.T) {
	h := tui.NewHistory(100)
	h.Add("same")
	h.Add("same")
	h.Add("same")

	if h.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (consecutive dedup)", h.Len())
	}
}

// TestTUI_History_EmptyAndWhitespace verifies empty/whitespace entries are ignored.
func TestTUI_History_EmptyAndWhitespace(t *testing.T) {
	h := tui.NewHistory(100)
	h.Add("")
	h.Add("   ")
	h.Add("\t\n")

	if h.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (empty entries ignored)", h.Len())
	}
}

// TestTUI_History_MaxSize verifies the max size cap.
func TestTUI_History_MaxSize(t *testing.T) {
	h := tui.NewHistory(3)
	h.Add("a")
	h.Add("b")
	h.Add("c")
	h.Add("d")

	if h.Len() != 3 {
		t.Errorf("Len() = %d, want 3 (capped)", h.Len())
	}

	entries := h.Entries()
	if entries[0] != "b" {
		t.Errorf("oldest entry = %q, want b (a should be trimmed)", entries[0])
	}
}

// TestTUI_History_SaveAndLoad verifies file persistence roundtrip.
func TestTUI_History_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "history")

	h := tui.NewHistory(100)
	h.Add("first command")
	h.Add("second command")
	h.Add("/help")

	if err := h.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	h2 := tui.NewHistory(100)
	if err := h2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if h2.Len() != 3 {
		t.Fatalf("loaded Len() = %d, want 3", h2.Len())
	}
	entries := h2.Entries()
	if entries[0] != "first command" {
		t.Errorf("loaded[0] = %q", entries[0])
	}
	if entries[2] != "/help" {
		t.Errorf("loaded[2] = %q", entries[2])
	}
}

// TestTUI_History_Reset verifies Reset exits browsing mode.
func TestTUI_History_Reset(t *testing.T) {
	h := tui.NewHistory(100)
	h.Add("entry")
	h.Up("draft")
	if !h.Browsing() {
		t.Fatal("should be browsing")
	}
	h.Reset()
	if h.Browsing() {
		t.Error("should not be browsing after Reset")
	}
}

// --- Autocomplete Tests ---

// TestTUI_Autocomplete_FilterSlashCommands verifies autocomplete filtering.
func TestTUI_Autocomplete_FilterSlashCommands(t *testing.T) {
	reg := tui.NewCommandRegistry()

	// Verify built-in commands exist.
	all := reg.Commands()
	if len(all) == 0 {
		t.Fatal("NewCommandRegistry should have built-in commands")
	}

	// Look up by exact name.
	helpCmd := reg.Lookup("help")
	if helpCmd == nil {
		t.Fatal("help command not found")
	}
	if helpCmd.Description == "" {
		t.Error("help command should have description")
	}

	// Look up by alias.
	qCmd := reg.Lookup("q")
	if qCmd == nil {
		t.Fatal("alias 'q' should resolve to quit command")
	}
	if qCmd.Name != "quit" {
		t.Errorf("alias q resolved to %q, want quit", qCmd.Name)
	}

	// Match prefix.
	matches := reg.Match("he")
	found := false
	for _, m := range matches {
		if m.Name == "help" {
			found = true
		}
	}
	if !found {
		t.Error("Match('he') should include help")
	}

	// Match empty prefix → all non-hidden commands.
	allMatches := reg.Match("")
	for _, m := range allMatches {
		if m.Hidden {
			t.Errorf("hidden command %q should not appear in Match('')", m.Name)
		}
	}

	// Match non-matching prefix.
	noMatch := reg.Match("zzz")
	if len(noMatch) != 0 {
		t.Errorf("Match('zzz') returned %d results, want 0", len(noMatch))
	}
}

// TestTUI_Autocomplete_RegisterCustomCommand verifies adding custom commands.
func TestTUI_Autocomplete_RegisterCustomCommand(t *testing.T) {
	reg := tui.NewCommandRegistry()
	initial := len(reg.Commands())

	reg.Register(tui.SlashCommand{
		Name:        "deploy",
		Description: "Deploy the application",
		Aliases:     []string{"d"},
	})

	if len(reg.Commands()) != initial+1 {
		t.Errorf("Commands count = %d, want %d", len(reg.Commands()), initial+1)
	}

	cmd := reg.Lookup("deploy")
	if cmd == nil {
		t.Fatal("deploy command not found")
	}
	if cmd.Description != "Deploy the application" {
		t.Errorf("description = %q", cmd.Description)
	}

	// Alias lookup.
	cmd = reg.Lookup("d")
	if cmd == nil {
		t.Fatal("alias 'd' not found")
	}
	if cmd.Name != "deploy" {
		t.Errorf("alias 'd' resolved to %q", cmd.Name)
	}
}

// TestTUI_Autocomplete_WidgetState verifies Autocomplete show/hide/navigate.
func TestTUI_Autocomplete_WidgetState(t *testing.T) {
	reg := tui.NewCommandRegistry()
	ac := tui.NewAutocomplete(reg)

	// Initially hidden.
	if ac.Visible() {
		t.Error("should be hidden initially")
	}
	if ac.Selected() != nil {
		t.Error("selected should be nil when hidden")
	}

	// Show with empty query → all non-hidden commands.
	ac.Show("")
	if !ac.Visible() {
		t.Error("should be visible after Show")
	}
	if ac.FilteredCount() == 0 {
		t.Error("FilteredCount should be > 0")
	}

	// Selected should be the first command.
	sel := ac.Selected()
	if sel == nil {
		t.Fatal("Selected should be non-nil")
	}

	// Navigate next.
	firstName := sel.Name
	ac.Next()
	sel2 := ac.Selected()
	if sel2 == nil {
		t.Fatal("Selected after Next should be non-nil")
	}
	if sel2.Name == firstName && ac.FilteredCount() > 1 {
		t.Error("Next should move to a different command")
	}

	// Navigate prev.
	ac.Prev()
	sel3 := ac.Selected()
	if sel3.Name != firstName {
		t.Errorf("Prev should return to first: got %q, want %q", sel3.Name, firstName)
	}

	// Hide.
	ac.Hide()
	if ac.Visible() {
		t.Error("should be hidden after Hide")
	}

	// Update with filter.
	ac.Show("hel")
	if ac.FilteredCount() == 0 {
		t.Error("'hel' should match 'help'")
	}
	ac.Update("q")
	if ac.FilteredCount() == 0 {
		t.Error("'q' should match 'quit'")
	}
}

// TestTUI_BuiltinCommands_ExpectedSet verifies the expected built-in slash commands.
func TestTUI_BuiltinCommands_ExpectedSet(t *testing.T) {
	reg := tui.NewCommandRegistry()

	expectedCommands := []string{
		"help", "clear", "model", "compact", "cost",
		"diff", "history", "quit", "bug", "config",
	}

	for _, name := range expectedCommands {
		cmd := reg.Lookup(name)
		if cmd == nil {
			t.Errorf("built-in command %q not found", name)
		}
	}
}

// assertThemeNonEmpty checks that all color fields of a ThemeConfig are non-empty.
func assertThemeNonEmpty(t *testing.T, cfg tui.ThemeConfig, source string) {
	t.Helper()
	fields := map[string]string{
		"UserColor":       cfg.UserColor,
		"AssistantColor":  cfg.AssistantColor,
		"ToolColor":       cfg.ToolColor,
		"ErrorColor":      cfg.ErrorColor,
		"BorderColor":     cfg.BorderColor,
		"AccentColor":     cfg.AccentColor,
		"DimColor":        cfg.DimColor,
		"SuccessColor":    cfg.SuccessColor,
		"WarningColor":    cfg.WarningColor,
		"SpinnerColor":    cfg.SpinnerColor,
		"StatusBgColor":   cfg.StatusBgColor,
		"StatusTextColor": cfg.StatusTextColor,
		"CodeBgColor":     cfg.CodeBgColor,
	}
	for name, value := range fields {
		if value == "" {
			t.Errorf("%s: %s is empty", source, name)
		}
	}
}
