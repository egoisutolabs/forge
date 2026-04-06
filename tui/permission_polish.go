package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------- Auto-approval shimmer ----------

// ShimmerTickMsg is sent on each shimmer animation frame.
type ShimmerTickMsg time.Time

// AutoApprovalShimmerMsg is sent when a bash command is auto-approved,
// triggering a brief visual shimmer sweep before proceeding.
type AutoApprovalShimmerMsg struct {
	ToolName string
	Command  string
}

// ShimmerState tracks the horizontal shimmer sweep animation shown when
// a command is auto-approved. The sweep takes ~300ms.
type ShimmerState struct {
	Active   bool
	Progress float64 // 0.0 to 1.0
	Command  string
	ToolName string
}

// NewShimmerState creates an inactive shimmer state.
func NewShimmerState() *ShimmerState {
	return &ShimmerState{}
}

// Trigger starts the shimmer animation for an auto-approved command.
func (s *ShimmerState) Trigger(command, toolName string) {
	s.Active = true
	s.Progress = 0
	s.Command = command
	s.ToolName = toolName
}

// Advance moves the shimmer progress forward by delta (0.0-1.0).
// Returns true when the animation is complete.
func (s *ShimmerState) Advance(delta float64) bool {
	s.Progress += delta
	if s.Progress >= 1.0 {
		s.Progress = 1.0
		s.Active = false
		return true
	}
	return false
}

// Render draws the shimmer bar at the current progress.
// Returns empty string when inactive.
func (s *ShimmerState) Render(width int) string {
	if !s.Active {
		return ""
	}

	label := fmt.Sprintf(" ✓ Auto-approved: %s ", s.Command)
	if len(label) > width-4 {
		label = label[:width-7] + "..."
	}

	barWidth := width - 2
	if barWidth < 10 {
		barWidth = 10
	}

	// Calculate shimmer position (a bright band sweeping left to right)
	shimmerPos := int(s.Progress * float64(barWidth))
	shimmerWidth := barWidth / 5
	if shimmerWidth < 3 {
		shimmerWidth = 3
	}

	var bar strings.Builder
	for i := 0; i < barWidth; i++ {
		dist := i - shimmerPos
		if dist < 0 {
			dist = -dist
		}
		if dist <= shimmerWidth/2 {
			bar.WriteString("█")
		} else if dist <= shimmerWidth {
			bar.WriteString("▓")
		} else {
			bar.WriteString("░")
		}
	}

	dimStyle := lipgloss.NewStyle().Faint(true)
	return label + "\n" + dimStyle.Render(bar.String())
}

// shimmerTick returns a command that fires a ShimmerTickMsg at ~30fps (fast enough for shimmer).
func shimmerTick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg {
		return ShimmerTickMsg(t)
	})
}

// ---------- Destructive command detection ----------

// destructivePattern defines a pattern that flags a command as destructive.
type destructivePattern struct {
	Pattern string
	Reason  string
}

// destructivePatterns lists dangerous command patterns with explanations.
var destructivePatterns = []destructivePattern{
	{"rm -rf", "Recursive force delete can permanently remove files and directories"},
	{"rm -r ", "Recursive delete can permanently remove files and directories"},
	{"rm ", "Delete can permanently remove files"},
	{"git reset --hard", "Discards all uncommitted changes permanently"},
	{"git push --force", "Force push can overwrite remote history and lose others' work"},
	{"git push -f ", "Force push can overwrite remote history and lose others' work"},
	{"git clean -f", "Removes untracked files permanently"},
	{"git checkout -- ", "Discards uncommitted changes to files"},
	{"git restore .", "Discards all uncommitted changes in working directory"},
	{"sudo ", "Runs command with elevated privileges — mistakes are harder to undo"},
	{"dd ", "Raw disk write can overwrite entire disks or partitions"},
	{"mkfs", "Creates filesystem, destroying all existing data on device"},
	{"chmod ", "Changes file permissions which may affect system security"},
	{"chown ", "Changes file ownership which may affect system security"},
	{"kill ", "Terminates running processes"},
	{"> /", "Redirect may overwrite system files"},
}

