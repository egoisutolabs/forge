package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Default cost thresholds at which the cost dialog is shown.
var DefaultCostThresholds = []float64{1.0, 5.0, 10.0}

// CostDialogChoice represents the user's response to the cost threshold dialog.
type CostDialogChoice int

const (
	CostContinue  CostDialogChoice = iota // keep going
	CostSetBudget                         // prompt for a budget
	CostStop                              // stop the session
)

// CostThresholdState tracks which thresholds have been shown.
type CostThresholdState struct {
	Thresholds      []float64
	ShownThresholds map[float64]bool
}

// NewCostThresholdState creates a CostThresholdState with default thresholds.
func NewCostThresholdState() *CostThresholdState {
	return &CostThresholdState{
		Thresholds:      DefaultCostThresholds,
		ShownThresholds: make(map[float64]bool),
	}
}

// CheckThreshold returns the threshold that was just crossed, or 0 if none.
// A threshold is only returned once (tracked by ShownThresholds).
func (cs *CostThresholdState) CheckThreshold(costUSD float64) float64 {
	for _, t := range cs.Thresholds {
		if costUSD >= t && !cs.ShownThresholds[t] {
			cs.ShownThresholds[t] = true
			return t
		}
	}
	return 0
}

// CostDialog wraps a huh form for the cost threshold confirmation.
type CostDialog struct {
	form      *huh.Form
	choice    string // "continue", "budget", "stop"
	costUSD   float64
	threshold float64
}

// NewCostDialog creates a cost threshold dialog.
func NewCostDialog(costUSD, threshold float64, theme Theme) *CostDialog {
	cd := &CostDialog{
		costUSD:   costUSD,
		threshold: threshold,
		choice:    "continue",
	}

	title := fmt.Sprintf("Session cost: $%.2f", costUSD)
	desc := fmt.Sprintf("Cost has exceeded $%.0f threshold.", threshold)

	sel := huh.NewSelect[string]().
		Key("cost-choice").
		Title(title).
		Description(desc).
		Options(
			huh.NewOption("Continue", "continue"),
			huh.NewOption("Set budget limit", "budget"),
			huh.NewOption("Stop", "stop"),
		).
		Value(&cd.choice)

	cd.form = huh.NewForm(
		huh.NewGroup(sel),
	).WithTheme(costDialogTheme(theme))

	return cd
}

// CostDialogResult is the outcome of the cost threshold dialog.
type CostDialogResult struct {
	Choice    CostDialogChoice
	Threshold float64
}

// CostDialogResultMsg is sent when the cost dialog completes.
type CostDialogResultMsg struct {
	Result CostDialogResult
}

// updateCostDialog forwards messages to the cost dialog form.
func (m AppModel) updateCostDialog(msg tea.Msg, cd *CostDialog) (AppModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" {
		// Esc = continue (dismiss)
		cmd := m.dialogQueue.Close()
		return m, cmd
	}

	form, cmd := cd.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		cd.form = f
	}

	if cd.form.State == huh.StateCompleted {
		var choice CostDialogChoice
		switch cd.choice {
		case "budget":
			choice = CostSetBudget
		case "stop":
			choice = CostStop
		default:
			choice = CostContinue
		}

		closeCmd := m.dialogQueue.Close()

		// Handle the choice
		switch choice {
		case CostStop:
			m.addSystemMessage("  Session stopped by cost limit.")
			return m, tea.Batch(closeCmd, tea.Quit)
		case CostSetBudget:
			m.addSystemMessage(fmt.Sprintf("  Budget set to $%.0f.", cd.threshold*2))
			// Set budget to 2x the threshold that triggered
			budget := cd.threshold * 2
			m.bridge.eng.SetMaxBudget(budget)
			return m, closeCmd
		default:
			return m, closeCmd
		}
	} else if cd.form.State == huh.StateAborted {
		closeCmd := m.dialogQueue.Close()
		return m, closeCmd
	}

	return m, cmd
}

// costDialogTheme creates a huh theme for the cost dialog.
func costDialogTheme(theme Theme) *huh.Theme {
	t := huh.ThemeCharm()

	t.Focused.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Config.WarningColor))

	t.Focused.Description = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	t.Focused.Base = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Config.WarningColor)).
		Padding(1, 2)

	t.Focused.SelectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.AccentColor)).
		Bold(true)

	t.Focused.UnselectedOption = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor))

	return t
}
