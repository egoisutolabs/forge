package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// TasksPanel is an expandable panel showing background agent details.
// It uses the BackgroundState from background.go for agent tracking.
type TasksPanel struct {
	Expanded bool
	Selected int // index of selected agent row in the rendered list
}

// NewTasksPanel creates a new collapsed tasks panel.
func NewTasksPanel() *TasksPanel {
	return &TasksPanel{
		Expanded: false,
		Selected: 0,
	}
}

// Toggle expands or collapses the panel.
func (p *TasksPanel) Toggle() {
	p.Expanded = !p.Expanded
}

// Collapse hides the panel.
func (p *TasksPanel) Collapse() {
	p.Expanded = false
}

// NextAgent moves selection down.
func (p *TasksPanel) NextAgent(count int) {
	if count == 0 {
		return
	}
	p.Selected = (p.Selected + 1) % count
}

// PrevAgent moves selection up.
func (p *TasksPanel) PrevAgent(count int) {
	if count == 0 {
		return
	}
	p.Selected--
	if p.Selected < 0 {
		p.Selected = count - 1
	}
}

// ClampSelected ensures the selected index is valid for the given agent count.
func (p *TasksPanel) ClampSelected(count int) {
	if count == 0 {
		p.Selected = 0
		return
	}
	if p.Selected >= count {
		p.Selected = count - 1
	}
}

// Render renders the expanded tasks panel using agents from BackgroundState.
func (p *TasksPanel) Render(agents []*BackgroundAgent, width int) string {
	if !p.Expanded || len(agents) == 0 {
		return ""
	}

	narrow := width < 60

	var sb strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	sb.WriteString("  " + headerStyle.Render("Background Agents") + "\n")

	for i, agent := range agents {
		prefix := "  "
		if i == p.Selected {
			prefix = "> "
		}

		statusIcon := "⠋"
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
		if agent.Completed {
			statusIcon = "✓"
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		}

		left := statusStyle.Render(prefix+statusIcon) + " " + agent.Name

		if narrow {
			sb.WriteString(left + "\n")
			continue
		}

		// Right side: elapsed time
		var rightParts []string
		elapsed := time.Since(agent.StartTime)
		if agent.Completed && !agent.CompletedAt.IsZero() {
			elapsed = agent.CompletedAt.Sub(agent.StartTime)
		}
		rightParts = append(rightParts, formatDuration(elapsed))
		if agent.OutputLen > 0 {
			rightParts = append(rightParts, fmt.Sprintf("%d chars", agent.OutputLen))
		}

		rightStr := lipgloss.NewStyle().Faint(true).Render(strings.Join(rightParts, " · "))
		leftWidth := lipgloss.Width(left)
		rightWidth := lipgloss.Width(rightStr)
		gap := width - leftWidth - rightWidth - 4
		if gap < 1 {
			gap = 1
		}
		sb.WriteString(left + strings.Repeat(" ", gap) + rightStr + "\n")
	}

	// Hint for selected agent
	if p.Selected >= 0 && p.Selected < len(agents) && !agents[p.Selected].Completed {
		hint := lipgloss.NewStyle().Faint(true).Render("  x:stop  esc:close")
		sb.WriteString(hint + "\n")
	} else {
		hint := lipgloss.NewStyle().Faint(true).Render("  esc:close")
		sb.WriteString(hint + "\n")
	}

	content := strings.TrimRight(sb.String(), "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1).
		Width(width - 4).
		Render(content)

	return box + "\n"
}
