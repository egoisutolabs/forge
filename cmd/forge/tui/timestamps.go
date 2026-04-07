package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// formatRelativeTime formats a time as a human-readable relative duration
// like "just now", "2m ago", "1h ago", etc.
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatRelativeTimeSince(t, time.Now())
}

// formatRelativeTimeSince formats a time relative to a given reference time.
// Extracted for testability.
func formatRelativeTimeSince(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}

	switch {
	case d < 30*time.Second:
		return "just now"
	case d < 90*time.Second:
		return "1m ago"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "1h ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "1d ago"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// renderTimestamp renders a dim relative timestamp string.
func renderTimestamp(t time.Time) string {
	rel := formatRelativeTime(t)
	if rel == "" {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Faint(true).
		Render(rel)
}
