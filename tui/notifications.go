package tui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// NotifPriority controls display order and auto-dismiss duration.
type NotifPriority int

const (
	NotifImmediate NotifPriority = iota // preempts current, no auto-dismiss
	NotifHigh                           // 5–60s display
	NotifMedium                         // 30s display
	NotifLow                            // 10s display
)

// defaultTimeout returns the default auto-dismiss duration for a priority.
func defaultTimeout(p NotifPriority) time.Duration {
	switch p {
	case NotifImmediate:
		return 0 // no auto-dismiss
	case NotifHigh:
		return 30 * time.Second
	case NotifMedium:
		return 30 * time.Second
	case NotifLow:
		return 10 * time.Second
	default:
		return 10 * time.Second
	}
}

// Notification is a single queued notification with priority and auto-dismiss.
type Notification struct {
	Key      string
	Message  string
	Priority NotifPriority
	Timeout  time.Duration // 0 = use default for priority
	Created  time.Time
}

// effectiveTimeout returns the auto-dismiss duration (Timeout if set, else default).
func (n Notification) effectiveTimeout() time.Duration {
	if n.Timeout > 0 {
		return n.Timeout
	}
	return defaultTimeout(n.Priority)
}

// expired returns true if the notification has outlived its timeout.
func (n Notification) expired(now time.Time) bool {
	timeout := n.effectiveTimeout()
	if timeout == 0 {
		return false // Immediate: never auto-expires
	}
	return now.Sub(n.Created) >= timeout
}

// NotifQueue is a thread-safe, priority-ordered notification queue.
// Notifications are deduped by Key; pushing a duplicate updates the message and
// resets the creation time.
type NotifQueue struct {
	items []Notification
	mu    sync.Mutex
}

// NewNotifQueue creates an empty notification queue.
func NewNotifQueue() *NotifQueue {
	return &NotifQueue{}
}

// Push adds or updates a notification. If a notification with the same key
// already exists, the message and created time are updated (folding). Immediate
// notifications are placed at the front of the queue.
func (q *NotifQueue) Push(n Notification) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if n.Created.IsZero() {
		n.Created = time.Now()
	}

	// Dedup: update existing notification with same key
	for i, existing := range q.items {
		if existing.Key == n.Key {
			q.items[i].Message = n.Message
			q.items[i].Created = n.Created
			q.items[i].Priority = n.Priority
			q.items[i].Timeout = n.Timeout
			return
		}
	}

	// Insert by priority (lower value = higher priority = earlier in slice)
	inserted := false
	for i, existing := range q.items {
		if n.Priority < existing.Priority {
			q.items = append(q.items, Notification{})
			copy(q.items[i+1:], q.items[i:])
			q.items[i] = n
			inserted = true
			break
		}
	}
	if !inserted {
		q.items = append(q.items, n)
	}
}

// Dismiss removes a notification by key. Returns true if found and removed.
func (q *NotifQueue) Dismiss(key string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, n := range q.items {
		if n.Key == key {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return true
		}
	}
	return false
}

// Tick removes expired notifications. Call on each frame or timer tick.
func (q *NotifQueue) Tick(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()

	filtered := q.items[:0]
	for _, n := range q.items {
		if !n.expired(now) {
			filtered = append(filtered, n)
		}
	}
	q.items = filtered
}

// Current returns the highest-priority (first) notification, or nil if empty.
func (q *NotifQueue) Current() *Notification {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}
	n := q.items[0]
	return &n
}

// All returns a copy of all pending notifications in priority order.
func (q *NotifQueue) All() []Notification {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]Notification, len(q.items))
	copy(out, q.items)
	return out
}

// Len returns the number of pending notifications.
func (q *NotifQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// NotifTickMsg is sent periodically to trigger auto-dismiss checks.
type NotifTickMsg time.Time

// notifTick returns a command that fires a NotifTickMsg every second.
func notifTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return NotifTickMsg(t)
	})
}

// renderNotifications renders the top notification above the input area.
func renderNotifications(q *NotifQueue, width int, theme Theme) string {
	n := q.Current()
	if n == nil {
		return ""
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Config.WarningColor)).
		Faint(true)

	msg := style.Render("  " + n.Message)
	if lipgloss.Width(msg) > width-2 {
		msg = msg[:width-2]
	}
	return msg + "\n"
}
