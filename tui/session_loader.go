package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Progressive session loading: when resuming a session, messages are
// loaded in batches so the UI remains responsive. A "Loading..." state
// is shown while messages are being loaded.

// SessionLoadState tracks the progress of loading session messages.
type SessionLoadState struct {
	messages  []DisplayMessage // all messages to load
	loaded    int              // how many have been loaded so far
	batchSize int              // messages per batch
	done      bool             // whether loading is complete
	startTime time.Time
}

// SessionLoadBatchMsg triggers loading the next batch of messages.
type SessionLoadBatchMsg struct{}

// SessionLoadDoneMsg signals that all session messages have been loaded.
type SessionLoadDoneMsg struct{}

// NewSessionLoadState creates a loader for the given messages.
// batchSize controls how many messages are loaded per frame.
func NewSessionLoadState(messages []DisplayMessage, batchSize int) *SessionLoadState {
	if batchSize < 1 {
		batchSize = 10
	}
	return &SessionLoadState{
		messages:  messages,
		batchSize: batchSize,
		startTime: time.Now(),
		done:      len(messages) == 0,
	}
}

// NextBatch returns the next batch of messages to append, advancing the
// internal cursor. Returns nil if all messages have been loaded.
func (sl *SessionLoadState) NextBatch() []DisplayMessage {
	if sl.done {
		return nil
	}
	end := sl.loaded + sl.batchSize
	if end > len(sl.messages) {
		end = len(sl.messages)
	}
	batch := sl.messages[sl.loaded:end]
	sl.loaded = end
	if sl.loaded >= len(sl.messages) {
		sl.done = true
	}
	return batch
}

// Done returns true if all messages have been loaded.
func (sl *SessionLoadState) Done() bool {
	return sl.done
}

// Progress returns a value between 0.0 and 1.0 representing load progress.
func (sl *SessionLoadState) Progress() float64 {
	if len(sl.messages) == 0 {
		return 1.0
	}
	return float64(sl.loaded) / float64(len(sl.messages))
}

// Loaded returns the number of messages loaded so far.
func (sl *SessionLoadState) Loaded() int {
	return sl.loaded
}

// Total returns the total number of messages to load.
func (sl *SessionLoadState) Total() int {
	return len(sl.messages)
}

// Elapsed returns how long loading has been in progress.
func (sl *SessionLoadState) Elapsed() time.Duration {
	return time.Since(sl.startTime)
}

// scheduleNextBatch returns a tea.Cmd that fires a SessionLoadBatchMsg
// after a short delay to yield to the render loop.
func scheduleNextBatch() tea.Cmd {
	return tea.Tick(time.Millisecond*8, func(_ time.Time) tea.Msg {
		return SessionLoadBatchMsg{}
	})
}
