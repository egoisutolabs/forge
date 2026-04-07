package tui

// Selection management: clears text selection on most keystrokes,
// preserves on navigation keys (shift/cmd+arrows, pgup/pgdown).
//
// In a terminal-based TUI, "text selection" refers to any OS-level
// terminal text selection. We track whether the user might have an
// active selection and clear it when they type. The actual clearing
// happens implicitly — we just track the intent so callers can decide
// whether to reset viewport state that might conflict with selection.

// SelectionState tracks whether the user might have an active text selection.
type SelectionState struct {
	active bool
}

// NewSelectionState creates a new SelectionState.
func NewSelectionState() *SelectionState {
	return &SelectionState{}
}

// Active returns true if a selection might be active.
func (s *SelectionState) Active() bool {
	return s.active
}

// SetActive marks that a selection may be active (e.g. after shift+arrow).
func (s *SelectionState) SetActive(active bool) {
	s.active = active
}

// Clear marks the selection as inactive.
func (s *SelectionState) Clear() {
	s.active = false
}

// shouldClearSelection returns true if the given key should clear any
// active text selection. Navigation/modifier keys preserve selection;
// most other keys (typing, enter, backspace, etc.) clear it.
func shouldClearSelection(key string) bool {
	switch key {
	// Shift+arrow combinations preserve selection
	case "shift+up", "shift+down", "shift+left", "shift+right":
		return false
	// Cmd/ctrl+shift arrows preserve selection
	case "ctrl+shift+left", "ctrl+shift+right":
		return false
	// Page navigation preserves selection
	case "pgup", "pgdown":
		return false
	// Plain arrow keys don't clear (they move cursor, not destructive)
	case "up", "down", "left", "right":
		return false
	// Modifier-only keys preserve
	case "shift+tab":
		return false
	// Everything else clears selection
	default:
		return true
	}
}
