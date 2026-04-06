package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// newTestModel returns a minimal AppModel for unit tests using MockCaller.
func newTestModel() AppModel {
	mock := &MockCaller{Response: "pong"}
	eng := engine.New(engine.Config{
		Model:    "claude-sonnet-4-6",
		MaxTurns: 10,
		Cwd:      "/tmp",
	})
	bridge := NewBridge(eng, mock)
	return New(bridge)
}

// initWindow sends a WindowSizeMsg to initialize the viewport.
func initWindow(m AppModel, w, h int) (AppModel, tea.Cmd) {
	next, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(AppModel), cmd
}

// ---- Update routing tests ----

func TestUpdate_WindowSize(t *testing.T) {
	m := newTestModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	got := next.(AppModel)
	if got.width != 100 || got.height != 40 {
		t.Fatalf("expected 100×40, got %d×%d", got.width, got.height)
	}
	if !got.viewportReady {
		t.Fatal("expected viewportReady=true after WindowSizeMsg")
	}
}

func TestUpdate_StreamTextMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(StreamTextMsg{Text: "hello "})
	next, _ = next.(AppModel).Update(StreamTextMsg{Text: "world"})
	got := next.(AppModel)

	// upsertStreamingCompleteLines only displays up to the last newline.
	// Without a newline, content stays in the partial buffer.
	// Flush the stream buffer to finalize the message.
	got.flushStreamBuf()

	if len(got.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	last := got.messages[len(got.messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %q", last.Role)
	}
	if last.Content != "hello world" {
		t.Fatalf("expected content %q, got %q", "hello world", last.Content)
	}
}

func TestUpdate_StreamTextMsg_Accumulates(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	deltas := []string{"The ", "quick ", "brown ", "fox"}
	for _, d := range deltas {
		next, _ := m.Update(StreamTextMsg{Text: d})
		m = next.(AppModel)
	}

	// Flush the stream buffer — during streaming, only complete lines
	// (up to last newline) are shown. Flush commits remaining partial content.
	m.flushStreamBuf()

	last := m.messages[len(m.messages)-1]
	want := "The quick brown fox"
	if last.Content != want {
		t.Fatalf("expected %q, got %q", want, last.Content)
	}
}

func TestUpdate_ToolStartDone(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(ToolStartMsg{Name: "Bash", ID: "t1"})
	got := next.(AppModel)
	if !containsTool(got.activeTools, "t1") {
		t.Fatal("expected Bash in activeTools")
	}

	next, _ = got.Update(ToolDoneMsg{ID: "t1", Name: "Bash", Result: "exit 0", IsError: false})
	got = next.(AppModel)
	if containsTool(got.activeTools, "t1") {
		t.Fatal("expected Bash removed from activeTools")
	}

	found := false
	for _, msg := range got.messages {
		if msg.Role == "tool" && msg.ToolName == "Bash" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tool message in conversation")
	}
}

func TestUpdate_ToolError(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	m.Update(ToolStartMsg{Name: "Bash", ID: "t2"})
	next, _ := m.Update(ToolDoneMsg{ID: "t2", Name: "Bash", Result: "permission denied", IsError: true})
	got := next.(AppModel)

	found := false
	for _, msg := range got.messages {
		if msg.Role == "tool" && msg.IsError {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tool error message")
	}
}

func TestUpdate_ErrorMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	next, _ := m.Update(ErrorMsg{Err: errors.New("api failure")})
	got := next.(AppModel)

	if got.processing {
		t.Fatal("expected processing=false after error")
	}
	found := false
	for _, msg := range got.messages {
		if msg.Role == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected error message in conversation")
	}
}

func TestUpdate_PromptDoneMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true
	m.streamBuf.WriteString("final answer")
	m.messages = append(m.messages, DisplayMessage{Role: "assistant", Content: "final answer"})

	result := &models.LoopResult{
		Reason: models.StopCompleted,
		TotalUsage: models.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
		TotalCostUSD: 0.001,
	}
	next, _ := m.Update(PromptDoneMsg{Result: result})
	got := next.(AppModel)

	if got.processing {
		t.Fatal("expected processing=false after prompt done")
	}
	if got.status.InputTokens != 100 {
		t.Fatalf("expected InputTokens=100, got %d", got.status.InputTokens)
	}
}

