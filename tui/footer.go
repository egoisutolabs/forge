package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FooterPill represents a navigable item in the footer status bar.
type FooterPill struct {
	Label  string // display label (e.g. "agent:research", "task:#3")
	Type   string // "agent", "task", "session"
	ID     string // unique identifier
	Active bool   // has recent activity (pulsing indicator)
}

// FooterNav manages the navigable footer pills state.
type FooterNav struct {
	Pills    []FooterPill
	Selected int  // -1 = not in footer mode
	active   bool // whether footer navigation mode is on
}

// NewFooterNav creates a FooterNav in the inactive state.
func NewFooterNav() *FooterNav {
	return &FooterNav{
		Selected: -1,
	}
}

// Active returns true if the footer is in navigation mode.
func (fn *FooterNav) Active() bool {
	return fn.active
}

// Enter activates footer navigation mode.
// Selects the first pill if available.
func (fn *FooterNav) Enter() {
	if len(fn.Pills) == 0 {
		return
	}
	fn.active = true
	fn.Selected = 0
}

// Exit deactivates footer navigation mode.
func (fn *FooterNav) Exit() {
	fn.active = false
	fn.Selected = -1
}

// Next moves selection to the next pill (wraps around).
func (fn *FooterNav) Next() {
	if len(fn.Pills) == 0 {
		return
	}
	fn.Selected = (fn.Selected + 1) % len(fn.Pills)
}

// Prev moves selection to the previous pill (wraps around).
func (fn *FooterNav) Prev() {
	if len(fn.Pills) == 0 {
		return
	}
	fn.Selected--
	if fn.Selected < 0 {
		fn.Selected = len(fn.Pills) - 1
	}
}

// SelectedPill returns the currently selected pill, or nil if none.
func (fn *FooterNav) SelectedPill() *FooterPill {
	if !fn.active || fn.Selected < 0 || fn.Selected >= len(fn.Pills) {
		return nil
	}
	return &fn.Pills[fn.Selected]
}

// BuildPills constructs the pill list from the current model state.
func (fn *FooterNav) BuildPills(backgroundAgts int, status StatusInfo) {
	fn.Pills = fn.Pills[:0]

	// Background agent pills
	for i := 0; i < backgroundAgts; i++ {
		fn.Pills = append(fn.Pills, FooterPill{
			Label:  fmt.Sprintf("agent:%d", i+1),
			Type:   "agent",
			ID:     fmt.Sprintf("agent-%d", i),
			Active: true,
		})
	}

	// Session pill (always present if we have a model)
	if status.Model != "" {
		fn.Pills = append(fn.Pills, FooterPill{
			Label:  abbreviateModel(status.Model),
			Type:   "session",
			ID:     "session",
			Active: status.Processing,
		})
	}

	// Clamp selection if pills changed
	if fn.active && fn.Selected >= len(fn.Pills) {
		if len(fn.Pills) > 0 {
			fn.Selected = len(fn.Pills) - 1
		} else {
			fn.Exit()
		}
	}
}

// HandleKey processes navigation keys when footer mode is active.
// Returns (handled, action) where action is the pill type+ID on Enter.
func (fn *FooterNav) HandleKey(msg tea.KeyMsg) (handled bool, action string) {
	if !fn.active {
		return false, ""
	}

	switch msg.String() {
	case "left":
		fn.Prev()
		return true, ""
	case "right":
		fn.Next()
		return true, ""
	case "enter":
		if pill := fn.SelectedPill(); pill != nil {
			return true, pill.Type + ":" + pill.ID
		}
		return true, ""
	case "esc":
		fn.Exit()
		return true, ""
	}

	// Any typing exits footer mode
	if msg.Type == tea.KeyRunes {
		fn.Exit()
		return false, "" // let the keystroke through to the input
	}

	return false, ""
}

// RenderFooterPills renders the footer pills into the status bar.
func RenderFooterPills(fn *FooterNav, width int, theme Theme) string {
	if len(fn.Pills) == 0 {
		return ""
	}

	var parts []string
	for i, pill := range fn.Pills {
		label := pill.Label
		if pill.Active {
			label = "● " + label
		}

		var style lipgloss.Style
		if fn.active && i == fn.Selected {
			// Selected pill: accent border
			style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(theme.Config.AccentColor)).
				Foreground(lipgloss.Color(theme.Config.AccentColor)).
				Padding(0, 1)
		} else {
			// Unselected: dim
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color(theme.Config.DimColor)).
				Padding(0, 1)
		}

		parts = append(parts, style.Render(label))
	}

	return strings.Join(parts, " ")
}
