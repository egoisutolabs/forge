package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// stallWarningThreshold is when the spinner turns yellow.
	stallWarningThreshold = 3 * time.Second
	// stallCriticalThreshold is when the spinner turns red.
	stallCriticalThreshold = 6 * time.Second
)

// StallState tracks token timing for stall detection during processing.
type StallState struct {
	lastTokenTime  time.Time // updated on each StreamTextMsg
	hasActiveTools bool      // true when tools are running (suppresses stall)
	stalled        bool      // true when no tokens for > stallWarningThreshold
}

// NewStallState creates a fresh stall tracker.
func NewStallState() *StallState {
	return &StallState{}
}

// OnStreamText records that a token arrived, resetting the stall timer.
func (s *StallState) OnStreamText() {
	s.lastTokenTime = time.Now()
	s.stalled = false
}

// OnToolStart marks that tools are active (suppresses stall display).
func (s *StallState) OnToolStart() {
	s.hasActiveTools = true
}

// OnToolDone checks if all tools are done based on a count.
func (s *StallState) OnToolDone(activeToolCount int) {
	s.hasActiveTools = activeToolCount > 0
}

// OnProcessingStart resets the stall state for a new processing cycle.
func (s *StallState) OnProcessingStart() {
	s.lastTokenTime = time.Now()
	s.stalled = false
	s.hasActiveTools = false
}

// OnProcessingDone clears stall state when processing ends.
func (s *StallState) OnProcessingDone() {
	s.stalled = false
	s.hasActiveTools = false
}

// StallLevel describes the severity of a stall.
type StallLevel int

const (
	StallNone     StallLevel = iota // normal operation
	StallWarning                    // 3s+ no tokens — yellow
	StallCritical                   // 6s+ no tokens — red
)

// Check evaluates the current stall state. Should be called on tick.
func (s *StallState) Check(now time.Time) StallLevel {
	if s.lastTokenTime.IsZero() || s.hasActiveTools {
		s.stalled = false
		return StallNone
	}
	elapsed := now.Sub(s.lastTokenTime)
	if elapsed >= stallCriticalThreshold {
		s.stalled = true
		return StallCritical
	}
	if elapsed >= stallWarningThreshold {
		s.stalled = true
		return StallWarning
	}
	s.stalled = false
	return StallNone
}

// Stalled returns whether the system is currently in a stalled state.
func (s *StallState) Stalled() bool {
	return s.stalled
}

// StallTickMsg triggers a stall check during processing.
type StallTickMsg time.Time

// stallTick returns a command that fires a StallTickMsg every 500ms.
func stallTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return StallTickMsg(t)
	})
}

// stallSpinnerStyle returns the appropriate spinner style for the current stall level.
func stallSpinnerStyle(level StallLevel, theme Theme) lipgloss.Style {
	switch level {
	case StallWarning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Config.WarningColor))
	case StallCritical:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Config.ErrorColor))
	default:
		return theme.SpinnerStyle
	}
}