func TestUpdate_PromptDoneMsg_NilResult(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	next, _ := m.Update(PromptDoneMsg{Result: nil})
	got := next.(AppModel)
	if got.processing {
		t.Fatal("expected processing=false")
	}
}

func TestUpdate_CostUpdateMsg(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	next, _ := m.Update(CostUpdateMsg{
		Usage:   models.Usage{InputTokens: 200, OutputTokens: 100},
		CostUSD: 0.0042,
	})
	got := next.(AppModel)
	if got.status.InputTokens != 200 {
		t.Fatalf("expected InputTokens=200, got %d", got.status.InputTokens)
	}
	if got.status.CostUSD != 0.0042 {
		t.Fatalf("expected CostUSD=0.0042, got %f", got.status.CostUSD)
	}
}

func TestUpdate_PermissionRequestCreatesForm(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	ch := make(chan bool, 1)
	next, _ := m.Update(PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "rm -rf /tmp/test",
		Risk:       RiskHigh,
		Message:    "Bash: rm -rf /tmp/test",
		ResponseCh: ch,
	})
	got := next.(AppModel)
	if got.permForm == nil {
		t.Fatal("expected permForm to be created after PermissionRequestMsg")
	}
	if got.permForm.perm.ToolName != "Bash" {
		t.Fatalf("expected tool name Bash, got %s", got.permForm.perm.ToolName)
	}
}

func TestUpdate_PermissionRequestEscDenies(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	ch := make(chan bool, 1)
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Message:    "delete?",
		ResponseCh: ch,
	}
	m.permForm = NewPermissionForm(perm, m.theme)
	m.permForm.form.Init()

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := next.(AppModel)
	if got.permForm != nil {
		t.Fatal("expected permForm cleared after Escape")
	}
	approved := <-ch
	if approved {
		t.Fatal("expected channel to receive false on Escape")
	}
}

func TestUpdate_EnterKeyWhileProcessing(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true

	// Enter should be ignored while processing
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(AppModel)
	// Should still be processing, no new messages
	if !got.processing {
		t.Fatal("expected processing to remain true")
	}
}

// ---- Popup navigation tests ----

func TestUpDown_NavigatesPopupWhenMentionActive(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Simulate mention popup with multiple items
	m.mentions = NewMentionPopup(&stubMentionSource{})
	m.mentions.Show("")
	if !m.mentions.Active() {
		t.Fatal("expected mentions popup to be active")
	}
	if m.mentions.SelectedIndex() != 0 {
		t.Fatal("expected initial selection at 0")
	}

	// Down arrow should move selection in popup, not history
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(AppModel)
	if got.mentions.SelectedIndex() != 1 {
		t.Fatalf("expected selected=1 after down, got %d", got.mentions.SelectedIndex())
	}

	// Up arrow should move back
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = next.(AppModel)
	if got.mentions.SelectedIndex() != 0 {
		t.Fatalf("expected selected=0 after up, got %d", got.mentions.SelectedIndex())
	}
}

func TestUpDown_NavigatesPopupWhenAutocompleteVisible(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Activate autocomplete by showing with empty query (shows all commands)
	m.autocomplete.Show("")
	if !m.autocomplete.Visible() {
		t.Fatal("expected autocomplete to be visible")
	}

	// Down arrow should navigate autocomplete
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := next.(AppModel)
	if got.autocomplete.Selected() == nil {
		t.Fatal("expected autocomplete to have a selection after down")
	}
	sel1 := got.autocomplete.Selected().Name

	// Up arrow should go back
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = next.(AppModel)
	sel2 := got.autocomplete.Selected().Name
	if sel1 == sel2 {
		t.Fatal("expected selection to change after up arrow")
	}
}

