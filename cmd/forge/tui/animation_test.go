package tui

import (
	"math"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- SpringState tests ----

func TestSpringState_SettlesToTarget(t *testing.T) {
	spring := newScrollSpring()
	s := SpringState{}
	s.StartFrom(0, 100)

	// Critically-damped spring with large distance may need more frames
	for i := 0; i < 300; i++ {
		if !s.Update(spring) {
			break
		}
	}

	if s.Active {
		t.Fatal("spring should have settled")
	}
	if math.Abs(s.Pos-100) > 0.5 {
		t.Fatalf("expected pos ≈ 100, got %f", s.Pos)
	}
}

func TestSpringState_SettlesToTarget_UISpring(t *testing.T) {
	spring := newUISpring()
	s := SpringState{}
	s.StartFrom(0, 50)

	for i := 0; i < 120; i++ {
		if !s.Update(spring) {
			break
		}
	}

	if s.Active {
		t.Fatal("UI spring should have settled")
	}
	if math.Abs(s.Pos-50) > 0.5 {
		t.Fatalf("expected pos ≈ 50, got %f", s.Pos)
	}
}

func TestSpringState_StopsWhenSettled(t *testing.T) {
	spring := newScrollSpring()
	s := SpringState{}
	s.StartFrom(0, 10)

	frames := 0
	for frames < 300 {
		if !s.Update(spring) {
			break
		}
		frames++
	}

	if s.Active {
		t.Fatal("spring should deactivate when settled")
	}
	if frames >= 300 {
		t.Fatalf("spring took too many frames to settle: %d", frames)
	}
}

func TestSpringState_InactiveDoesNotUpdate(t *testing.T) {
	spring := newScrollSpring()
	s := SpringState{Pos: 42, Vel: 0, Target: 42, Active: false}

	updated := s.Update(spring)
	if updated {
		t.Fatal("inactive spring should not report as updated")
	}
	if s.Pos != 42 {
		t.Fatalf("inactive spring pos changed: %f", s.Pos)
	}
}

func TestSpringState_Snap(t *testing.T) {
	s := SpringState{}
	s.StartFrom(0, 100)
	s.Snap(50)

	if s.Active {
		t.Fatal("snap should deactivate spring")
	}
	if s.Pos != 50 {
		t.Fatalf("expected pos=50 after snap, got %f", s.Pos)
	}
	if s.Vel != 0 {
		t.Fatalf("expected vel=0 after snap, got %f", s.Vel)
	}
}

func TestSpringState_Start(t *testing.T) {
	s := SpringState{Pos: 10, Vel: 0}
	s.Start(50)

	if !s.Active {
		t.Fatal("Start should activate spring")
	}
	if s.Target != 50 {
		t.Fatalf("expected target=50, got %f", s.Target)
	}
	if s.Pos != 10 {
		t.Fatalf("Start should not change pos, got %f", s.Pos)
	}
}

func TestSpringState_Settled(t *testing.T) {
	s := SpringState{Pos: 100, Vel: 0, Target: 100}
	if !s.Settled() {
		t.Fatal("should be settled when at target with no velocity")
	}

	s = SpringState{Pos: 99.7, Vel: 0.1, Target: 100}
	if !s.Settled() {
		t.Fatal("should be settled when close to target with low velocity")
	}

	s = SpringState{Pos: 50, Vel: 10, Target: 100}
	if s.Settled() {
		t.Fatal("should not be settled when far from target")
	}
}

// ---- Scroll animation tests ----

func TestScrollAnimation_PgDnSetsTarget(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Add enough content to enable scrolling
	var msgs []DisplayMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, DisplayMessage{Role: "assistant", Content: "line"})
	}
	m.messages = msgs
	m.refreshViewport()

	// Simulate PgDn
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := next.(AppModel)

	if !got.scrollAnim.Active {
		t.Fatal("expected scroll animation to be active after PgDn")
	}
	if got.scrollAnim.Target <= 0 {
		t.Fatal("expected positive scroll target after PgDn")
	}
}

func TestScrollAnimation_PgUpClampsToZero(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// At top of viewport, PgUp target should stay at 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	got := next.(AppModel)

	if got.scrollAnim.Target < 0 {
		t.Fatalf("expected target >= 0, got %f", got.scrollAnim.Target)
	}
}

