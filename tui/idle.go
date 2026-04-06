package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

const (
	// idleThreshold is the duration of inactivity before showing the idle dialog.
	idleThreshold = 30 * time.Minute
)

// IdleChoice represents the user's response to the idle dialog.
type IdleChoice int

const (
	IdleContinue IdleChoice = iota // continue the current conversation
	IdleClear                      // clear conversation and start fresh
	IdleNeverAsk                   // don't ask again (persist to config)
)

// IdleState tracks user activity for idle detection.
type IdleState struct {
	lastActivityTime time.Time     // updated on each keypress/submit
	neverAsk         bool          // persisted preference to suppress dialog
	dialogShowing    bool          // whether the idle dialog is currently displayed
	form             *huh.Form     // the huh form when dialog is active
	idleDuration     time.Duration // how long user was idle (for display)
}

// NewIdleState creates a new idle tracker.
func NewIdleState() *IdleState {
	return &IdleState{
		lastActivityTime: time.Now(),
		neverAsk:         loadIdleNeverAsk(),
	}
}

// RecordActivity updates the last activity time.
func (s *IdleState) RecordActivity() {
	s.lastActivityTime = time.Now()
}

// CheckIdle returns true if the user has been idle past the threshold.
// Also returns the idle duration for display.
func (s *IdleState) CheckIdle(now time.Time) (bool, time.Duration) {
	if s.neverAsk || s.dialogShowing {
		return false, 0
	}
	if s.lastActivityTime.IsZero() {
		return false, 0
	}
	elapsed := now.Sub(s.lastActivityTime)
	if elapsed >= idleThreshold {
		return true, elapsed
	}
	return false, 0
}

// ShowDialog creates and returns the idle return dialog.
func (s *IdleState) ShowDialog(idleDuration time.Duration, theme Theme) *huh.Form {
	s.dialogShowing = true
	s.idleDuration = idleDuration

	durationStr := formatIdleDuration(idleDuration)

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("You've been away for %s. Continue or start fresh?", durationStr)).
				Options(
					huh.NewOption("Continue", "continue"),
					huh.NewOption("Clear conversation", "clear"),
					huh.NewOption("Don't ask again", "never"),
				).
				Value(&choice),
		),
	).WithTheme(huh.ThemeCatppuccin())

	s.form = form
	return form
}

// HandleChoice processes the user's idle dialog selection.
// Returns the choice and clears the dialog state.
func (s *IdleState) HandleChoice() IdleChoice {
	s.dialogShowing = false
	if s.form == nil {
		return IdleContinue
	}

	// Extract the choice value from the form
	choice := s.form.GetString("0")
	s.form = nil

	switch choice {
	case "clear":
		return IdleClear
	case "never":
		s.neverAsk = true
		saveIdleNeverAsk(true)
		return IdleNeverAsk
	default:
		return IdleContinue
	}
}

// DismissDialog cancels the idle dialog (e.g. on Escape).
func (s *IdleState) DismissDialog() {
	s.dialogShowing = false
	s.form = nil
	s.RecordActivity()
}

// IsDialogShowing returns whether the idle dialog is currently displayed.
func (s *IdleState) IsDialogShowing() bool {
	return s.dialogShowing
}

// NeverAsk returns the persisted preference.
func (s *IdleState) NeverAsk() bool {
	return s.neverAsk
}

// SetNeverAsk sets the persisted preference.
func (s *IdleState) SetNeverAsk(v bool) {
	s.neverAsk = v
	saveIdleNeverAsk(v)
}

// IdleDuration returns how long the user was idle (for display in dialog).
func (s *IdleState) IdleDuration() time.Duration {
	return s.idleDuration
}

// formatIdleDuration returns a human-friendly duration string.
func formatIdleDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// renderIdleDialog renders the idle return dialog.
func renderIdleDialog(s *IdleState, width int, theme Theme) string {
	if !s.dialogShowing || s.form == nil {
		return ""
	}

	durationStr := formatIdleDuration(s.idleDuration)
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.WarningColor)).
		Bold(true).
		Render(fmt.Sprintf("  ⏰ Away for %s", durationStr))

	formView := s.form.View()

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Config.WarningColor)).
		Padding(0, 1).
		Width(width - 8).
		Render(header + "\n" + formView)

	return "\n" + box + "\n"
}

// IdleCheckMsg triggers an idle check on user input after inactivity.
type IdleCheckMsg struct {
	Duration time.Duration
}

// idleConfigPath returns the path to the idle config file.
func idleConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".forge", "idle.conf")
}

// loadIdleNeverAsk reads the "never ask" preference from config.
func loadIdleNeverAsk() bool {
	path := idleConfigPath()
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return string(data) == "never_ask=true\n"
}

// saveIdleNeverAsk persists the "never ask" preference to config.
func saveIdleNeverAsk(neverAsk bool) {
	path := idleConfigPath()
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0o755)
	content := "never_ask=false\n"
	if neverAsk {
		content = "never_ask=true\n"
	}
	_ = os.WriteFile(path, []byte(content), 0o600)
}

// idleTick returns a command that fires an idle check every minute.
func idleTick() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return IdleCheckMsg{}
	})
}