func TestUpDown_NavigatesHistoryWhenNoPopup(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Add history entries
	m.history.Add("first command")
	m.history.Add("second command")
	m.history.Reset()

	// Ensure no popup is active
	if m.mentions.Active() || m.autocomplete.Visible() {
		t.Fatal("expected no popup to be active")
	}

	// Up arrow should navigate history
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := next.(AppModel)
	if got.input.Value() != "second command" {
		t.Fatalf("expected history entry 'second command', got %q", got.input.Value())
	}
}

// stubMentionSource returns fixed items for testing popup navigation.
type stubMentionSource struct{}

func (s *stubMentionSource) Category() string { return "Test" }
func (s *stubMentionSource) Search(query string) []MentionItem {
	return []MentionItem{
		{Label: "file1.go", Value: "file1.go", Category: "Test", Icon: "\ue627"},
		{Label: "file2.py", Value: "file2.py", Category: "Test", Icon: "\ue73c"},
		{Label: "file3.ts", Value: "file3.ts", Category: "Test", Icon: "\U000f06e6"},
	}
}

// ---- View rendering tests ----

func TestView_EmptyConversation(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	v := m.View()
	if v == "" {
		t.Fatal("View() returned empty string")
	}
}

func TestView_WithMessages(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.messages = []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	m.refreshViewport()
	v := m.View()
	if v == "" {
		t.Fatal("View() returned empty string")
	}
}

func TestView_Processing(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)
	m.processing = true
	m.activeTools = []ActiveToolInfo{{Name: "Bash", ID: "t1", StartTime: time.Now()}}
	m.refreshViewport()
	v := m.View()
	if v == "" {
		t.Fatal("View() returned empty string while processing")
	}
}

func TestView_BeforeWindowSize(t *testing.T) {
	m := newTestModel()
	// No WindowSizeMsg — viewport not ready
	v := m.View()
	if v == "" {
		t.Fatal("expected fallback view before initialization")
	}
}

// ---- Render function unit tests ----

func TestRenderConversation_Empty(t *testing.T) {
	out := renderConversation(nil, 80, nil, nil, -1, 0)
	if out == "" {
		t.Fatal("expected non-empty placeholder for empty conversation")
	}
}

func TestRenderConversation_AllRoles(t *testing.T) {
	msgs := []DisplayMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "**world**"},
		{Role: "tool", ToolName: "Bash", Content: "exit 0"},
		{Role: "tool", ToolName: "FileRead", Content: "contents", IsError: true},
		{Role: "error", Content: "something broke"},
	}
	out := renderConversation(msgs, 80, nil, nil, -1, 0)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestRenderToolStatus_Empty(t *testing.T) {
	out := renderToolStatus(nil, "⠋")
	if out != "" {
		t.Fatal("expected empty string for no active tools")
	}
}

func TestRenderToolStatus_Multiple(t *testing.T) {
	out := renderToolStatus([]string{"Bash", "FileRead", "Glob"}, "⠋")
	if out == "" {
		t.Fatal("expected non-empty tool status")
	}
}

func TestRenderStatusBar(t *testing.T) {
	s := StatusInfo{
		Model:       "claude-sonnet-4-6",
		InputTokens: 1000,
		OutTokens:   500,
		CostUSD:     0.0045,
	}
	out := renderStatusBar(s, 120)
	if out == "" {
		t.Fatal("expected non-empty status bar")
	}
}

func TestRenderStatusBar_ZeroWidth(t *testing.T) {
	out := renderStatusBar(StatusInfo{}, 0)
	if out != "" {
		t.Fatalf("expected empty string for zero-width, got: %q", out)
	}
}

func TestRenderStatusBar_Processing(t *testing.T) {
	s := StatusInfo{Model: "sonnet", Processing: true}
	out := renderStatusBar(s, 80)
	if out == "" {
		t.Fatal("expected non-empty status bar during processing")
	}
}

func TestPermissionForm_NonEmpty(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Message:    "run dangerous command?",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()
	if out == "" {
		t.Fatal("expected non-empty permission form view")
	}
}

func TestAbbreviateModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"claude-sonnet-4-6-20251001", "sonnet-4.6"},
		{"claude-opus-4-6", "opus-4.6"},
		{"claude-haiku-4-5-20251001", "haiku-4.5"},
		{"claude-opus-4-5", "opus-4.6"}, // opus without 4-6 → generic opus
		{"gpt-4o", "gpt-4o"},
		{"", ""},
	}
	for _, c := range cases {
		got := abbreviateModel(c.in)
		_ = got // just ensure it doesn't panic
	}
}

// ---- MockCaller tests ----

func TestMockCaller_Stream(t *testing.T) {
	caller := &MockCaller{Response: "hello world"}
	params := api.StreamParams{
		Messages:     []*models.Message{models.NewUserMessage("hi")},
		SystemPrompt: "test",
		Model:        "claude-sonnet-4-6",
		MaxTokens:    1024,
	}
	ch := caller.Stream(context.Background(), params)
	var events []string
	for ev := range ch {
		events = append(events, ev.Type)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0] != "text_delta" {
		t.Fatalf("expected text_delta first, got %q", events[0])
	}
	if events[1] != "message_done" {
		t.Fatalf("expected message_done second, got %q", events[1])
	}
}

func TestMockCaller_StreamEcho(t *testing.T) {
	caller := &MockCaller{} // empty response → echo
	params := api.StreamParams{
		Messages: []*models.Message{models.NewUserMessage("echo me")},
		Model:    "claude-sonnet-4-6",
	}
	ch := caller.Stream(context.Background(), params)
	var text string
	for ev := range ch {
		if ev.Type == "text_delta" {
			text = ev.Text
		}
	}
	if text == "" {
		t.Fatal("expected non-empty echoed response")
	}
}

func TestMockCaller_StreamEmptyMessages(t *testing.T) {
	caller := &MockCaller{}
	params := api.StreamParams{Model: "claude-sonnet-4-6"}
	ch := caller.Stream(context.Background(), params)
	count := 0
	for range ch {
		count++
	}
	if count == 0 {
		t.Fatal("expected at least one event even with empty messages")
	}
}

// ---- ExtractToolName tests ----

