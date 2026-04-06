package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TerminalTitle manages the terminal window/tab title via ANSI OSC escapes.
// Uses OSC 0: \033]0;TITLE\007 which sets both window and icon title.

// TitleState tracks what terminal title is currently set to avoid
// redundant writes.
type TitleState struct {
	current string
	cwd     string // working directory, used for idle title
}

// NewTitleState creates a new TitleState with the given working directory.
func NewTitleState(cwd string) *TitleState {
	return &TitleState{cwd: cwd}
}

// SetProcessing sets the terminal title to indicate active processing.
func (ts *TitleState) SetProcessing() string {
	return ts.set("⚒ Forge - processing...")
}

// SetIdle sets the terminal title to show the working directory.
func (ts *TitleState) SetIdle() string {
	dir := shortenCwd(ts.cwd)
	return ts.set(fmt.Sprintf("⚒ Forge - %s", dir))
}

// SetCustom sets a custom terminal title.
func (ts *TitleState) SetCustom(title string) string {
	return ts.set(title)
}

// Current returns the current title string.
func (ts *TitleState) Current() string {
	return ts.current
}

// set updates the title if it changed, returning the ANSI escape sequence.
// Returns empty string if the title hasn't changed.
func (ts *TitleState) set(title string) string {
	if ts.current == title {
		return ""
	}
	ts.current = title
	return formatTitleSequence(title)
}

// formatTitleSequence returns the ANSI OSC escape to set terminal title.
func formatTitleSequence(title string) string {
	return fmt.Sprintf("\033]0;%s\007", title)
}

// WriteTitleSequence writes the ANSI escape sequence directly to stderr
// (NOT stdout) to avoid interfering with Bubbletea's output management.
// Writing ANSI to stdout can cause terminal responses (OSC replies, CPR)
// to leak into stdin and appear as gibberish in the textarea.
func WriteTitleSequence(title string) {
	if title == "" {
		return
	}
	seq := formatTitleSequence(title)
	_, _ = os.Stderr.WriteString(seq)
}

// ResetTitle clears the terminal title (restores terminal default).
func ResetTitle() {
	_, _ = os.Stderr.WriteString("\033]0;\007")
}

// shortenCwd converts an absolute path to a user-friendly display form.
// Replaces home directory with ~ and shows only the last 2 components
// if the path is long.
func shortenCwd(cwd string) string {
	if cwd == "" {
		return "~"
	}

	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}

	// If path has more than 3 components, show ~/../last/component
	parts := strings.Split(cwd, string(filepath.Separator))
	if len(parts) > 3 {
		return filepath.Join(parts[0], "..", parts[len(parts)-2], parts[len(parts)-1])
	}

	return cwd
}