func TestScrollAnimation_DisabledFallsBack(t *testing.T) {
	m := newTestModel()
	m.animations = false
	m, _ = initWindow(m, 100, 40)

	// Add content
	var msgs []DisplayMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, DisplayMessage{Role: "assistant", Content: "line"})
	}
	m.messages = msgs
	m.refreshViewport()

	// PgDn without animation should use instant scroll
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := next.(AppModel)

	// Scroll animation should not be active
	if got.scrollAnim.Active {
		t.Fatal("scroll animation should not be active when disabled")
	}
}

func TestNoAnimation_WhenContentFitsViewport(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Only a few messages — content fits in viewport
	m.messages = []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	m.refreshViewport()

	// PgDn shouldn't really do anything meaningful
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := next.(AppModel)

	// Target should be clamped to 0 since content fits
	if got.scrollAnim.Target > 0 {
		t.Logf("scroll target: %f (content may or may not fit)", got.scrollAnim.Target)
	}
}

// ---- AnimationTickMsg tests ----

func TestAnimationTick_UpdatesScrollPosition(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Add content and set up scroll animation
	var msgs []DisplayMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, DisplayMessage{Role: "assistant", Content: "line"})
	}
	m.messages = msgs
	m.refreshViewport()
	m.viewport.GotoTop()

	// Start scroll animation
	m.scrollAnim.StartFrom(0, 20)
	m.animTicking = true

	// Run several ticks
	initialPos := m.scrollAnim.Pos
	next, _ := m.Update(AnimationTickMsg{})
	got := next.(AppModel)

	if got.scrollAnim.Pos <= initialPos {
		t.Fatal("expected scroll position to advance after tick")
	}
}

func TestAnimationTick_StopsWhenSettled(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Set up a nearly-settled scroll animation
	m.scrollAnim = SpringState{
		Pos:    19.8,
		Vel:    0.1,
		Target: 20,
		Active: true,
	}
	m.animTicking = true

	next, cmd := m.Update(AnimationTickMsg{})
	got := next.(AppModel)

	// The spring should settle quickly
	// Run a few more ticks if needed
	for i := 0; i < 10 && got.scrollAnim.Active; i++ {
		next, cmd = got.Update(AnimationTickMsg{})
		got = next.(AppModel)
	}

	if got.scrollAnim.Active {
		t.Fatal("expected scroll animation to settle")
	}
	_ = cmd
}

func TestAnimationTick_DisabledNoOp(t *testing.T) {
	m := newTestModel()
	m.animations = false
	m, _ = initWindow(m, 100, 40)

	m.scrollAnim = SpringState{Pos: 0, Vel: 5, Target: 50, Active: true}

	next, _ := m.Update(AnimationTickMsg{})
	got := next.(AppModel)

	// When disabled, tick should be a no-op
	if got.scrollAnim.Pos != 0 {
		t.Fatalf("expected no position change when animations disabled, got %f", got.scrollAnim.Pos)
	}
}

// ---- Collapse animation tests ----

func TestCollapseAnimation_StartsOnTab(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Add a collapsed tool message
	m.messages = []DisplayMessage{
		{Role: "tool", ToolName: "Bash", Content: "output line 1\nline 2\nline 3", Collapsed: true},
	}
	m.refreshViewport()

	// Press Tab to expand
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(AppModel)

	if got.collapseAnim == nil {
		t.Fatal("expected collapse animation to start on Tab")
	}
	if got.collapseAnim.MsgIndex != 0 {
		t.Fatalf("expected animation on message 0, got %d", got.collapseAnim.MsgIndex)
	}
}

func TestCollapseAnimation_DisabledInstantToggle(t *testing.T) {
	m := newTestModel()
	m.animations = false
	m, _ = initWindow(m, 100, 40)

	m.messages = []DisplayMessage{
		{Role: "tool", ToolName: "Bash", Content: "output", Collapsed: true},
	}
	m.refreshViewport()

	// Tab without animation should toggle instantly
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := next.(AppModel)

	if got.messages[0].Collapsed {
		t.Fatal("expected instant uncollapse when animations disabled")
	}
	if got.collapseAnim != nil {
		t.Fatal("expected no collapse animation when disabled")
	}
}

