package tui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

// FocusState represents the terminal's focus state.
type FocusState int

const (
	FocusUnknown FocusState = iota
	FocusFocused
	FocusBlurred
)

// String returns a human-readable focus state.
func (f FocusState) String() string {
	switch f {
	case FocusFocused:
		return "focused"
	case FocusBlurred:
		return "blurred"
	default:
		return "unknown"
	}
}

// TerminalFocus tracks terminal focus/blur state using ANSI focus reporting.
// Enable by writing enableFocusReporting to stdout; the terminal will then
// send "\033[I" (focus in) and "\033[O" (focus out) escape sequences, which
// Bubbletea surfaces as tea.FocusMsg and tea.BlurMsg.
type TerminalFocus struct {
	state FocusState
	clock *AnimClock
}

// ANSI escape sequences for focus reporting (DECSET 1004).
const (
	enableFocusReporting  = "\033[?1004h"
	disableFocusReporting = "\033[?1004l"
)

// NewTerminalFocus creates a new TerminalFocus tracker with an optional
// AnimClock that will be slowed when the terminal loses focus.
func NewTerminalFocus(clock *AnimClock) *TerminalFocus {
	return &TerminalFocus{
		state: FocusUnknown,
		clock: clock,
	}
}

// EnableReporting returns a tea.Cmd that sends the ANSI escape to enable
// focus reporting. Call this from Init().
func EnableFocusReporting(w io.Writer) tea.Cmd {
	return func() tea.Msg {
		_, _ = w.Write([]byte(enableFocusReporting))
		return nil
	}
}

// DisableFocusReporting writes the ANSI escape to disable focus reporting.
// Call this on shutdown / cleanup.
func DisableFocusReporting(w io.Writer) {
	_, _ = w.Write([]byte(disableFocusReporting))
}

// HandleFocus processes a tea.FocusMsg, updating state and clock.
func (tf *TerminalFocus) HandleFocus() {
	tf.state = FocusFocused
	if tf.clock != nil {
		tf.clock.SetBlurred(false)
	}
}

// HandleBlur processes a tea.BlurMsg, updating state and clock.
func (tf *TerminalFocus) HandleBlur() {
	tf.state = FocusBlurred
	if tf.clock != nil {
		tf.clock.SetBlurred(true)
	}
}

// State returns the current focus state.
func (tf *TerminalFocus) State() FocusState {
	return tf.state
}

// IsFocused returns true if the terminal is currently focused.
func (tf *TerminalFocus) IsFocused() bool {
	return tf.state == FocusFocused
}

// IsBlurred returns true if the terminal is currently blurred.
func (tf *TerminalFocus) IsBlurred() bool {
	return tf.state == FocusBlurred
}
