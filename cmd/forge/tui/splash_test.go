package tui

import (
	"strings"
	"testing"
)

func TestRenderSplash_ContainsForge(t *testing.T) {
	info := SplashInfo{
		Version: "v0.1.0",
		Model:   "claude-sonnet-4-6",
		Cwd:     "/tmp/project",
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Forge") {
		t.Fatalf("splash should contain 'Forge', got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsModel(t *testing.T) {
	info := SplashInfo{
		Version: "v0.1.0",
		Model:   "claude-sonnet-4-6",
		Cwd:     "/tmp/project",
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "sonnet") {
		t.Fatalf("splash should contain model name, got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsVersion(t *testing.T) {
	info := SplashInfo{
		Version: "v0.1.0",
		Model:   "claude-sonnet-4-6",
		Cwd:     "/tmp/project",
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "v0.1.0") {
		t.Fatalf("splash should contain version, got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsHints(t *testing.T) {
	info := SplashInfo{
		Version: "v0.1.0",
		Model:   "claude-sonnet-4-6",
		Cwd:     "/tmp/project",
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "shortcuts") {
		t.Fatalf("splash should contain shortcut hints, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "commands") {
		t.Fatalf("splash should contain commands hint, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "mentions") {
		t.Fatalf("splash should contain mentions hint, got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsCounts(t *testing.T) {
	info := SplashInfo{
		Version:      "v0.1.0",
		Model:        "claude-sonnet-4-6",
		Cwd:          "/tmp/project",
		CommandCount: 10,
		ToolCount:    23,
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "10 commands") {
		t.Fatalf("splash should contain command count, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "23 tools") {
		t.Fatalf("splash should contain tool count, got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsHammerArt(t *testing.T) {
	info := SplashInfo{
		Version: "v0.1.0",
		Model:   "claude-sonnet-4-6",
		Cwd:     "/tmp/project",
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "⣿") {
		t.Fatalf("splash should contain Mjolnir braille art, got:\n%s", stripped)
	}
}

func TestRenderSplash_ContainsSkillNames(t *testing.T) {
	info := SplashInfo{
		Version:    "v0.1.0",
		Model:      "claude-sonnet-4-6",
		Cwd:        "/tmp/project",
		SkillNames: []string{"/commit", "/review", "/forge", "/cost", "/help"},
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "5 skills loaded") {
		t.Fatalf("splash should contain skill count, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "/commit") {
		t.Fatalf("splash should list skill names, got:\n%s", stripped)
	}
}

func TestRenderSplash_SkillNamesTruncated(t *testing.T) {
	info := SplashInfo{
		Version:    "v0.1.0",
		Model:      "claude-sonnet-4-6",
		Cwd:        "/tmp/project",
		SkillNames: []string{"/a", "/b", "/c", "/d", "/e", "/f", "/g"},
	}
	theme := ResolveTheme(DarkTheme())
	out := renderSplash(info, 120, theme)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "7 skills loaded") {
		t.Fatalf("splash should show total skill count, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "...") {
		t.Fatalf("splash should truncate long skill lists with ..., got:\n%s", stripped)
	}
	// Should NOT contain /f or /g (only first 5)
	if strings.Contains(stripped, "/f") {
		t.Fatalf("splash should only show first 5 skills, got:\n%s", stripped)
	}
}

func TestSplashNotShownWithMessages(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "hello"},
	}
	splash := &SplashScreen{
		Info: SplashInfo{
			Version: "v0.1.0",
			Model:   "claude-sonnet-4-6",
			Cwd:     "/tmp",
		},
		Theme: ResolveTheme(DarkTheme()),
	}
	out := renderConversation(msgs, 80, nil, splash, -1, 0)
	stripped := stripANSI(out)
	if strings.Contains(stripped, "? for shortcuts") {
		t.Fatal("splash hints should not appear when messages exist")
	}
}

func TestSplashShownWhenEmpty(t *testing.T) {
	splash := &SplashScreen{
		Info: SplashInfo{
			Version: "v0.1.0",
			Model:   "claude-sonnet-4-6",
			Cwd:     "/tmp",
		},
		Theme: ResolveTheme(DarkTheme()),
	}
	out := renderConversation(nil, 80, nil, splash, -1, 0)
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Forge") {
		t.Fatalf("splash should be shown when no messages, got:\n%s", stripped)
	}
}

func TestShortenHome(t *testing.T) {
	got := shortenHome("/var/log")
	if got != "/var/log" {
		t.Fatalf("expected /var/log, got %s", got)
	}
}

// ---- Agent indicator tests ----

func TestAgentSpawnMsg_SystemMessage(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(AgentSpawnMsg{Name: "researcher", Background: true})
	got := next.(AppModel)

	found := false
	for _, msg := range got.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Agent spawned: researcher") {
			found = true
			if !strings.Contains(msg.Content, "(background)") {
				t.Fatal("expected (background) label for background agent")
			}
		}
	}
	if !found {
		t.Fatal("expected system message for agent spawn")
	}
}

func TestAgentSpawnMsg_ForegroundNoLabel(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(AgentSpawnMsg{Name: "helper", Background: false})
	got := next.(AppModel)

	for _, msg := range got.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "helper") {
			if strings.Contains(msg.Content, "(background)") {
				t.Fatal("foreground agent should not have (background) label")
			}
			return
		}
	}
	t.Fatal("expected system message for foreground agent spawn")
}

func TestAgentDoneMsg_SystemMessage(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.backgroundAgts = 1

	next, _ := m.Update(AgentDoneMsg{Name: "researcher"})
	got := next.(AppModel)

	found := false
	for _, msg := range got.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Agent completed: researcher") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected system message for agent completion")
	}
}

// ---- Skill invocation indicator test ----

func TestSkillInvocation_SystemMessage(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// /commit is not handled by the TUI, so it falls through to the engine
	// and should produce a skill indicator system message
	_ = m.submitPrompt("/commit")

	found := false
	for _, msg := range m.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Running /commit") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected skill invocation system message for /commit")
	}
}

// ---- System message rendering test ----

func TestRenderMessage_System(t *testing.T) {
	msg := DisplayMessage{
		Role:    "system",
		Content: "  ⚡ Running /commit...",
	}
	out := renderMessage(msg, 80)
	if out == "" {
		t.Fatal("expected non-empty system message rendering")
	}
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Running /commit") {
		t.Fatalf("expected system message content, got:\n%s", stripped)
	}
}

func TestSkillNames(t *testing.T) {
	reg := NewCommandRegistry()
	names := skillNames(reg)
	if len(names) == 0 {
		t.Fatal("expected at least one skill name from builtin commands")
	}
	foundHelp := false
	for _, n := range names {
		if n == "/help" {
			foundHelp = true
		}
	}
	if !foundHelp {
		t.Fatal("expected /help in skill names")
	}
}
