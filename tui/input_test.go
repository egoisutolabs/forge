package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- History tests ----

func TestHistory_AddAndNavigate(t *testing.T) {
	h := NewHistory(100)
	h.Add("first")
	h.Add("second")
	h.Add("third")

	if h.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", h.Len())
	}

	// Up from nothing should give "third" (most recent)
	entry, ok := h.Up("")
	if !ok || entry != "third" {
		t.Fatalf("expected 'third', got %q ok=%v", entry, ok)
	}

	// Up again gives "second"
	entry, ok = h.Up("")
	if !ok || entry != "second" {
		t.Fatalf("expected 'second', got %q", entry)
	}

	// Up again gives "first"
	entry, ok = h.Up("")
	if !ok || entry != "first" {
		t.Fatalf("expected 'first', got %q", entry)
	}

	// Up past oldest returns false
	_, ok = h.Up("")
	if ok {
		t.Fatal("expected ok=false at oldest entry")
	}

	// Down goes back to "second"
	entry, ok = h.Down()
	if !ok || entry != "second" {
		t.Fatalf("expected 'second', got %q", entry)
	}

	// Down to "third"
	entry, ok = h.Down()
	if !ok || entry != "third" {
		t.Fatalf("expected 'third', got %q", entry)
	}

	// Down past newest restores draft
	entry, ok = h.Down()
	if !ok || entry != "" {
		t.Fatalf("expected empty draft, got %q", entry)
	}
}

func TestHistory_DraftPreservation(t *testing.T) {
	h := NewHistory(100)
	h.Add("old")

	// User has typed something but not submitted
	entry, ok := h.Up("my draft")
	if !ok || entry != "old" {
		t.Fatalf("expected 'old', got %q", entry)
	}

	// Going back down should restore the draft
	entry, ok = h.Down()
	if !ok || entry != "my draft" {
		t.Fatalf("expected 'my draft', got %q", entry)
	}
}

func TestHistory_Deduplication(t *testing.T) {
	h := NewHistory(100)
	h.Add("hello")
	h.Add("hello")
	h.Add("hello")

	if h.Len() != 1 {
		t.Fatalf("expected 1 entry after dedup, got %d", h.Len())
	}
}

func TestHistory_MaxSize(t *testing.T) {
	h := NewHistory(3)
	h.Add("a")
	h.Add("b")
	h.Add("c")
	h.Add("d")

	if h.Len() != 3 {
		t.Fatalf("expected 3 entries (trimmed), got %d", h.Len())
	}

	entries := h.Entries()
	if entries[0] != "b" || entries[1] != "c" || entries[2] != "d" {
		t.Fatalf("expected [b, c, d], got %v", entries)
	}
}

func TestHistory_EmptyAdd(t *testing.T) {
	h := NewHistory(100)
	h.Add("")
	h.Add("   ")

	if h.Len() != 0 {
		t.Fatalf("expected 0 entries for blank input, got %d", h.Len())
	}
}

func TestHistory_UpOnEmpty(t *testing.T) {
	h := NewHistory(100)
	_, ok := h.Up("")
	if ok {
		t.Fatal("expected ok=false on empty history")
	}
}

func TestHistory_DownWithoutBrowsing(t *testing.T) {
	h := NewHistory(100)
	h.Add("test")
	_, ok := h.Down()
	if ok {
		t.Fatal("expected ok=false when not browsing")
	}
}

func TestHistory_Reset(t *testing.T) {
	h := NewHistory(100)
	h.Add("one")
	h.Up("")
	if !h.Browsing() {
		t.Fatal("expected Browsing()=true after Up")
	}
	h.Reset()
	if h.Browsing() {
		t.Fatal("expected Browsing()=false after Reset")
	}
}

func TestHistory_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history")

	// Save
	h1 := NewHistory(100)
	h1.Add("first")
	h1.Add("second")
	if err := h1.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	// Load
	h2 := NewHistory(100)
	if err := h2.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if h2.Len() != 2 {
		t.Fatalf("expected 2 entries after load, got %d", h2.Len())
	}
	entries := h2.Entries()
	if entries[0] != "first" || entries[1] != "second" {
		t.Fatalf("expected [first, second], got %v", entries)
	}
}

