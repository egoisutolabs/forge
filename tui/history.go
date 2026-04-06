package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// History provides input history with up/down navigation.
// It uses a ring buffer approach with a configurable max size.
type History struct {
	entries []string // oldest at index 0, newest at end
	cursor  int      // -1 = current input (not browsing), 0..len-1 = history index from newest
	draft   string   // saves current unsent input when browsing history
	maxSize int
}

// NewHistory creates a History with the given max size.
func NewHistory(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &History{
		entries: make([]string, 0, maxSize),
		cursor:  -1,
		maxSize: maxSize,
	}
}

// Add appends a new entry to history. Consecutive duplicates are ignored.
func (h *History) Add(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}
	// Deduplicate: skip if identical to most recent entry
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		return
	}
	h.entries = append(h.entries, entry)
	// Trim if over capacity
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	// Reset cursor after adding
	h.cursor = -1
	h.draft = ""
}

// Up moves to the previous (older) history entry.
// currentInput is the current textarea content to save as draft.
// Returns the history entry to display, and true if navigation occurred.
func (h *History) Up(currentInput string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}

	if h.cursor == -1 {
		// First time pressing up — save current input as draft
		h.draft = currentInput
		h.cursor = 0
	} else if h.cursor < len(h.entries)-1 {
		h.cursor++
	} else {
		// Already at oldest entry
		return h.entries[0], false
	}

	// cursor=0 is newest, cursor=len-1 is oldest
	idx := len(h.entries) - 1 - h.cursor
	return h.entries[idx], true
}

// Down moves to the next (newer) history entry.
// Returns the entry to display (or the draft), and true if navigation occurred.
func (h *History) Down() (string, bool) {
	if h.cursor <= -1 {
		return "", false
	}

	h.cursor--
	if h.cursor == -1 {
		// Back to current input — restore draft
		return h.draft, true
	}

	idx := len(h.entries) - 1 - h.cursor
	return h.entries[idx], true
}

// Reset exits history browsing mode without changing entries.
func (h *History) Reset() {
	h.cursor = -1
	h.draft = ""
}

// Browsing returns true if the user is currently navigating history.
func (h *History) Browsing() bool {
	return h.cursor >= 0
}

// Entries returns a copy of all history entries (oldest first).
func (h *History) Entries() []string {
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}

// Len returns the number of entries.
func (h *History) Len() int {
	return len(h.entries)
}

// DefaultHistoryPath returns ~/.forge/history.
func DefaultHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".forge", "history")
}

// LoadFromFile reads history entries from a file (one per line).
func (h *History) LoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			h.entries = append(h.entries, line)
		}
	}
	// Trim to max size
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
	return scanner.Err()
}

// SaveToFile writes history entries to a file (one per line).
// Creates parent directories if needed.
func (h *History) SaveToFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range h.entries {
		w.WriteString(entry)
		w.WriteByte('\n')
	}
	return w.Flush()
}
