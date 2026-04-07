package tui

import (
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
)

// AnimationTickMsg is sent on each animation frame (~60fps).
type AnimationTickMsg time.Time

// SpringState tracks a single spring-animated float value.
type SpringState struct {
	Pos    float64
	Vel    float64
	Target float64
	Active bool
}

// Settled returns true when the spring is close enough to its target.
func (s *SpringState) Settled() bool {
	return math.Abs(s.Pos-s.Target) < 0.5 && math.Abs(s.Vel) < 0.5
}

// Update advances the spring one step using the given spring config.
// Returns true if still animating.
func (s *SpringState) Update(spring harmonica.Spring) bool {
	if !s.Active {
		return false
	}
	s.Pos, s.Vel = spring.Update(s.Pos, s.Vel, s.Target)
	if s.Settled() {
		s.Pos = s.Target
		s.Vel = 0
		s.Active = false
		return false
	}
	return true
}

// Start begins animating from current position toward target.
func (s *SpringState) Start(target float64) {
	s.Target = target
	s.Active = true
}

// StartFrom begins animating from a specific position toward target.
func (s *SpringState) StartFrom(from, target float64) {
	s.Pos = from
	s.Vel = 0
	s.Target = target
	s.Active = true
}

// Snap immediately sets the spring to the target with no animation.
func (s *SpringState) Snap(target float64) {
	s.Pos = target
	s.Vel = 0
	s.Target = target
	s.Active = false
}

// CollapseAnimation tracks a tool result expand/collapse height transition.
type CollapseAnimation struct {
	MsgIndex   int     // index in messages slice being animated
	Collapsing bool    // true = expanded → collapsed
	Height     float64 // current animated height in lines (set from spring)
}

// animationTick returns a command that fires an AnimationTickMsg at ~60fps.
func animationTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return AnimationTickMsg(t)
	})
}

// newScrollSpring creates a gentle, critically-damped spring for viewport scrolling.
func newScrollSpring() harmonica.Spring {
	return harmonica.NewSpring(harmonica.FPS(60), 3.0, 1.0)
}

// newUISpring creates a snappier spring for UI element animations.
func newUISpring() harmonica.Spring {
	return harmonica.NewSpring(harmonica.FPS(60), 5.0, 1.0)
}

// clipToLines clips a rendered string to at most n visible lines.
// Uses index scanning instead of Split+Join to avoid allocations.
func clipToLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	idx := 0
	for i := 0; i < n; i++ {
		next := strings.IndexByte(s[idx:], '\n')
		if next < 0 {
			return s // fewer than n lines total
		}
		idx += next + 1
	}
	// idx points past the n-th newline; trim it
	if idx > 0 {
		return s[:idx-1]
	}
	return s
}

// countLines returns the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
