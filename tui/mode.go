package tui

import "github.com/charmbracelet/lipgloss"

// InputMode represents the current mode of the input field.
type InputMode int

const (
	ModeNormal     InputMode = iota // default input
	ModeBash                        // ! prefix — direct bash execution
	ModeProcessing                  // engine is processing
	ModePlan                        // plan mode
)

// String returns a human-readable label for the input mode.
func (m InputMode) String() string {
	switch m {
	case ModeBash:
		return "bash"
	case ModeProcessing:
		return "..."
	case ModePlan:
		return "plan"
	default:
		return ""
	}
}

// modeBorderColor returns the border color for the given mode.
func modeBorderColor(mode InputMode, theme Theme) lipgloss.Color {
	switch mode {
	case ModeBash:
		return lipgloss.Color(theme.Config.WarningColor) // orange/yellow
	case ModeProcessing:
		return lipgloss.Color(theme.Config.BorderColor) // dim
	case ModePlan:
		return lipgloss.Color(theme.Config.AccentColor) // blue
	default:
		return lipgloss.Color(theme.Config.AccentColor) // focused accent
	}
}

// modeLabel returns the label text (if any) to display in the input border.
func modeLabel(mode InputMode) string {
	return mode.String()
}

// renderInputWithMode renders the textarea with mode-aware border color and label.
func renderInputWithMode(inputView string, mode InputMode, width int, theme Theme) string {
	borderColor := modeBorderColor(mode, theme)
	label := modeLabel(mode)

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(width - 4)

	rendered := style.Render(inputView)

	// Insert mode label into the top border if present
	if label != "" {
		labelStyled := lipgloss.NewStyle().
			Foreground(borderColor).
			Bold(true).
			Render(" " + label + " ")
		rendered = insertBorderLabel(rendered, labelStyled)
	}

	return rendered
}

// insertBorderLabel inserts a label into the top-left of a rounded border.
// It replaces part of the first line after the opening corner.
func insertBorderLabel(rendered, label string) string {
	if len(rendered) < 4 {
		return rendered
	}
	// The first line is the top border: ╭──────...──╮
	// Insert label after the first 2 characters (╭─)
	lines := splitLines(rendered)
	if len(lines) == 0 {
		return rendered
	}

	top := lines[0]
	runes := []rune(top)
	labelRunes := []rune(label)
	labelWidth := lipgloss.Width(label)

	if len(runes) > labelWidth+3 {
		// Replace chars 2..2+labelWidth with the label
		result := make([]rune, 0, len(runes))
		result = append(result, runes[:2]...)
		result = append(result, labelRunes...)
		result = append(result, runes[2+labelWidth:]...)
		lines[0] = string(result)
	}

	return joinLines(lines)
}

// splitLines splits a string into lines without using strings to avoid import.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// joinLines joins lines with newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for _, l := range lines[1:] {
		result += "\n" + l
	}
	return result
}
