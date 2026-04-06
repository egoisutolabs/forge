package tui

import "time"

// UndoEntry captures the input state at a point in time.
type UndoEntry struct {
	Text   string
	Cursor int
}

// UndoStack provides undo/redo for text input with time-based debounce.
// Entries are coalesced when changes arrive within the debounce window.
type UndoStack struct {
	entries  []UndoEntry
	pos      int           // current position in entries (-1 = empty)
	maxSize  int           // maximum number of entries
	debounce time.Duration // minimum gap between recorded snapshots
	lastPush time.Time     // timestamp of last snapshot
}

// NewUndoStack creates an UndoStack with the given capacity and debounce duration.
func NewUndoStack(maxSize int, debounce time.Duration) *UndoStack {
	if maxSize <= 0 {
		maxSize = 50
	}
	if debounce <= 0 {
		debounce = 1000 * time.Millisecond
	}
	return &UndoStack{
		entries:  make([]UndoEntry, 0, maxSize),
		pos:      -1,
		maxSize:  maxSize,
		debounce: debounce,
	}
}

// Push records a new state. If called within the debounce window of the last
// push, the most recent entry is updated in-place instead of appending.
func (u *UndoStack) Push(text string, cursor int) {
	now := time.Now()
	u.pushAt(text, cursor, now)
}

// pushAt is the testable version of Push that accepts an explicit timestamp.
func (u *UndoStack) pushAt(text string, cursor int, now time.Time) {
	entry := UndoEntry{Text: text, Cursor: cursor}

	// If within debounce window, update the current entry in-place
	if u.pos >= 0 && now.Sub(u.lastPush) < u.debounce {
		u.entries[u.pos] = entry
		u.lastPush = now
		return
	}

	// Discard any redo history beyond current position
	u.entries = u.entries[:u.pos+1]

	u.entries = append(u.entries, entry)

	// Trim from front if over capacity
	if len(u.entries) > u.maxSize {
		u.entries = u.entries[len(u.entries)-u.maxSize:]
	}

	u.pos = len(u.entries) - 1
	u.lastPush = now
}

// Undo moves back one entry and returns the previous state.
// Returns ok=false if there is nothing to undo.
func (u *UndoStack) Undo() (UndoEntry, bool) {
	if u.pos <= 0 {
		return UndoEntry{}, false
	}
	u.pos--
	return u.entries[u.pos], true
}

// Redo moves forward one entry and returns the restored state.
// Returns ok=false if there is nothing to redo.
func (u *UndoStack) Redo() (UndoEntry, bool) {
	if u.pos >= len(u.entries)-1 {
		return UndoEntry{}, false
	}
	u.pos++
	return u.entries[u.pos], true
}

// Clear resets the undo stack (e.g. on submit).
func (u *UndoStack) Clear() {
	u.entries = u.entries[:0]
	u.pos = -1
	u.lastPush = time.Time{}
}

// Len returns the number of entries in the stack.
func (u *UndoStack) Len() int {
	return len(u.entries)
}

// Pos returns the current position in the stack (-1 if empty).
func (u *UndoStack) Pos() int {
	return u.pos
}

// CanUndo returns true if an undo is available.
func (u *UndoStack) CanUndo() bool {
	return u.pos > 0
}

// CanRedo returns true if a redo is available.
func (u *UndoStack) CanRedo() bool {
	return u.pos < len(u.entries)-1
}