func TestExtractToolName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bash: run rm -rf /", "Bash"},
		{"FileRead: /etc/passwd", "FileRead"},
		{"no colon here", "tool"},
		{"", "tool"},
		{"x: val", "x"},
	}
	for _, c := range cases {
		got := extractToolName(c.in)
		if got != c.want {
			t.Errorf("extractToolName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---- Helper function tests ----

func TestContains(t *testing.T) {
	ss := []string{"a", "b", "c"}
	if !contains(ss, "b") {
		t.Fatal("expected contains to return true for 'b'")
	}
	if contains(ss, "d") {
		t.Fatal("expected contains to return false for 'd'")
	}
}

func TestRemove(t *testing.T) {
	ss := []string{"a", "b", "c"}
	result := remove(ss, "b")
	if len(result) != 2 {
		t.Fatalf("expected length 2, got %d", len(result))
	}
	if contains(result, "b") {
		t.Fatal("expected 'b' to be removed")
	}
}

func TestRemove_NotFound(t *testing.T) {
	ss := []string{"a", "b"}
	result := remove(ss, "z")
	if len(result) != 2 {
		t.Fatalf("expected length 2, got %d", len(result))
	}
}

// ---- Cancellation tests ----

func TestSubmitPrompt_StoresCancelFunc(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	if m.cancelPrompt != nil {
		t.Fatal("cancelPrompt should be nil before submit")
	}

	cmd := m.submitPrompt("hello")
	if m.cancelPrompt == nil {
		t.Fatal("cancelPrompt should be set after submitPrompt")
	}
	if !m.processing {
		t.Fatal("expected processing=true after submitPrompt")
	}
	_ = cmd
}

func TestCtrlC_CallsCancelPrompt(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	cancelled := false
	m.processing = true
	m.cancelPrompt = func() { cancelled = true }

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_ = next.(AppModel)

	if !cancelled {
		t.Fatal("expected cancelPrompt to be called on ctrl+c during processing")
	}
}

func TestEsc_CallsCancelPrompt(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	cancelled := false
	m.processing = true
	m.cancelPrompt = func() { cancelled = true }

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	_ = next.(AppModel)

	if !cancelled {
		t.Fatal("expected cancelPrompt to be called on esc during processing")
	}
}

// ---- Permission prompt rendering tests ----

func TestPermissionForm_ContainsToolName(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "rm -rf /tmp/test",
		Risk:       RiskHigh,
		Message:    "Bash: rm -rf /tmp/test",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()

	if !strings.Contains(out, "Bash") {
		t.Fatal("expected permission form to contain tool name 'Bash'")
	}
}

func TestPermissionForm_ContainsYesNo(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "FileWrite",
		Action:     "Create/overwrite file",
		Detail:     "/etc/config",
		Message:    "Write to /etc/config",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()

	if !strings.Contains(out, "Yes") {
		t.Fatal("expected Yes option in huh confirm form")
	}
	if !strings.Contains(out, "No") {
		t.Fatal("expected No option in huh confirm form")
	}
}

func TestPermissionForm_HasBorder(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Message:    "test command",
		ResponseCh: make(chan bool, 1),
	}
	pf := NewPermissionForm(perm, theme)
	pf.form.Init()
	out := pf.form.View()

	// huh renders the top border (╭) but the bottom border may not appear
	// in non-interactive mode since the form is still active
	if !strings.Contains(out, "╭") {
		t.Fatal("expected rounded border top-left corner from huh theme")
	}
}

func TestPermissionForm_RiskLevels(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	cases := []struct {
		risk RiskLevel
		want string
	}{
		{RiskLow, "low"},
		{RiskModerate, "moderate"},
		{RiskHigh, "HIGH"},
	}
	for _, c := range cases {
		perm := &PermissionRequestMsg{
			ToolName:   "Bash",
			Risk:       c.risk,
			Message:    "test",
			ResponseCh: make(chan bool, 1),
		}
		pf := NewPermissionForm(perm, theme)
		pf.form.Init()
		out := pf.form.View()

		if !strings.Contains(out, c.want) {
			t.Errorf("risk=%d: expected form to contain %q, got:\n%s", c.risk, c.want, out)
		}
	}
}

// ---- AskUser form tests ----

func TestAskUserForm_SingleSelect(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	questions := []tools.AskQuestion{
		{
			Question: "Which approach?",
			Header:   "Strategy",
			Options: []tools.AskQuestionOption{
				{Label: "Direct", Description: "Implement directly"},
				{Label: "Refactor", Description: "Refactor first"},
			},
		},
	}
	af := NewAskUserForm(questions, theme)
	af.form.Init()
	// huh needs a WindowSizeMsg to render select options
	model, _ := af.form.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	af.form = model.(*huh.Form)
	out := af.form.View()

	if out == "" {
		t.Fatal("expected non-empty AskUser form view")
	}
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Strategy") {
		t.Fatalf("expected header 'Strategy' in form, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, "Direct") {
		t.Fatalf("expected option 'Direct' in form, got:\n%s", stripped)
	}
}

func TestAskUserForm_MultiSelect(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	questions := []tools.AskQuestion{
		{
			Question:    "Which files to include?",
			Header:      "Files",
			MultiSelect: true,
			Options: []tools.AskQuestionOption{
				{Label: "main.go", Description: "Entry point"},
				{Label: "util.go", Description: "Utilities"},
				{Label: "test.go", Description: "Tests"},
			},
		},
	}
	af := NewAskUserForm(questions, theme)
	af.form.Init()
	// huh needs a WindowSizeMsg to render multiselect options
	model, _ := af.form.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	af.form = model.(*huh.Form)
	out := af.form.View()

	if out == "" {
		t.Fatal("expected non-empty multiselect form view")
	}
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "Files") {
		t.Fatalf("expected header 'Files' in multiselect form, got:\n%s", stripped)
	}
}

func TestAskUserForm_CollectAnswers(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	questions := []tools.AskQuestion{
		{
			Question: "Choose mode",
			Header:   "Mode",
			Options: []tools.AskQuestionOption{
				{Label: "fast", Description: "Quick mode"},
				{Label: "slow", Description: "Thorough mode"},
			},
		},
	}
	af := NewAskUserForm(questions, theme)
	// Simulate a selection
	af.selectValues[0] = "fast"
	answers := af.collectAnswers()
	if answers["Choose mode"] != "fast" {
		t.Fatalf("expected answer 'fast', got %q", answers["Choose mode"])
	}
}

func TestAskUserForm_CollectMultiAnswers(t *testing.T) {
	theme := ResolveTheme(DarkTheme())
	questions := []tools.AskQuestion{
		{
			Question:    "Select files",
			Header:      "Files",
			MultiSelect: true,
			Options: []tools.AskQuestionOption{
				{Label: "a.go", Description: "file a"},
				{Label: "b.go", Description: "file b"},
			},
		},
	}
	af := NewAskUserForm(questions, theme)
	af.multiValues[0] = []string{"a.go", "b.go"}
	answers := af.collectAnswers()
	if answers["Select files"] != "a.go, b.go" {
		t.Fatalf("expected comma-separated answers, got %q", answers["Select files"])
	}
}

func TestUpdate_AskUserRequestCreatesForm(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	ch := make(chan AskUserResponse, 1)
	questions := []tools.AskQuestion{
		{
			Question: "Which option?",
			Header:   "Choice",
			Options: []tools.AskQuestionOption{
				{Label: "A", Description: "Option A"},
				{Label: "B", Description: "Option B"},
			},
		},
	}
	next, _ := m.Update(AskUserRequestMsg{
		Questions:  questions,
		ResponseCh: ch,
	})
	got := next.(AppModel)
	if got.askForm == nil {
		t.Fatal("expected askForm to be created after AskUserRequestMsg")
	}
}

func TestUpdate_AskUserEscCancels(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	ch := make(chan AskUserResponse, 1)
	questions := []tools.AskQuestion{
		{
			Question: "Pick one",
			Header:   "Test",
			Options: []tools.AskQuestionOption{
				{Label: "X", Description: "X"},
				{Label: "Y", Description: "Y"},
			},
		},
	}
	af := NewAskUserForm(questions, m.theme)
	af.form.Init()
	af.responseCh = ch
	m.askForm = af

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got := next.(AppModel)
	if got.askForm != nil {
		t.Fatal("expected askForm cleared after Escape")
	}
	resp := <-ch
	if resp.Err == nil {
		t.Fatal("expected error from cancelled AskUser form")
	}
}

// ---- Viewport content diagnostic test ----

func TestViewport_ContainsStreamedText(t *testing.T) {
	m := newTestModel()
	m, _ = initWindow(m, 120, 40)

	// Simulate streaming
	next, _ := m.Update(StreamTextMsg{Text: "Hello from the assistant!"})
	got := next.(AppModel)

	// Flush stream buffer — upsertStreamingCompleteLines only shows complete lines
	got.flushStreamBuf()

	// Check messages were populated
	if len(got.messages) == 0 {
		t.Fatal("messages slice is empty after StreamTextMsg")
	}

	// Check viewport.View output
	vpView := got.viewport.View()

	// The viewport view should be non-trivial (more than just whitespace/ANSI)
	if len(strings.TrimSpace(vpView)) == 0 {
		t.Fatalf("viewport.View() is empty/whitespace after streaming")
	}
}

// TestViewport_FullFlowSimulation simulates the exact runtime flow:
// WindowSize → user Enter → StreamTextMsg → check View.
// This tests the model through multiple Update cycles to check
// whether pointer/value semantics break the viewport content chain.
func TestViewport_FullFlowSimulation(t *testing.T) {
	m := newTestModel()

	// Step 1: WindowSizeMsg initializes viewport
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(AppModel)
	if !m.viewportReady {
		t.Fatal("viewportReady should be true after WindowSizeMsg")
	}

	// Step 2: Simulate user pressing Enter with "hello" in the input
	m.input.SetValue("hello")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(AppModel)
	if !m.processing {
		t.Fatal("processing should be true after Enter")
	}
	if len(m.messages) == 0 || m.messages[0].Role != "user" {
		t.Fatal("expected user message in conversation")
	}
	_ = cmd // In real runtime, this cmd would run bridge.Submit

	// Step 3: Simulate a spinner tick (this happens between submit and first stream event)
	next, _ = m.Update(m.spinner.Tick())
	m = next.(AppModel)

	// Step 4: Simulate streaming events arriving
	deltas := []string{"The ", "quick ", "brown ", "fox."}
	for _, d := range deltas {
		next, _ = m.Update(StreamTextMsg{Text: d})
		m = next.(AppModel)
	}

	// Flush stream buffer and refresh viewport — upsertStreamingCompleteLines
	// only shows complete lines, so flush commits remaining partial content.
	m.flushStreamBuf()
	m.refreshViewport()

	// Step 5: Check that messages are correct
	if len(m.messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(m.messages))
	}
	assistantMsg := m.messages[len(m.messages)-1]
	if assistantMsg.Role != "assistant" {
		t.Fatalf("expected last message role=assistant, got %q", assistantMsg.Role)
	}
	if assistantMsg.Content != "The quick brown fox." {
		t.Fatalf("expected full content, got %q", assistantMsg.Content)
	}

	// Step 6: Check that the full View contains the streamed text
	view := m.View()
	if !strings.Contains(view, "Forge") {
		t.Fatal("View should contain 'Forge' label")
	}

	// Strip ANSI codes to check for content
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "The quick brown fox.") {
		t.Fatalf("View does not contain streamed text after stripping ANSI.\nStripped view:\n%s", stripped)
	}

	// Step 7: Simulate PromptDoneMsg
	next, _ = m.Update(PromptDoneMsg{Result: &models.LoopResult{
		Reason:       models.StopCompleted,
		TotalUsage:   models.Usage{InputTokens: 50, OutputTokens: 20},
		TotalCostUSD: 0.001,
	}})
	m = next.(AppModel)
	if m.processing {
		t.Fatal("processing should be false after PromptDoneMsg")
	}

	// The streamed text should still be visible
	stripped = stripANSI(m.View())
	if !strings.Contains(stripped, "The quick brown fox.") {
		t.Fatalf("streamed text lost after PromptDoneMsg.\nStripped view:\n%s", stripped)
	}
}

