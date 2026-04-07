package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Typeahead provides ghost text predictions after the cursor.
// Predictions are based on command history and common patterns.
// Tab accepts the ghost text; any other input updates the prediction.
type Typeahead struct {
	ghost   string // the predicted suffix (dim text shown after cursor)
	history *History
}

// NewTypeahead creates a Typeahead backed by the given history.
func NewTypeahead(history *History) *Typeahead {
	return &Typeahead{
		history: history,
	}
}

// Update recalculates the ghost text based on the current input.
// Should be called on every input change.
func (ta *Typeahead) Update(input string) {
	ta.ghost = ""
	if input == "" {
		return
	}

	// Search history from newest to oldest for a prefix match
	entries := ta.history.Entries()
	lower := strings.ToLower(input)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if len(entry) > len(input) && strings.HasPrefix(strings.ToLower(entry), lower) {
			// Ghost text is the suffix the user hasn't typed yet
			ta.ghost = entry[len(input):]
			return
		}
	}

	// Check common patterns for slash commands
	commonPatterns := []string{
		"/help", "/clear", "/model", "/compact", "/cost",
		"/diff", "/history", "/quit", "/config",
	}
	for _, pattern := range commonPatterns {
		if len(pattern) > len(input) && strings.HasPrefix(pattern, lower) {
			ta.ghost = pattern[len(input):]
			return
		}
	}
}

// Ghost returns the current ghost text (empty if no prediction).
func (ta *Typeahead) Ghost() string {
	return ta.ghost
}

// Accept returns the full text (input + ghost) and clears the prediction.
// Returns empty string if there is no ghost text.
func (ta *Typeahead) Accept(currentInput string) (string, bool) {
	if ta.ghost == "" {
		return "", false
	}
	full := currentInput + ta.ghost
	ta.ghost = ""
	return full, true
}

// Clear removes the current ghost text.
func (ta *Typeahead) Clear() {
	ta.ghost = ""
}

// HasGhost returns true if ghost text is currently available.
func (ta *Typeahead) HasGhost() bool {
	return ta.ghost != ""
}

// RenderGhost returns the ghost text styled as dim/faint text.
func (ta *Typeahead) RenderGhost(theme Theme) string {
	if ta.ghost == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor)).
		Faint(true).
		Render(ta.ghost)
}
