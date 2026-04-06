package tui

import (
	"regexp"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// autoBackgroundTimeout is the default duration of no user interaction
	// during processing before the agent is automatically backgrounded.
	autoBackgroundTimeout = 60 * time.Second

	// bgStallCheckInterval is how often the background stall watchdog checks output.
	bgStallCheckInterval = 5 * time.Second

	// bgStallThreshold is how long output must be unchanged before a stall notification fires.
	bgStallThreshold = 45 * time.Second

	// panelEvictionGrace is how long a completed agent stays in the footer before eviction.
	panelEvictionGrace = 30 * time.Second
)

// interactivePattern matches common interactive prompts (y/n, Continue?, etc.)
var interactivePattern = regexp.MustCompile(`(?i)(y/n|\[y\]|\[n\]|continue\?|proceed\?|confirm\?|press enter|are you sure)`)

// BackgroundAgent represents a backgrounded agent with tracking state.
type BackgroundAgent struct {
	Name          string
	StartTime     time.Time
	OutputLen     int       // length of last observed output
	LastGrowth    time.Time // when output last grew
	StallNotified bool      // whether a stall notification was already sent
	Completed     bool      // whether the agent has finished
	CompletedAt   time.Time // when the agent completed
	Viewing       bool      // whether user is currently viewing this agent
	LastOutput    string    // last chunk of output (for interactive pattern matching)
}

// BackgroundState manages all backgrounded agents and auto-background logic.
type BackgroundState struct {
	Agents          map[string]*BackgroundAgent
	AutoTimeout     time.Duration // configurable auto-background timeout
	lastInteraction time.Time     // last user interaction during processing
	backgrounded    bool          // whether current processing is backgrounded
}

// NewBackgroundState creates a new background state tracker.
func NewBackgroundState() *BackgroundState {
	return &BackgroundState{
		Agents:      make(map[string]*BackgroundAgent),
		AutoTimeout: autoBackgroundTimeout,
	}
}

// RecordInteraction records that the user interacted (resets auto-background timer).
func (bs *BackgroundState) RecordInteraction() {
	bs.lastInteraction = time.Now()
}

// IsBackgrounded returns whether the current processing is backgrounded.
func (bs *BackgroundState) IsBackgrounded() bool {
	return bs.backgrounded
}

// Background moves the current processing to background.
func (bs *BackgroundState) Background(name string) {
	bs.backgrounded = true
	if _, exists := bs.Agents[name]; !exists {
		now := time.Now()
		bs.Agents[name] = &BackgroundAgent{
			Name:       name,
			StartTime:  now,
			LastGrowth: now,
		}
	}
}

// Foreground returns processing to foreground.
func (bs *BackgroundState) Foreground() {
	bs.backgrounded = false
}

// OnProcessingStart resets background state for a new processing cycle.
func (bs *BackgroundState) OnProcessingStart() {
	bs.backgrounded = false
	bs.lastInteraction = time.Now()
}

// OnProcessingDone clears the backgrounded flag.
func (bs *BackgroundState) OnProcessingDone() {
	bs.backgrounded = false
}

// CheckAutoBackground checks if processing should be auto-backgrounded.
// Returns true if the auto-background timeout has elapsed with no interaction.
func (bs *BackgroundState) CheckAutoBackground(now time.Time) bool {
	if bs.backgrounded {
		return false // already backgrounded
	}
	if bs.lastInteraction.IsZero() {
		return false
	}
	return now.Sub(bs.lastInteraction) >= bs.AutoTimeout
}

// MarkCompleted marks a background agent as completed and starts the eviction timer.
func (bs *BackgroundState) MarkCompleted(name string) {
	if agent, ok := bs.Agents[name]; ok {
		agent.Completed = true
		agent.CompletedAt = time.Now()
	}
}

// SetViewing marks whether the user is viewing a background agent.
func (bs *BackgroundState) SetViewing(name string, viewing bool) {
	if agent, ok := bs.Agents[name]; ok {
		agent.Viewing = viewing
	}
}

// UpdateOutput records new output length for stall detection.
func (bs *BackgroundState) UpdateOutput(name string, outputLen int, lastOutput string) {
	agent, ok := bs.Agents[name]
	if !ok {
		return
	}
	if outputLen > agent.OutputLen {
		agent.OutputLen = outputLen
		agent.LastGrowth = time.Now()
		agent.StallNotified = false // reset stall notification on new output
	}
	agent.LastOutput = lastOutput
}

// CheckStalls checks all background agents for stalls.
// Returns a list of agent names that are newly stalled and match interactive patterns.
func (bs *BackgroundState) CheckStalls(now time.Time) []string {
	var stalled []string
	for name, agent := range bs.Agents {
		if agent.Completed || agent.StallNotified {
			continue
		}
		if now.Sub(agent.LastGrowth) >= bgStallThreshold {
			// Check if last output matches interactive patterns
			if interactivePattern.MatchString(agent.LastOutput) {
				agent.StallNotified = true
				stalled = append(stalled, name)
			}
		}
	}
	return stalled
}

// EvictCompleted removes completed agents past their grace period.
// Agents currently being viewed are not evicted.
// Returns names of evicted agents.
func (bs *BackgroundState) EvictCompleted(now time.Time) []string {
	var evicted []string
	for name, agent := range bs.Agents {
		if !agent.Completed {
			continue
		}
		if agent.Viewing {
			continue // user is viewing — don't evict
		}
		if now.Sub(agent.CompletedAt) >= panelEvictionGrace {
			evicted = append(evicted, name)
		}
	}
	for _, name := range evicted {
		delete(bs.Agents, name)
	}
	return evicted
}

// AgentCount returns the number of active (non-completed) background agents.
func (bs *BackgroundState) AgentCount() int {
	count := 0
	for _, agent := range bs.Agents {
		if !agent.Completed {
			count++
		}
	}
	return count
}

// AllAgents returns a snapshot of all tracked agents.
func (bs *BackgroundState) AllAgents() []*BackgroundAgent {
	agents := make([]*BackgroundAgent, 0, len(bs.Agents))
	for _, agent := range bs.Agents {
		agents = append(agents, agent)
	}
	return agents
}

// Remove removes an agent from tracking entirely.
func (bs *BackgroundState) Remove(name string) {
	delete(bs.Agents, name)
}

// --- Bubbletea messages ---

// BackgroundAgentMsg is sent when the user presses Ctrl+B to background processing.
type BackgroundAgentMsg struct{}

// AutoBackgroundMsg is sent when the auto-background timeout fires.
type AutoBackgroundMsg struct{}

// BgStallCheckMsg triggers periodic stall checks on background agents.
type BgStallCheckMsg time.Time

// BgEvictionCheckMsg triggers periodic eviction checks on completed agents.
type BgEvictionCheckMsg time.Time

// bgStallCheck returns a command that fires a BgStallCheckMsg periodically.
func bgStallCheck() tea.Cmd {
	return tea.Tick(bgStallCheckInterval, func(t time.Time) tea.Msg {
		return BgStallCheckMsg(t)
	})
}

// bgEvictionCheck returns a command that fires a BgEvictionCheckMsg periodically.
func bgEvictionCheck() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return BgEvictionCheckMsg(t)
	})
}

// autoBackgroundTick returns a command that fires an AutoBackgroundMsg after the timeout.
func autoBackgroundTick(timeout time.Duration) tea.Cmd {
	return tea.Tick(timeout, func(t time.Time) tea.Msg {
		return AutoBackgroundMsg{}
	})
}
