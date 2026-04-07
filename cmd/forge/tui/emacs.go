package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
)

// KillRing implements an Emacs-style kill ring for text editing.
// Killed text is pushed onto the ring and can be yanked back.
// Consecutive kills accumulate (are joined) into a single entry.
type KillRing struct {
	ring        []string
	pos         int
	maxSize     int
	lastWasKill bool // true if the previous operation was a kill (for accumulation)
}

// NewKillRing creates a KillRing with the given max size.
func NewKillRing(maxSize int) *KillRing {
	if maxSize <= 0 {
		maxSize = 10
	}
	return &KillRing{
		ring:    make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Push adds killed text to the ring. If the previous operation was also a kill,
// the text is appended to the most recent entry instead of creating a new one.
func (kr *KillRing) Push(text string) {
	if text == "" {
		return
	}
	if kr.lastWasKill && len(kr.ring) > 0 {
		// Accumulate consecutive kills
		kr.ring[len(kr.ring)-1] += text
	} else {
		kr.ring = append(kr.ring, text)
		if len(kr.ring) > kr.maxSize {
			kr.ring = kr.ring[len(kr.ring)-kr.maxSize:]
		}
	}
	kr.pos = len(kr.ring) - 1
	kr.lastWasKill = true
}

// Yank returns the most recent kill ring entry, or "" if empty.
func (kr *KillRing) Yank() string {
	if len(kr.ring) == 0 {
		return ""
	}
	return kr.ring[kr.pos]
}

// YankPop cycles to the previous entry in the ring and returns it.
// Returns "" if the ring is empty.
func (kr *KillRing) YankPop() string {
	if len(kr.ring) == 0 {
		return ""
	}
	kr.pos--
	if kr.pos < 0 {
		kr.pos = len(kr.ring) - 1
	}
	return kr.ring[kr.pos]
}

// BreakAccumulate marks the end of a consecutive kill sequence.
// The next Push will create a new ring entry instead of appending.
func (kr *KillRing) BreakAccumulate() {
	kr.lastWasKill = false
}

// Len returns the number of entries in the ring.
func (kr *KillRing) Len() int {
	return len(kr.ring)
}

// EmacsKeys handles Emacs-style keybindings on a textarea value and cursor position.
// It returns the new value, new cursor position, and whether the key was handled.
type EmacsKeys struct {
	KillRing *KillRing
}

// HandleKey processes an Emacs keybinding and returns (newValue, newCursor, handled).
// The cursor is a byte offset into value.
func (e *EmacsKeys) HandleKey(key string, value string, cursor int) (string, int, bool) {
	switch key {
	case "ctrl+a":
		// Move to start of line
		e.KillRing.BreakAccumulate()
		lineStart := findLineStart(value, cursor)
		return value, lineStart, true

	case "ctrl+e":
		// Move to end of line
		e.KillRing.BreakAccumulate()
		lineEnd := findLineEnd(value, cursor)
		return value, lineEnd, true

	case "ctrl+k":
		// Kill from cursor to end of line
		lineEnd := findLineEnd(value, cursor)
		if cursor == lineEnd && cursor < len(value) {
			// At end of line — kill the newline
			killed := value[cursor : cursor+1]
			newVal := value[:cursor] + value[cursor+1:]
			e.KillRing.Push(killed)
			return newVal, cursor, true
		}
		killed := value[cursor:lineEnd]
		if killed == "" {
			return value, cursor, true
		}
		newVal := value[:cursor] + value[lineEnd:]
		e.KillRing.Push(killed)
		return newVal, cursor, true

	case "ctrl+u":
		// Kill entire line (from cursor to start)
		lineStart := findLineStart(value, cursor)
		killed := value[lineStart:cursor]
		if killed == "" {
			return value, cursor, true
		}
		newVal := value[:lineStart] + value[cursor:]
		e.KillRing.Push(killed)
		return newVal, lineStart, true

	case "ctrl+w":
		// Kill word backward
		if cursor == 0 {
			return value, cursor, true
		}
		wordStart := findWordBackward(value, cursor)
		killed := value[wordStart:cursor]
		newVal := value[:wordStart] + value[cursor:]
		e.KillRing.Push(killed)
		return newVal, wordStart, true

	case "ctrl+y":
		// Yank (paste) most recent kill ring entry
		e.KillRing.BreakAccumulate()
		text := e.KillRing.Yank()
		if text == "" {
			return value, cursor, true
		}
		newVal := value[:cursor] + text + value[cursor:]
		return newVal, cursor + len(text), true

	default:
		return value, cursor, false
	}
}

// findLineStart returns the byte offset of the start of the current line.
func findLineStart(s string, cursor int) int {
	if cursor <= 0 {
		return 0
	}
	idx := strings.LastIndex(s[:cursor], "\n")
	if idx < 0 {
		return 0
	}
	return idx + 1
}

// findLineEnd returns the byte offset of the end of the current line (before \n).
func findLineEnd(s string, cursor int) int {
	idx := strings.Index(s[cursor:], "\n")
	if idx < 0 {
		return len(s)
	}
	return cursor + idx
}

// emacsInputCursor computes the byte offset into the textarea's full value
// based on the current line and column position.
func emacsInputCursor(ta *textarea.Model) int {
	val := ta.Value()
	line := ta.Line()
	info := ta.LineInfo()
	col := info.CharOffset

	// Walk through the value counting newlines to find the byte offset
	offset := 0
	currentLine := 0
	for i, ch := range val {
		if currentLine == line {
			offset = i + col
			break
		}
		if ch == '\n' {
			currentLine++
		}
	}
	// If we're on the last line and didn't break early
	if currentLine < line {
		offset = len(val)
	}
	if offset > len(val) {
		offset = len(val)
	}
	return offset
}

// findWordBackward returns the byte offset of the start of the previous word.
func findWordBackward(s string, cursor int) int {
	if cursor <= 0 {
		return 0
	}
	i := cursor - 1
	// Skip trailing whitespace
	for i > 0 && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i--
	}
	// Skip word characters
	for i > 0 && s[i-1] != ' ' && s[i-1] != '\t' && s[i-1] != '\n' {
		i--
	}
	return i
}
