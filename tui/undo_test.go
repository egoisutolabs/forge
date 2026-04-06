package tui

import (
	"testing"
	"time"
)

func TestUndoStack_PushAndUndo(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	// Push three entries with gaps > debounce
	u.pushAt("a", 1, now)
	u.pushAt("ab", 2, now.Add(2*time.Second))
	u.pushAt("abc", 3, now.Add(4*time.Second))

	if u.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", u.Len())
	}

	// Undo should return "ab"
	entry, ok := u.Undo()
	if !ok || entry.Text != "ab" || entry.Cursor != 2 {
		t.Fatalf("expected {ab, 2}, got {%q, %d} ok=%v", entry.Text, entry.Cursor, ok)
	}

	// Undo again should return "a"
	entry, ok = u.Undo()
	if !ok || entry.Text != "a" || entry.Cursor != 1 {
		t.Fatalf("expected {a, 1}, got {%q, %d}", entry.Text, entry.Cursor)
	}

	// Undo at beginning should fail
	_, ok = u.Undo()
	if ok {
		t.Fatal("expected ok=false at beginning of stack")
	}
}

func TestUndoStack_Redo(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	u.pushAt("a", 1, now)
	u.pushAt("ab", 2, now.Add(2*time.Second))
	u.pushAt("abc", 3, now.Add(4*time.Second))

	// Undo twice
	u.Undo()
	u.Undo()

	// Redo should go forward
	entry, ok := u.Redo()
	if !ok || entry.Text != "ab" {
		t.Fatalf("expected {ab}, got {%q} ok=%v", entry.Text, ok)
	}

	entry, ok = u.Redo()
	if !ok || entry.Text != "abc" {
		t.Fatalf("expected {abc}, got {%q}", entry.Text)
	}

	// Redo at end should fail
	_, ok = u.Redo()
	if ok {
		t.Fatal("expected ok=false at end of stack")
	}
}

func TestUndoStack_RedoDiscardedByNewPush(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	u.pushAt("a", 1, now)
	u.pushAt("ab", 2, now.Add(2*time.Second))
	u.pushAt("abc", 3, now.Add(4*time.Second))

	// Undo to "ab"
	u.Undo()

	// Push new content — should discard "abc"
	u.pushAt("ax", 2, now.Add(6*time.Second))

	if u.Len() != 3 {
		t.Fatalf("expected 3 entries after discard, got %d", u.Len())
	}

	// Redo should fail (redo history was discarded)
	_, ok := u.Redo()
	if ok {
		t.Fatal("expected redo to fail after new push")
	}
}

func TestUndoStack_Debounce(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	// Push multiple times within debounce window
	u.pushAt("a", 1, now)
	u.pushAt("ab", 2, now.Add(100*time.Millisecond))
	u.pushAt("abc", 3, now.Add(200*time.Millisecond))

	// Should have coalesced into 1 entry
	if u.Len() != 1 {
		t.Fatalf("expected 1 coalesced entry, got %d", u.Len())
	}

	// The entry should be the latest value
	entry, ok := u.Redo()
	if ok {
		t.Fatalf("expected no redo; entry=%v", entry)
	}

	// Push after debounce gap creates new entry
	u.pushAt("abcd", 4, now.Add(2*time.Second))
	if u.Len() != 2 {
		t.Fatalf("expected 2 entries after gap, got %d", u.Len())
	}
}

func TestUndoStack_MaxSize(t *testing.T) {
	u := NewUndoStack(3, 1*time.Millisecond)
	now := time.Now()

	for i := 0; i < 5; i++ {
		u.pushAt("entry"+string(rune('a'+i)), i, now.Add(time.Duration(i)*time.Second))
	}

	if u.Len() != 3 {
		t.Fatalf("expected 3 entries (capped), got %d", u.Len())
	}
}

func TestUndoStack_Clear(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	u.pushAt("a", 1, now)
	u.pushAt("b", 1, now.Add(2*time.Second))
	u.Clear()

	if u.Len() != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", u.Len())
	}
	if u.Pos() != -1 {
		t.Fatalf("expected pos=-1 after clear, got %d", u.Pos())
	}
}

func TestUndoStack_CanUndoRedo(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	now := time.Now()

	if u.CanUndo() {
		t.Fatal("expected CanUndo=false on empty stack")
	}
	if u.CanRedo() {
		t.Fatal("expected CanRedo=false on empty stack")
	}

	u.pushAt("a", 1, now)
	if u.CanUndo() {
		t.Fatal("expected CanUndo=false with single entry")
	}

	u.pushAt("ab", 2, now.Add(2*time.Second))
	if !u.CanUndo() {
		t.Fatal("expected CanUndo=true with two entries")
	}
	if u.CanRedo() {
		t.Fatal("expected CanRedo=false at end")
	}

	u.Undo()
	if !u.CanRedo() {
		t.Fatal("expected CanRedo=true after undo")
	}
}

func TestUndoStack_EmptyUndo(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	_, ok := u.Undo()
	if ok {
		t.Fatal("expected ok=false on empty stack")
	}
}

func TestUndoStack_EmptyRedo(t *testing.T) {
	u := NewUndoStack(50, 1000*time.Millisecond)
	_, ok := u.Redo()
	if ok {
		t.Fatal("expected ok=false on empty stack")
	}
}

func TestUndoStack_DefaultValues(t *testing.T) {
	u := NewUndoStack(0, 0)
	if u.maxSize != 50 {
		t.Fatalf("expected default maxSize=50, got %d", u.maxSize)
	}
	if u.debounce != 1000*time.Millisecond {
		t.Fatalf("expected default debounce=1s, got %v", u.debounce)
	}
}
