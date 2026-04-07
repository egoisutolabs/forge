package tui

import (
	"strings"
	"testing"
	"time"
)

// ---- Auto-approval shimmer tests ----

func TestShimmerState_New(t *testing.T) {
	s := NewShimmerState()
	if s.Active {
		t.Fatal("expected shimmer inactive on creation")
	}
	if s.Progress != 0 {
		t.Fatal("expected progress=0")
	}
}

func TestShimmerState_Trigger(t *testing.T) {
	s := NewShimmerState()
	s.Trigger("git status", "Bash")
	if !s.Active {
		t.Fatal("expected shimmer active after trigger")
	}
	if s.Command != "git status" {
		t.Fatalf("expected command %q, got %q", "git status", s.Command)
	}
	if s.ToolName != "Bash" {
		t.Fatalf("expected tool Bash, got %q", s.ToolName)
	}
}

func TestShimmerState_Advance(t *testing.T) {
	s := NewShimmerState()
	s.Trigger("git status", "Bash")

	// Advance partway
	done := s.Advance(0.5)
	if done {
		t.Fatal("expected not done at 50%")
	}
	if s.Progress != 0.5 {
		t.Fatalf("expected progress=0.5, got %f", s.Progress)
	}

	// Advance past 1.0 should complete
	done = s.Advance(0.6)
	if !done {
		t.Fatal("expected done when progress >= 1.0")
	}
	if s.Active {
		t.Fatal("expected shimmer inactive after completion")
	}
}

func TestShimmerState_Render(t *testing.T) {
	s := NewShimmerState()
	s.Trigger("git status", "Bash")
	s.Progress = 0.5

	rendered := s.Render(80)
	if rendered == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(rendered, "git status") {
		t.Fatal("expected render to contain command")
	}
}

func TestShimmerState_RenderInactive(t *testing.T) {
	s := NewShimmerState()
	rendered := s.Render(80)
	if rendered != "" {
		t.Fatal("expected empty render when inactive")
	}
}

// ---- Destructive command detection tests ----

func TestIsDestructiveCommand(t *testing.T) {
	tests := []struct {
		cmd         string
		destructive bool
		reason      string
	}{
		{"rm -rf /tmp/test", true, ""},
		{"rm file.txt", true, ""},
		{"git reset --hard", true, ""},
		{"git reset --hard HEAD~3", true, ""},
		{"git push --force", true, ""},
		{"git push -f origin main", true, ""},
		{"git clean -fd", true, ""},
		{"sudo rm -rf /", true, ""},
		{"git checkout -- .", true, ""},
		{"git restore .", true, ""},
		{"dd if=/dev/zero of=/dev/sda", true, ""},
		{"chmod 777 /etc/passwd", true, ""},
		{"kill -9 1234", true, ""},
		// Non-destructive
		{"git status", false, ""},
		{"git log", false, ""},
		{"ls -la", false, ""},
		{"echo hello", false, ""},
		{"cat file.txt", false, ""},
		{"go test ./...", false, ""},
		{"npm install", false, ""},
		{"git diff", false, ""},
	}

	for _, tt := range tests {
		got, reason := IsDestructiveCommand(tt.cmd)
		if got != tt.destructive {
			t.Errorf("IsDestructiveCommand(%q) = %v, want %v", tt.cmd, got, tt.destructive)
		}
		if got && reason == "" {
			t.Errorf("IsDestructiveCommand(%q) returned empty reason for destructive command", tt.cmd)
		}
	}
}

func TestDestructiveWarning_Render(t *testing.T) {
	w := DestructiveWarning{
		Command:    "rm -rf /",
		Reason:     "Recursive force delete can permanently remove files",
		ShowDetail: false,
	}

	rendered := w.Render(80)
	if !strings.Contains(rendered, "rm -rf") {
		t.Fatal("expected warning to contain command")
	}
	if !strings.Contains(rendered, "DESTRUCTIVE") || !strings.Contains(rendered, "destructive") {
		// Check for either case
		lower := strings.ToLower(rendered)
		if !strings.Contains(lower, "destructive") && !strings.Contains(lower, "danger") {
			t.Fatal("expected warning to contain destructive/danger label")
		}
	}
}

func TestDestructiveWarning_ToggleDetail(t *testing.T) {
	w := DestructiveWarning{
		Command:    "git reset --hard",
		Reason:     "Discards all uncommitted changes permanently",
		ShowDetail: false,
	}

	// Initially no detail
	r1 := w.Render(80)

	// Toggle detail on
	w.ShowDetail = true
	r2 := w.Render(80)

	if r1 == r2 {
		t.Fatal("expected render to change when detail toggled")
	}
	if !strings.Contains(r2, w.Reason) {
		t.Fatal("expected expanded render to contain reason")
	}
}

// ---- Editable rule prefix tests ----

func TestRulePrefix_ExtractFromCommand(t *testing.T) {
	tests := []struct {
		cmd    string
		prefix string
	}{
		{"git status", "git *"},
		{"git log --oneline", "git *"},
		{"npm install", "npm *"},
		{"go test ./tui/...", "go *"},
		{"ls -la", "ls *"},
		{"echo hello world", "echo *"},
		{"python3 script.py", "python3 *"},
	}

	for _, tt := range tests {
		got := ExtractRulePrefix(tt.cmd)
		if got != tt.prefix {
			t.Errorf("ExtractRulePrefix(%q) = %q, want %q", tt.cmd, got, tt.prefix)
		}
	}
}

