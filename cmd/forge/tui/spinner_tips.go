package tui

import (
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tipPool is the default set of tips shown below the spinner during processing.
var tipPool = []string{
	"Try /compact to free context space",
	"Use @ to reference files by name",
	"Ctrl+O opens the file finder",
	"Ctrl+F searches across files",
	"Use /cost to check session spending",
	"Tab collapses tool results",
	"PgUp/PgDn to scroll conversation",
	"! at start enters bash mode",
}

// contextTips maps active tool names to relevant tips.
var contextTips = map[string]string{
	"Bash":      "Use ! prefix to run shell commands directly",
	"Read":      "Use @ to reference files by name",
	"Grep":      "Ctrl+F opens the global search dialog",
	"Glob":      "Ctrl+O opens the file finder",
	"Edit":      "Tab collapses tool results to save space",
	"Write":     "Tab collapses tool results to save space",
	"Agent":     "Background agents appear in the footer",
	"WebSearch": "Use /cost to check session spending",
}

const (
	// tipRotateInterval is how often tips rotate while still processing.
	tipRotateInterval = 8 * time.Second
)

// SpinnerTips manages tip selection and rotation during processing.
type SpinnerTips struct {
	currentTip     string    // currently displayed tip
	pickedThisTurn bool      // one-shot guard: only pick once per processing turn
	lastRotation   time.Time // when the tip was last rotated
	tipIndex       int       // index into shuffled pool for round-robin
	shuffled       []string  // shuffled copy of tipPool
}

// NewSpinnerTips creates a new tip manager.
func NewSpinnerTips() *SpinnerTips {
	return &SpinnerTips{}
}

// PickTip selects a tip for the current processing turn. If active tools
// are provided, prefer a context-relevant tip. One-shot: subsequent calls
// in the same turn are no-ops.
func (st *SpinnerTips) PickTip(activeTools []ActiveToolInfo) {
	if st.pickedThisTurn {
		return
	}
	st.pickedThisTurn = true
	st.lastRotation = time.Now()

	// Try context-aware tip first
	for _, tool := range activeTools {
		if tip, ok := contextTips[tool.Name]; ok {
			st.currentTip = tip
			return
		}
	}

	// Fall back to pool rotation
	st.currentTip = st.nextFromPool()
}

// RotateIfDue checks if enough time has passed and picks a new tip.
func (st *SpinnerTips) RotateIfDue(now time.Time, activeTools []ActiveToolInfo) bool {
	if st.lastRotation.IsZero() || now.Sub(st.lastRotation) < tipRotateInterval {
		return false
	}
	st.lastRotation = now

	// Check context tips first
	for _, tool := range activeTools {
		if tip, ok := contextTips[tool.Name]; ok {
			if tip != st.currentTip {
				st.currentTip = tip
				return true
			}
		}
	}

	st.currentTip = st.nextFromPool()
	return true
}

// Reset clears the one-shot guard for a new processing turn.
func (st *SpinnerTips) Reset() {
	st.pickedThisTurn = false
	st.currentTip = ""
	st.lastRotation = time.Time{}
}

// Current returns the currently selected tip, or empty string if none.
func (st *SpinnerTips) Current() string {
	return st.currentTip
}

// Picked returns whether a tip has been picked this turn.
func (st *SpinnerTips) Picked() bool {
	return st.pickedThisTurn
}

// nextFromPool returns the next tip from a shuffled pool, reshuffling when exhausted.
func (st *SpinnerTips) nextFromPool() string {
	if len(st.shuffled) == 0 || st.tipIndex >= len(st.shuffled) {
		st.shuffled = make([]string, len(tipPool))
		copy(st.shuffled, tipPool)
		rand.Shuffle(len(st.shuffled), func(i, j int) {
			st.shuffled[i], st.shuffled[j] = st.shuffled[j], st.shuffled[i]
		})
		st.tipIndex = 0
	}
	tip := st.shuffled[st.tipIndex]
	st.tipIndex++
	return tip
}

// TipRotateMsg triggers a tip rotation check during processing.
type TipRotateMsg time.Time

// tipRotateTick returns a command that fires a TipRotateMsg at the rotation interval.
func tipRotateTick() tea.Cmd {
	return tea.Tick(tipRotateInterval, func(t time.Time) tea.Msg {
		return TipRotateMsg(t)
	})
}

// renderSpinnerTip renders the current tip in faint style below the spinner area.
func renderSpinnerTip(tip string, width int, theme Theme) string {
	if tip == "" {
		return ""
	}
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.DimColor)).
		Faint(true)
	rendered := style.Render("  💡 " + tip)
	if lipgloss.Width(rendered) > width-2 {
		rendered = rendered[:width-2]
	}
	return rendered + "\n"
}