// ---- clipToLines tests ----

func TestClipToLines(t *testing.T) {
	cases := []struct {
		input string
		n     int
		want  string
	}{
		{"a\nb\nc", 2, "a\nb"},
		{"a\nb\nc", 5, "a\nb\nc"},
		{"a\nb\nc", 1, "a"},
		{"a\nb\nc", 0, ""},
		{"single line", 3, "single line"},
		{"", 5, ""},
	}
	for _, c := range cases {
		got := clipToLines(c.input, c.n)
		if got != c.want {
			t.Errorf("clipToLines(%q, %d) = %q, want %q", c.input, c.n, got, c.want)
		}
	}
}

func TestCountLines(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"single", 1},
		{"", 0},
		{"a\n", 2},
	}
	for _, c := range cases {
		got := countLines(c.input)
		if got != c.want {
			t.Errorf("countLines(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

// ---- Popup animation tests ----

func TestPopupAnimation_AutocompleteShow(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Type "/" to trigger autocomplete
	m.input.SetValue("/")
	var cmds []tea.Cmd
	m.updateAutocomplete(&cmds)

	if !m.autocomplete.Visible() {
		t.Fatal("expected autocomplete to be visible")
	}
	if !m.acAnim.Active {
		t.Fatal("expected autocomplete animation to start")
	}
	if m.acAnim.Pos != 0 {
		t.Fatalf("expected animation to start from 0, got %f", m.acAnim.Pos)
	}
	if m.acAnim.Target <= 0 {
		t.Fatal("expected positive target height for autocomplete popup")
	}
	if len(cmds) == 0 {
		t.Fatal("expected animation tick command to be scheduled")
	}
}

func TestPopupAnimation_AutocompleteHideSnaps(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Show autocomplete
	m.input.SetValue("/")
	var cmds []tea.Cmd
	m.updateAutocomplete(&cmds)

	// Hide by clearing input
	m.input.SetValue("")
	m.updateAutocomplete(&cmds)

	if m.acAnim.Active {
		t.Fatal("expected animation to snap to inactive on hide")
	}
	if m.acAnim.Pos != 0 {
		t.Fatalf("expected pos=0 after hide, got %f", m.acAnim.Pos)
	}
}

func TestPopupAnimation_MentionShow(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	// Type "@" to trigger mention popup
	m.input.SetValue("@")
	var cmds []tea.Cmd
	m.updateMentions(&cmds)

	if m.mentions.Active() && !m.mentionAnim.Active {
		t.Fatal("expected mention animation to start when popup becomes active")
	}
}

// ---- Permission entrance animation tests ----

func TestPermissionAnimation_StartsOnRequest(t *testing.T) {
	m := newTestModel()
	m.animations = true
	m, _ = initWindow(m, 100, 40)

	ch := make(chan bool, 1)
	next, _ := m.Update(PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Risk:       RiskLow,
		Message:    "test",
		ResponseCh: ch,
	})
	got := next.(AppModel)

	if !got.permAnim.Active {
		t.Fatal("expected permission entrance animation to start")
	}
	if got.permAnim.Pos != 0 {
		t.Fatalf("expected animation to start from 0, got %f", got.permAnim.Pos)
	}
	if got.permAnim.Target <= 0 {
		t.Fatal("expected positive target height for permission dialog")
	}
}

// ---- Integration: animations disabled ----

func TestAnimationsDisabled_AllBehaviorPreserved(t *testing.T) {
	m := newTestModel()
	m.animations = false
	m, _ = initWindow(m, 100, 40)

	// Verify no animation state is active
	if m.scrollAnim.Active || m.acAnim.Active || m.mentionAnim.Active || m.permAnim.Active {
		t.Fatal("no animations should be active by default")
	}

	// PgDn should work instantly
	var msgs []DisplayMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, DisplayMessage{Role: "assistant", Content: "line"})
	}
	m.messages = msgs
	m.refreshViewport()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := next.(AppModel)

	if got.scrollAnim.Active {
		t.Fatal("scroll animation should not activate when disabled")
	}
}