// IsDestructiveCommand checks whether a command matches known destructive patterns.
// Returns (isDestructive, reason).
func IsDestructiveCommand(cmd string) (bool, string) {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	for _, dp := range destructivePatterns {
		if strings.Contains(lower, strings.ToLower(dp.Pattern)) {
			return true, dp.Reason
		}
	}
	return false, ""
}

// DestructiveWarning holds state for the red destructive command banner.
type DestructiveWarning struct {
	Command    string
	Reason     string
	ShowDetail bool // toggled by '?' key
}

// Render draws the destructive warning banner.
func (w *DestructiveWarning) Render(width int) string {
	bannerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("1")).
		Padding(0, 1).
		Width(width - 4)

	header := bannerStyle.Render("⚠ DESTRUCTIVE COMMAND")

	cmdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	cmdLine := cmdStyle.Render("  " + w.Command)

	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	b.WriteString(cmdLine)

	if w.ShowDetail {
		reasonStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Italic(true)
		b.WriteByte('\n')
		b.WriteString(reasonStyle.Render("  Why flagged: " + w.Reason))
	} else {
		hintStyle := lipgloss.NewStyle().Faint(true)
		b.WriteByte('\n')
		b.WriteString(hintStyle.Render("  Press '?' for details"))
	}

	return b.String()
}

// ---------- Editable rule prefixes ----------

// ExtractRulePrefix generates a wildcard pattern from a command.
// e.g., "git log --oneline" → "git *"
func ExtractRulePrefix(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "*"
	}
	return parts[0] + " *"
}

// RulePrefixMatches checks if a command matches a pattern.
// Patterns use trailing * as wildcard: "git *" matches "git status", "git log", etc.
// Multi-word prefixes work too: "go test *" matches "go test ./...".
func RulePrefixMatches(pattern, cmd string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return strings.HasPrefix(strings.ToLower(cmd), strings.ToLower(prefix))
	}

	// Exact match fallback
	return strings.EqualFold(pattern, cmd)
}

// SessionRule is a single session-scoped permission rule with a pattern.
type SessionRule struct {
	ToolName string
	Pattern  string
}

// SessionRuleStore holds editable session-scoped permission rules.
type SessionRuleStore struct {
	rules []SessionRule
}

// NewSessionRuleStore creates an empty rule store.
func NewSessionRuleStore() *SessionRuleStore {
	return &SessionRuleStore{}
}

// Add adds a new rule to the store.
func (s *SessionRuleStore) Add(toolName, pattern string) {
	s.rules = append(s.rules, SessionRule{ToolName: toolName, Pattern: pattern})
}

// Matches checks if any rule matches the given tool and command.
func (s *SessionRuleStore) Matches(toolName, cmd string) bool {
	for _, r := range s.rules {
		if r.ToolName == toolName && RulePrefixMatches(r.Pattern, cmd) {
			return true
		}
	}
	return false
}

// Rules returns a copy of all current rules.
func (s *SessionRuleStore) Rules() []SessionRule {
	out := make([]SessionRule, len(s.rules))
	copy(out, s.rules)
	return out
}

// ---------- Question-style preview (AskUser enhancements) ----------

// NumericKeyToIndex converts a numeric rune ('1'-'9') to a 0-based index.
// Returns -1 for invalid keys.
func NumericKeyToIndex(r rune) int {
	if r >= '1' && r <= '9' {
		return int(r - '1')
	}
	return -1
}

// FormatOptionLabels prepends numeric keys to option labels for quick selection.
// e.g., ["Yes", "No"] → ["1. Yes", "2. No"]
func FormatOptionLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		if i < 9 {
			out[i] = fmt.Sprintf("%d. %s", i+1, l)
		} else {
			out[i] = fmt.Sprintf("   %s", l)
		}
	}
	return out
}