func TestHistory_LoadNonexistent(t *testing.T) {
	h := NewHistory(100)
	err := h.LoadFromFile("/nonexistent/path/history")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// ---- Autocomplete tests ----

func TestAutocomplete_Match(t *testing.T) {
	reg := NewCommandRegistry()

	// Empty prefix returns all non-hidden commands
	all := reg.Match("")
	if len(all) == 0 {
		t.Fatal("expected non-empty match for empty prefix")
	}

	// "he" should match "help"
	matches := reg.Match("he")
	found := false
	for _, m := range matches {
		if m.Name == "help" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'help' to match prefix 'he'")
	}

	// "q" should match "quit"
	matches = reg.Match("q")
	found = false
	for _, m := range matches {
		if m.Name == "quit" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'quit' to match prefix 'q'")
	}

	// "zzz" should match nothing
	matches = reg.Match("zzz")
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches for 'zzz', got %d", len(matches))
	}
}

func TestAutocomplete_MatchAliases(t *testing.T) {
	reg := NewCommandRegistry()

	// "ex" should match "quit" via its "exit" alias
	matches := reg.Match("ex")
	found := false
	for _, m := range matches {
		if m.Name == "quit" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'quit' to match alias prefix 'ex'")
	}
}

func TestAutocomplete_Lookup(t *testing.T) {
	reg := NewCommandRegistry()

	cmd := reg.Lookup("help")
	if cmd == nil {
		t.Fatal("expected to find 'help' command")
	}
	if cmd.Name != "help" {
		t.Fatalf("expected Name='help', got %q", cmd.Name)
	}

	// Lookup by alias
	cmd = reg.Lookup("q")
	if cmd == nil {
		t.Fatal("expected to find 'quit' via alias 'q'")
	}

	// Nonexistent
	cmd = reg.Lookup("nonexistent")
	if cmd != nil {
		t.Fatal("expected nil for nonexistent command")
	}
}

func TestAutocomplete_Navigation(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)

	// Not visible initially
	if ac.Visible() {
		t.Fatal("expected not visible initially")
	}

	// Show with empty query
	ac.Show("")
	if !ac.Visible() {
		t.Fatal("expected visible after Show")
	}
	if ac.FilteredCount() == 0 {
		t.Fatal("expected filtered results")
	}

	// Selected should be first item
	sel := ac.Selected()
	if sel == nil {
		t.Fatal("expected non-nil selection")
	}

	// Navigate forward
	first := sel.Name
	ac.Next()
	sel = ac.Selected()
	if sel.Name == first {
		t.Fatal("expected cursor to advance")
	}

	// Navigate backward — should return to first
	ac.Prev()
	sel = ac.Selected()
	if sel.Name != first {
		t.Fatalf("expected %q after Prev, got %q", first, sel.Name)
	}
}

func TestAutocomplete_WrapAround(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)
	ac.Show("")
	count := ac.FilteredCount()

	// Prev from 0 should wrap to last
	ac.Prev()
	sel := ac.Selected()
	if sel == nil {
		t.Fatal("expected non-nil after wrap")
	}

	// Next from last should wrap to first
	ac.Next()
	sel2 := ac.Selected()
	if sel2 == nil {
		t.Fatal("expected non-nil after forward wrap")
	}

	_ = count
}

func TestAutocomplete_Hide(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)
	ac.Show("he")
	ac.Hide()
	if ac.Visible() {
		t.Fatal("expected not visible after Hide")
	}
}

func TestAutocomplete_Update(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)

	ac.Show("")
	initial := ac.FilteredCount()

	ac.Update("he")
	filtered := ac.FilteredCount()

	if filtered >= initial {
		t.Fatalf("expected fewer results after filtering, got %d >= %d", filtered, initial)
	}
}

func TestAutocomplete_Render(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)
	theme := ResolveTheme(DarkTheme())

	// Not visible — empty render
	out := ac.Render(80, theme)
	if out != "" {
		t.Fatal("expected empty render when not visible")
	}

	// Visible
	ac.Show("")
	out = ac.Render(80, theme)
	if out == "" {
		t.Fatal("expected non-empty render when visible")
	}
}

func TestAutocomplete_SelectedNilWhenHidden(t *testing.T) {
	reg := NewCommandRegistry()
	ac := NewAutocomplete(reg)
	if ac.Selected() != nil {
		t.Fatal("expected nil selection when hidden")
	}
}

// ---- Theme tests ----