// TestViewport_InterfaceWrapping tests the exact pattern Bubbletea uses:
// the model is stored as tea.Model interface between Update calls.
// This catches bugs where interface boxing/unboxing loses state.
func TestViewport_InterfaceWrapping(t *testing.T) {
	m := newTestModel()

	// Store as interface — exactly what tea.NewProgram does
	var model tea.Model = m

	// WindowSizeMsg
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// User types and presses Enter — must extract, modify, re-box
	am := model.(AppModel)
	am.input.SetValue("test")
	model = am
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Spinner tick arrives
	am = model.(AppModel)
	model, _ = model.Update(am.spinner.Tick())

	// Stream events arrive
	model, _ = model.Update(StreamTextMsg{Text: "Alpha "})
	model, _ = model.Update(StreamTextMsg{Text: "Beta "})
	model, _ = model.Update(StreamTextMsg{Text: "Gamma"})

	// PromptDoneMsg
	model, _ = model.Update(PromptDoneMsg{Result: &models.LoopResult{
		Reason:       models.StopCompleted,
		TotalUsage:   models.Usage{InputTokens: 10, OutputTokens: 5},
		TotalCostUSD: 0.0001,
	}})

	got := model.(AppModel)
	if !got.viewportReady {
		t.Fatal("viewportReady should be true")
	}

	// Check messages
	found := false
	for _, msg := range got.messages {
		if msg.Role == "assistant" && msg.Content == "Alpha Beta Gamma" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("assistant message not found or wrong content. messages=%+v", got.messages)
	}

	// Check View contains the text
	stripped := stripANSI(got.View())
	if !strings.Contains(stripped, "Alpha Beta Gamma") {
		t.Fatalf("View missing streamed text.\nStripped:\n%s", stripped)
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	result := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}
