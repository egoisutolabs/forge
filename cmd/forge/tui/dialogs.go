package tui

import (
	"sort"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// DialogPriority determines the order in which dialogs are shown.
// Lower numeric value = higher priority.
type DialogPriority int

const (
	DialogPermission   DialogPriority = iota // highest — tool approval
	DialogAskUser                            // engine question
	DialogCostWarning                        // budget threshold
	DialogIdleReturn                         // idle return prompt
	DialogNotification                       // lowest — transient notices
)

// QueuedDialog is a dialog waiting to be shown or currently active.
type QueuedDialog struct {
	Priority  DialogPriority
	ID        string // unique identifier (e.g. "perm", "askuser", "cost-5.00")
	Render    func() string
	HandleKey func(tea.KeyMsg) (handled bool, close bool)
	InitCmd   tea.Cmd // optional command to run when dialog becomes active
}

// DialogQueue manages a single-active-dialog model with priority queueing.
// When a higher-priority dialog arrives while a lower-priority one is active,
// the lower-priority dialog is queued and the higher-priority one takes over.
type DialogQueue struct {
	active  *QueuedDialog
	pending []QueuedDialog
	mu      sync.Mutex
}

// NewDialogQueue creates an empty dialog queue.
func NewDialogQueue() *DialogQueue {
	return &DialogQueue{}
}

// Push adds a dialog to the queue. If no dialog is active, it becomes active
// immediately. If the new dialog has higher priority (lower numeric value)
// than the current active dialog, the active one is queued and the new one
// takes its place. Returns the InitCmd if the dialog became active.
func (dq *DialogQueue) Push(d QueuedDialog) tea.Cmd {
	dq.mu.Lock()
	defer dq.mu.Unlock()

	if dq.active == nil {
		dq.active = &d
		return d.InitCmd
	}

	if d.Priority < dq.active.Priority {
		// New dialog has higher priority — demote current to pending
		dq.pending = append(dq.pending, *dq.active)
		dq.active = &d
		return d.InitCmd
	}

	// Lower or equal priority — queue it
	dq.pending = append(dq.pending, d)
	return nil
}

// Close removes the active dialog and activates the next highest-priority
// dialog from the pending queue. Returns the InitCmd of the newly activated
// dialog, or nil if the queue is empty.
func (dq *DialogQueue) Close() tea.Cmd {
	dq.mu.Lock()
	defer dq.mu.Unlock()

	dq.active = nil

	if len(dq.pending) == 0 {
		return nil
	}

	// Sort pending by priority (lowest numeric = highest priority)
	sort.Slice(dq.pending, func(i, j int) bool {
		return dq.pending[i].Priority < dq.pending[j].Priority
	})

	// Activate the highest-priority pending dialog
	next := dq.pending[0]
	dq.pending = dq.pending[1:]
	dq.active = &next
	return next.InitCmd
}

// Active returns the currently active dialog, or nil if none.
func (dq *DialogQueue) Active() *QueuedDialog {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	return dq.active
}

// ActiveID returns the ID of the currently active dialog, or "".
func (dq *DialogQueue) ActiveID() string {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	if dq.active != nil {
		return dq.active.ID
	}
	return ""
}

// HasActive returns true if a dialog is currently active.
func (dq *DialogQueue) HasActive() bool {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	return dq.active != nil
}

// PendingCount returns the number of queued (non-active) dialogs.
func (dq *DialogQueue) PendingCount() int {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	return len(dq.pending)
}

// RemoveByID removes a dialog from the pending queue by ID.
// Does not affect the active dialog (use Close for that).
func (dq *DialogQueue) RemoveByID(id string) {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	for i, d := range dq.pending {
		if d.ID == id {
			dq.pending = append(dq.pending[:i], dq.pending[i+1:]...)
			return
		}
	}
}