func TestRulePrefix_MatchesCommand(t *testing.T) {
	tests := []struct {
		pattern string
		cmd     string
		matches bool
	}{
		{"git *", "git status", true},
		{"git *", "git log --oneline", true},
		{"git *", "npm install", false},
		{"npm *", "npm install", true},
		{"npm *", "npm run build", true},
		{"go test *", "go test ./...", true},
		{"go test *", "go build", false},
		{"*", "anything", true},
	}

	for _, tt := range tests {
		got := RulePrefixMatches(tt.pattern, tt.cmd)
		if got != tt.matches {
			t.Errorf("RulePrefixMatches(%q, %q) = %v, want %v", tt.pattern, tt.cmd, got, tt.matches)
		}
	}
}

func TestSessionRuleStore_AddAndMatch(t *testing.T) {
	store := NewSessionRuleStore()
	store.Add("Bash", "git *")

	if !store.Matches("Bash", "git status") {
		t.Fatal("expected rule to match git status")
	}
	if store.Matches("Bash", "npm install") {
		t.Fatal("expected rule not to match npm install")
	}
	if store.Matches("Edit", "git status") {
		t.Fatal("expected rule not to match different tool")
	}
}

func TestSessionRuleStore_MultipleRules(t *testing.T) {
	store := NewSessionRuleStore()
	store.Add("Bash", "git *")
	store.Add("Bash", "npm *")

	if !store.Matches("Bash", "git status") {
		t.Fatal("expected match for git")
	}
	if !store.Matches("Bash", "npm install") {
		t.Fatal("expected match for npm")
	}
	if store.Matches("Bash", "python script.py") {
		t.Fatal("expected no match for python")
	}
}

func TestSessionRuleStore_Rules(t *testing.T) {
	store := NewSessionRuleStore()
	store.Add("Bash", "git *")
	store.Add("Bash", "npm *")

	rules := store.Rules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
}

// ---- Question-style preview (AskUser enhancements) tests ----

func TestAskUserPreview_NumericQuickSelect(t *testing.T) {
	// Verify numeric key mapping for options 1-9
	for i := 1; i <= 9; i++ {
		idx := NumericKeyToIndex(rune('0' + i))
		if idx != i-1 {
			t.Errorf("NumericKeyToIndex(%d) = %d, want %d", i, idx, i-1)
		}
	}

	// '0' and other chars should return -1
	if NumericKeyToIndex('0') != -1 {
		t.Error("expected -1 for '0'")
	}
	if NumericKeyToIndex('a') != -1 {
		t.Error("expected -1 for 'a'")
	}
}

func TestAskUserPreview_OptionLabels(t *testing.T) {
	labels := FormatOptionLabels([]string{"Yes", "No", "Maybe"})
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}
	// Each label should have a numeric prefix
	if !strings.HasPrefix(labels[0], "1") {
		t.Fatalf("expected label to start with '1', got %q", labels[0])
	}
	if !strings.HasPrefix(labels[1], "2") {
		t.Fatalf("expected label to start with '2', got %q", labels[1])
	}
	if !strings.HasPrefix(labels[2], "3") {
		t.Fatalf("expected label to start with '3', got %q", labels[2])
	}
}

// ---- Enhanced PermissionForm tests ----

func TestPermissionForm_DestructiveWarningCreated(t *testing.T) {
	ch := make(chan bool, 1)
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "rm -rf /tmp/test",
		Risk:       RiskHigh,
		Message:    "Bash: rm -rf /tmp/test",
		ResponseCh: ch,
	}
	theme := ResolveTheme(DarkTheme())
	pf := NewPermissionForm(perm, theme)

	if pf.destructiveWarning == nil {
		t.Fatal("expected destructive warning for rm -rf command")
	}
	if pf.destructiveWarning.Command != "rm -rf /tmp/test" {
		t.Fatalf("expected command %q, got %q", "rm -rf /tmp/test", pf.destructiveWarning.Command)
	}
}

func TestPermissionForm_NoWarningForSafeCommand(t *testing.T) {
	ch := make(chan bool, 1)
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "git status",
		Risk:       RiskLow,
		Message:    "Bash: git status",
		ResponseCh: ch,
	}
	theme := ResolveTheme(DarkTheme())
	pf := NewPermissionForm(perm, theme)

	if pf.destructiveWarning != nil {
		t.Fatal("expected no destructive warning for git status")
	}
}

func TestPermissionForm_RulePrefixPresent(t *testing.T) {
	ch := make(chan bool, 1)
	perm := &PermissionRequestMsg{
		ToolName:   "Bash",
		Action:     "Execute command",
		Detail:     "git log --oneline",
		Risk:       RiskModerate,
		Message:    "Bash: git log --oneline",
		ResponseCh: ch,
	}
	theme := ResolveTheme(DarkTheme())
	pf := NewPermissionForm(perm, theme)

	if pf.rulePrefix != "git *" {
		t.Fatalf("expected rulePrefix 'git *', got %q", pf.rulePrefix)
	}
}

// ---- Shimmer animation message tests ----

func TestShimmerTickMsg(t *testing.T) {
	msg := ShimmerTickMsg(time.Now())
	_ = msg // Type check is sufficient
}

func TestAutoApprovalShimmerMsg(t *testing.T) {
	msg := AutoApprovalShimmerMsg{
		ToolName: "Bash",
		Command:  "git status",
	}
	if msg.ToolName != "Bash" {
		t.Fatal("expected ToolName=Bash")
	}
}