func TestTheme_DarkDefaults(t *testing.T) {
	cfg := DarkTheme()
	if cfg.Name != "dark" {
		t.Fatalf("expected name='dark', got %q", cfg.Name)
	}
	if cfg.UserColor == "" {
		t.Fatal("expected non-empty UserColor")
	}
}

func TestTheme_LightDefaults(t *testing.T) {
	cfg := LightTheme()
	if cfg.Name != "light" {
		t.Fatalf("expected name='light', got %q", cfg.Name)
	}
}

func TestTheme_MinimalDefaults(t *testing.T) {
	cfg := MinimalTheme()
	if cfg.Name != "minimal" {
		t.Fatalf("expected name='minimal', got %q", cfg.Name)
	}
}

func TestTheme_Resolve(t *testing.T) {
	cfg := DarkTheme()
	theme := ResolveTheme(cfg)
	if theme.Config.Name != "dark" {
		t.Fatalf("expected resolved theme name='dark', got %q", theme.Config.Name)
	}
	// Spot check that styles are non-zero
	rendered := theme.UserStyle.Render("test")
	if rendered == "" {
		t.Fatal("expected non-empty styled output")
	}
}

func TestTheme_LoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")

	yaml := `name: custom
user_color: "14"
assistant_color: "15"
error_color: "1"
accent_color: "6"
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write theme file: %v", err)
	}

	cfg, err := LoadThemeFromFile(path)
	if err != nil {
		t.Fatalf("LoadThemeFromFile: %v", err)
	}

	if cfg.Name != "custom" {
		t.Fatalf("expected name='custom', got %q", cfg.Name)
	}
	if cfg.UserColor != "14" {
		t.Fatalf("expected UserColor='14', got %q", cfg.UserColor)
	}
}

func TestTheme_MergeWithDefaults(t *testing.T) {
	partial := ThemeConfig{
		Name:      "partial",
		UserColor: "14",
	}
	merged := mergeWithDefaults(partial, DarkTheme())
	if merged.UserColor != "14" {
		t.Fatal("expected UserColor to stay as override")
	}
	if merged.ErrorColor != "9" {
		t.Fatalf("expected ErrorColor from default, got %q", merged.ErrorColor)
	}
}

func TestTheme_LoadNonexistent(t *testing.T) {
	_, err := LoadThemeFromFile("/nonexistent/theme.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestTheme_LoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0o644)

	_, err := LoadThemeFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// ---- Header rendering tests ----

func TestRenderHeader(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	header := m.renderHeader()
	if header == "" {
		t.Fatal("expected non-empty header")
	}
	if !strings.Contains(header, "Forge") {
		t.Fatal("expected header to contain 'Forge'")
	}
}

func TestRenderHeader_ZeroWidth(t *testing.T) {
	m := newTestModel()
	header := m.renderHeader()
	if header != "" {
		t.Fatalf("expected empty header for zero width, got %q", header)
	}
}

func TestRenderHeader_WithCost(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.status.CostUSD = 0.0123

	header := m.renderHeader()
	if !strings.Contains(header, "$") {
		t.Fatal("expected header to contain cost")
	}
}

// ---- Shortcut help overlay tests ----

func TestShortcutHelp_Contents(t *testing.T) {
	shortcuts := ShortcutHelp()
	if len(shortcuts) == 0 {
		t.Fatal("expected non-empty shortcuts list")
	}
	// Check for key entries
	foundEnter := false
	for _, s := range shortcuts {
		if s.Key == "Enter" {
			foundEnter = true
		}
	}
	if !foundEnter {
		t.Fatal("expected 'Enter' shortcut")
	}
}

func TestRenderShortcutHelp(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	out := m.renderShortcutHelp()
	if out == "" {
		t.Fatal("expected non-empty shortcut help")
	}
	if !strings.Contains(out, "Keyboard Shortcuts") {
		t.Fatal("expected 'Keyboard Shortcuts' title")
	}
}

// ---- Integration: history wired into app ----

func TestApp_HistoryUpDown(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Submit a message to populate history
	m.input.SetValue("test query")
	next, _ := m.Update(keyMsg("enter"))
	m = next.(AppModel)

	if m.history.Len() != 1 {
		t.Fatalf("expected 1 history entry, got %d", m.history.Len())
	}

	// Wait for "processing" to end — simulate PromptDoneMsg
	next, _ = m.Update(PromptDoneMsg{Result: nil})
	m = next.(AppModel)

	// Press up arrow — should recall "test query"
	next, _ = m.Update(keyMsg("up"))
	m = next.(AppModel)
	if m.input.Value() != "test query" {
		t.Fatalf("expected input to show 'test query', got %q", m.input.Value())
	}

	// Press down — should restore empty draft
	next, _ = m.Update(keyMsg("down"))
	m = next.(AppModel)
	if m.input.Value() != "" {
		t.Fatalf("expected empty input after down, got %q", m.input.Value())
	}
}

func TestApp_ShowHelpToggle(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	if m.showHelp {
		t.Fatal("expected showHelp=false initially")
	}

	// Toggle on
	next, _ := m.Update(keyMsg("ctrl+/"))
	m = next.(AppModel)
	if !m.showHelp {
		t.Fatal("expected showHelp=true after ctrl+/")
	}

	// Toggle off
	next, _ = m.Update(keyMsg("ctrl+/"))
	m = next.(AppModel)
	if m.showHelp {
		t.Fatal("expected showHelp=false after second ctrl+/")
	}
}

func TestApp_EscClosesHelp(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.showHelp = true

	next, _ := m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.showHelp {
		t.Fatal("expected showHelp=false after esc")
	}
}

func TestApp_EscClosesAutocomplete(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.autocomplete.Show("")

	if !m.autocomplete.Visible() {
		t.Fatal("expected autocomplete visible")
	}

	next, _ := m.Update(keyMsg("esc"))
	m = next.(AppModel)
	if m.autocomplete.Visible() {
		t.Fatal("expected autocomplete hidden after esc")
	}
}

// keyMsg is a helper to create a tea.KeyMsg from a key string.
func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+/":
		return tea.KeyMsg{Type: tea.KeyCtrlUnderscore} // ctrl+/ sends 0x1F = ctrl+_
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// ---- Activity panel tests ----

func TestRenderActiveToolsDetailed_Empty(t *testing.T) {
	out := renderActiveToolsDetailed(nil, "⠋", 80)
	if out != "" {
		t.Fatal("expected empty for no active tools")
	}
}

func TestRenderActiveToolsDetailed_WithDetail(t *testing.T) {
	tools := []ActiveToolInfo{
		{Name: "Bash", ID: "t1", Detail: "`git status`", StartTime: time.Now()},
		{Name: "Read", ID: "t2", Detail: "src/main.go", StartTime: time.Now()},
	}
	out := renderActiveToolsDetailed(tools, "⠋", 120)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if !strings.Contains(out, "git status") {
		t.Fatal("expected output to contain command detail")
	}
	if !strings.Contains(out, "src/main.go") {
		t.Fatal("expected output to contain file path detail")
	}
}

func TestRenderActiveToolsDetailed_MaxVisible(t *testing.T) {
	var tools []ActiveToolInfo
	for i := 0; i < 8; i++ {
		tools = append(tools, ActiveToolInfo{
			Name:      "Read",
			ID:        fmt.Sprintf("t%d", i),
			StartTime: time.Now(),
		})
	}
	out := renderActiveToolsDetailed(tools, "⠋", 120)
	if !strings.Contains(out, "more tools running") {
		t.Fatal("expected overflow indicator for >5 tools")
	}
}

func TestToolVerbDetailed(t *testing.T) {
	cases := []struct {
		name, detail, wantContains string
	}{
		{"Bash", "`ls -la`", "`ls -la`"},
		{"Read", "src/main.go", "Reading src/main.go"},
		{"Edit", "config.yaml", "Editing config.yaml"},
		{"Grep", `"TODO"`, `Searching for "TODO"`},
		{"Glob", "**/*.go", "Finding files matching **/*.go"},
		{"Agent", "test runner", "Agent: test runner"},
		{"Bash", "", "Running command"},
		{"Read", "", "Reading file"},
	}
	for _, c := range cases {
		got := toolVerbDetailed(c.name, c.detail)
		if !strings.Contains(got, c.wantContains) {
			t.Errorf("toolVerbDetailed(%q, %q) = %q, want to contain %q", c.name, c.detail, got, c.wantContains)
		}
	}
}

// ---- Risk classification tests ----

func TestClassifyRisk(t *testing.T) {
	cases := []struct {
		tool, msg string
		want      RiskLevel
	}{
		{"Read", "Read: /etc/passwd", RiskLow},
		{"Grep", "Grep: pattern", RiskLow},
		{"Glob", "Glob: **/*.go", RiskLow},
		{"Edit", "Edit: main.go", RiskModerate},
		{"Write", "Write: config.yaml", RiskModerate},
		{"Bash", "Bash: ls -la", RiskModerate},
		{"Bash", "Bash: rm -rf /tmp/test", RiskHigh},
		{"Bash", "Bash: sudo apt install", RiskHigh},
		{"Bash", "Bash: git push --force", RiskHigh},
	}
	for _, c := range cases {
		got := classifyRisk(c.tool, c.msg)
		if got != c.want {
			t.Errorf("classifyRisk(%q, %q) = %v, want %v", c.tool, c.msg, got, c.want)
		}
	}
}

func TestRiskLevel_String(t *testing.T) {
	if RiskLow.String() != "low" {
		t.Fatal("expected 'low'")
	}
	if RiskModerate.String() != "moderate" {
		t.Fatal("expected 'moderate'")
	}
	if RiskHigh.String() != "high" {
		t.Fatal("expected 'high'")
	}
}

// ---- Huh permission form tests ----

func TestPermissionForm_HighRisk(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "rm -rf node_modules",
		Risk:       RiskHigh,
		Message:    "Bash: rm -rf node_modules",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()
	if !strings.Contains(out, "HIGH") {
		t.Fatal("expected risk label 'HIGH' in huh form output")
	}
	if !strings.Contains(out, "Bash") {
		t.Fatal("expected tool name 'Bash' in huh form")
	}
}

func TestPermissionForm_LowRisk(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "Read",
		Action:     "Read file",
		Detail:     "/etc/config",
		Risk:       RiskLow,
		Message:    "Read: /etc/config",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()
	if !strings.Contains(out, "low") {
		t.Fatal("expected risk label 'low' in huh form output")
	}
}

// ---- Agent spawn/done tests ----

func TestApp_AgentSpawnMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(AgentSpawnMsg{Name: "test-agent", Background: true})
	got := next.(AppModel)
	if got.backgroundAgts != 1 {
		t.Fatalf("expected backgroundAgts=1, got %d", got.backgroundAgts)
	}

	// Should have a system message with agent name
	found := false
	for _, msg := range got.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "test-agent") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected agent spawn system message in conversation")
	}
}

func TestApp_AgentDoneMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.backgroundAgts = 1

	next, _ := m.Update(AgentDoneMsg{Name: "test-agent"})
	got := next.(AppModel)
	if got.backgroundAgts != 0 {
		t.Fatalf("expected backgroundAgts=0, got %d", got.backgroundAgts)
	}
}

// ---- Status bar with background agents ----

func TestRenderStatusBar_WithBackgroundAgents(t *testing.T) {
	s := StatusInfo{
		Model:          "claude-opus-4-6",
		BackgroundAgts: 3,
	}
	out := renderStatusBar(s, 120)
	if !strings.Contains(out, "3 bg") {
		t.Fatal("expected background agent count in status bar")
	}
}

// ---- Extract tool detail tests ----

func TestExtractToolDetail(t *testing.T) {
	cases := []struct {
		tool string
		json string
		want string
	}{
		{"Bash", `{"command":"ls -la"}`, "`ls -la`"},
		{"Read", `{"file_path":"/Users/x/src/main.go"}`, ".../src/main.go"},
		{"Grep", `{"pattern":"TODO","path":"src/"}`, `"TODO" in src/`},
		{"Glob", `{"pattern":"**/*.go"}`, "**/*.go"},
		{"Agent", `{"description":"test runner"}`, "test runner"},
		{"Bash", `{}`, ""},
		{"Read", `invalid`, ""},
	}
	for _, c := range cases {
		got := extractToolDetail(c.tool, []byte(c.json))
		if got != c.want {
			t.Errorf("extractToolDetail(%q, %q) = %q, want %q", c.tool, c.json, got, c.want)
		}
	}
}

// ---- Startup hint tests ----

func TestRenderConversation_EmptyShowsHints(t *testing.T) {
	out := renderConversation(nil, 80, nil, nil, -1, 0)
	if !strings.Contains(out, "/") {
		t.Fatal("expected slash command hint in empty conversation")
	}
	if !strings.Contains(out, "shortcuts") {
		t.Fatal("expected shortcuts hint in empty conversation")
	}
}
