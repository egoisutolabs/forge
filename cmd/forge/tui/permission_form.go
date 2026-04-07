package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// PermissionForm wraps a huh.Form for interactive permission approval.
// It embeds into the AppModel Update/View cycle and sends the result
// back via the PermissionRequestMsg.ResponseCh when completed.
type PermissionForm struct {
	form     *huh.Form
	approved bool
	perm     *PermissionRequestMsg

	// Destructive warning (nil if command is not flagged as destructive)
	destructiveWarning *DestructiveWarning

	// Editable rule prefix for "Allow all matching: <pattern>" option
	rulePrefix string
}

// NewPermissionForm creates a huh-based permission confirmation dialog.
func NewPermissionForm(perm *PermissionRequestMsg, theme Theme) *PermissionForm {
	pf := &PermissionForm{
		perm: perm,
	}

	// Build description with action + detail
	desc := perm.Action
	if perm.Detail != "" && perm.Detail != perm.Message {
		desc += "\n" + perm.Detail
	}

	// Check for destructive commands and attach warning
	if perm.ToolName == "Bash" {
		if isDestructive, reason := IsDestructiveCommand(perm.Detail); isDestructive {
			pf.destructiveWarning = &DestructiveWarning{
				Command: perm.Detail,
				Reason:  reason,
			}
		}
		// Always generate a rule prefix for Bash commands
		pf.rulePrefix = ExtractRulePrefix(perm.Detail)
	}

	// Risk indicator prefix
	riskLabel := riskIndicator(perm.Risk)

	title := fmt.Sprintf("%s  %s", perm.ToolName, riskLabel)

	confirm := huh.NewConfirm().
		Key("approve").
		Title(title).
		Description(desc).
		Affirmative("Yes").
		Negative("No").
		Value(&pf.approved)

	pf.form = huh.NewForm(
		huh.NewGroup(confirm),
	).WithTheme(permissionTheme(perm.Risk, theme))

	return pf
}

// updatePermForm forwards messages to the huh permission form and handles completion.
func (m AppModel) updatePermForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Allow Escape to deny immediately
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.permForm.perm.ResponseCh <- false
			m.permForm = nil
			return m, nil
		case "?":
			// Toggle destructive warning detail
			if m.permForm.destructiveWarning != nil {
				m.permForm.destructiveWarning.ShowDetail = !m.permForm.destructiveWarning.ShowDetail
				return m, nil
			}
		case "a":
			// Add session rule for matching commands and auto-approve
			if m.permForm.rulePrefix != "" {
				m.sessionRules.Add(m.permForm.perm.ToolName, m.permForm.rulePrefix)
				m.permForm.perm.ResponseCh <- true
				m.permForm = nil
				return m, nil
			}
		}
	}

	form, cmd := m.permForm.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.permForm.form = f
	}

	if m.permForm.form.State == huh.StateCompleted {
		m.permForm.perm.ResponseCh <- m.permForm.approved
		m.permForm = nil
		m.suppressInput()
	} else if m.permForm.form.State == huh.StateAborted {
		m.permForm.perm.ResponseCh <- false
		m.permForm = nil
		m.suppressInput()
	}

	return m, cmd
}

// riskIndicator returns a colored risk bar string.
func riskIndicator(risk RiskLevel) string {
	switch risk {
	case RiskModerate:
		return "Risk: moderate"
	case RiskHigh:
		return "Risk: HIGH"
	default:
		return "Risk: low"
	}
}

// permissionTheme creates a huh theme that matches our TUI color scheme
// and adjusts border color based on risk level.
func permissionTheme(risk RiskLevel, theme Theme) *huh.Theme {
	t := huh.ThemeCharm()

	borderColor := theme.Config.WarningColor // amber default
	switch risk {
	case RiskHigh:
		borderColor = theme.Config.ErrorColor // red for high risk
	case RiskLow:
		borderColor = theme.Config.SuccessColor // green for low risk
	}

	t.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15"))

	t.Focused.Description = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	t.Focused.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		Padding(1, 2)

	t.Focused.SelectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AccentColor)).
		Bold(true)

	t.Focused.UnselectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	return t
}
