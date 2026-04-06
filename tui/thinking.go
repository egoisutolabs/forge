package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// itoa is a simple int-to-string for small counts.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// thinkingHeaderStyle is the dim style for the thinking section header.
var thinkingHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("8")).
	Faint(true)

// thinkingContentStyle is the dim style for the thinking content body.
var thinkingContentStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("8")).
	Faint(true)

// renderThinkingBlock renders a collapsible thinking section.
// When collapsed, shows "💭 Thinking... (Tab to expand)".
// When expanded, shows the full thinking content in dim text.
func renderThinkingBlock(thinking string, expanded bool, width int) string {
	if thinking == "" {
		return ""
	}

	if !expanded {
		return "  " + thinkingHeaderStyle.Render("💭 Thinking… (Tab to expand)")
	}

	var sb strings.Builder
	sb.WriteString("  " + thinkingHeaderStyle.Render("💭 Thinking…"))
	sb.WriteByte('\n')

	// Render thinking content with left border, similar to tool results
	lines := strings.Split(thinking, "\n")
	maxLines := 30
	for i, line := range lines {
		if i >= maxLines {
			remaining := len(lines) - maxLines
			sb.WriteString(thinkingContentStyle.Render("  │ "))
			sb.WriteString(thinkingContentStyle.Render(
				"... " + itoa(remaining) + " more lines"))
			sb.WriteByte('\n')
			break
		}
		// Truncate long lines
		if len(line) > width-8 && width > 12 {
			line = line[:width-11] + "..."
		}
		sb.WriteString(thinkingContentStyle.Render("  │ " + line))
		sb.WriteByte('\n')
	}

	return strings.TrimRight(sb.String(), "\n")
}
