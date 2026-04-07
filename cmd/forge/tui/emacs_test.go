package tui

import "testing"

// ---- KillRing tests ----

func TestKillRing_PushAndYank(t *testing.T) {
	kr := NewKillRing(10)
	kr.Push("hello")
	kr.BreakAccumulate()
	kr.Push("world")

	got := kr.Yank()
	if got != "world" {
		t.Fatalf("expected yank to return %q, got %q", "world", got)
	}
}

func TestKillRing_YankPop(t *testing.T) {
	kr := NewKillRing(10)
	kr.Push("first")
	kr.BreakAccumulate()
	kr.Push("second")
	kr.BreakAccumulate()
	kr.Push("third")

	if got := kr.Yank(); got != "third" {
		t.Fatalf("expected %q, got %q", "third", got)
	}
	if got := kr.YankPop(); got != "second" {
		t.Fatalf("expected %q, got %q", "second", got)
	}
	if got := kr.YankPop(); got != "first" {
		t.Fatalf("expected %q, got %q", "first", got)
	}
	// Wrap around
	if got := kr.YankPop(); got != "third" {
		t.Fatalf("expected wrap to %q, got %q", "third", got)
	}
}

func TestKillRing_ConsecutiveKillsAccumulate(t *testing.T) {
	kr := NewKillRing(10)
	kr.Push("hello ")
	kr.Push("world") // consecutive — should append

	if kr.Len() != 1 {
		t.Fatalf("expected 1 entry (accumulated), got %d", kr.Len())
	}
	if got := kr.Yank(); got != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", got)
	}
}

func TestKillRing_BreakStopsAccumulation(t *testing.T) {
	kr := NewKillRing(10)
	kr.Push("first")
	kr.BreakAccumulate()
	kr.Push("second")

	if kr.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", kr.Len())
	}
}

func TestKillRing_MaxSize(t *testing.T) {
	kr := NewKillRing(3)
	for i := 0; i < 5; i++ {
		kr.BreakAccumulate()
		kr.Push("entry")
	}
	if kr.Len() != 3 {
		t.Fatalf("expected max 3 entries, got %d", kr.Len())
	}
}

func TestKillRing_EmptyYank(t *testing.T) {
	kr := NewKillRing(10)
	if got := kr.Yank(); got != "" {
		t.Fatalf("expected empty yank, got %q", got)
	}
}

func TestKillRing_PushEmpty(t *testing.T) {
	kr := NewKillRing(10)
	kr.Push("")
	if kr.Len() != 0 {
		t.Fatalf("expected 0 entries after pushing empty string, got %d", kr.Len())
	}
}

// ---- EmacsKeys tests ----

func TestEmacs_CtrlA_MovesToStart(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	val, cursor, handled := ek.HandleKey("ctrl+a", "hello world", 5)

	if !handled {
		t.Fatal("expected ctrl+a to be handled")
	}
	if val != "hello world" {
		t.Fatalf("expected value unchanged, got %q", val)
	}
	if cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", cursor)
	}
}

func TestEmacs_CtrlE_MovesToEnd(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	val, cursor, handled := ek.HandleKey("ctrl+e", "hello world", 5)

	if !handled {
		t.Fatal("expected ctrl+e to be handled")
	}
	if val != "hello world" {
		t.Fatalf("expected value unchanged, got %q", val)
	}
	if cursor != 11 {
		t.Fatalf("expected cursor at 11, got %d", cursor)
	}
}

func TestEmacs_CtrlK_KillToEndOfLine(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	val, cursor, handled := ek.HandleKey("ctrl+k", "hello world", 5)

	if !handled {
		t.Fatal("expected ctrl+k to be handled")
	}
	if val != "hello" {
		t.Fatalf("expected %q, got %q", "hello", val)
	}
	if cursor != 5 {
		t.Fatalf("expected cursor at 5, got %d", cursor)
	}
	if got := ek.KillRing.Yank(); got != " world" {
		t.Fatalf("expected kill ring to contain %q, got %q", " world", got)
	}
}

func TestEmacs_CtrlU_KillEntireLine(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	val, cursor, handled := ek.HandleKey("ctrl+u", "hello world", 5)

	if !handled {
		t.Fatal("expected ctrl+u to be handled")
	}
	if val != " world" {
		t.Fatalf("expected %q, got %q", " world", val)
	}
	if cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", cursor)
	}
	if got := ek.KillRing.Yank(); got != "hello" {
		t.Fatalf("expected kill ring to contain %q, got %q", "hello", got)
	}
}

func TestEmacs_CtrlW_KillWordBackward(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	val, cursor, handled := ek.HandleKey("ctrl+w", "hello world", 11)

	if !handled {
		t.Fatal("expected ctrl+w to be handled")
	}
	if val != "hello " {
		t.Fatalf("expected %q, got %q", "hello ", val)
	}
	if cursor != 6 {
		t.Fatalf("expected cursor at 6, got %d", cursor)
	}
	if got := ek.KillRing.Yank(); got != "world" {
		t.Fatalf("expected kill ring to contain %q, got %q", "world", got)
	}
}

func TestEmacs_CtrlY_YankText(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	// Kill something first
	ek.HandleKey("ctrl+k", "hello world", 5)

	// Yank it back into a new string
	val, cursor, handled := ek.HandleKey("ctrl+y", "hello", 5)

	if !handled {
		t.Fatal("expected ctrl+y to be handled")
	}
	if val != "hello world" {
		t.Fatalf("expected %q, got %q", "hello world", val)
	}
	if cursor != 11 {
		t.Fatalf("expected cursor at 11, got %d", cursor)
	}
}

func TestEmacs_CtrlK_ConsecutiveKillsAccumulate(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}

	// Kill "world" from "hello world"
	val, cursor, _ := ek.HandleKey("ctrl+k", "hello world", 5)
	// val = "hello", cursor = 5

	// Kill "llo" from "hello"
	val, _, _ = ek.HandleKey("ctrl+k", val, 2)
	// val = "he", cursor = 2

	// Consecutive kills should have accumulated
	if got := ek.KillRing.Yank(); got != " worldllo" {
		t.Fatalf("expected accumulated kill %q, got %q", " worldllo", got)
	}
	_ = cursor
	_ = val
}

func TestEmacs_UnknownKey_NotHandled(t *testing.T) {
	ek := &EmacsKeys{KillRing: NewKillRing(10)}
	_, _, handled := ek.HandleKey("ctrl+x", "hello", 3)
	if handled {
		t.Fatal("expected unknown key to not be handled")
	}
}

// ---- Helper function tests ----

func TestFindLineStart(t *testing.T) {
	tests := []struct {
		s      string
		cursor int
		want   int
	}{
		{"hello", 3, 0},
		{"hello\nworld", 8, 6},
		{"hello\nworld", 0, 0},
		{"a\nb\nc", 4, 4},
	}
	for _, tt := range tests {
		got := findLineStart(tt.s, tt.cursor)
		if got != tt.want {
			t.Errorf("findLineStart(%q, %d) = %d, want %d", tt.s, tt.cursor, got, tt.want)
		}
	}
}

func TestFindLineEnd(t *testing.T) {
	tests := []struct {
		s      string
		cursor int
		want   int
	}{
		{"hello", 3, 5},
		{"hello\nworld", 3, 5},
		{"hello\nworld", 6, 11},
	}
	for _, tt := range tests {
		got := findLineEnd(tt.s, tt.cursor)
		if got != tt.want {
			t.Errorf("findLineEnd(%q, %d) = %d, want %d", tt.s, tt.cursor, got, tt.want)
		}
	}
}

func TestFindWordBackward(t *testing.T) {
	tests := []struct {
		s      string
		cursor int
		want   int
	}{
		{"hello world", 11, 6},
		{"hello world", 5, 0},
		{"hello  world", 12, 7},
		{"a", 1, 0},
		{"", 0, 0},
	}
	for _, tt := range tests {
		got := findWordBackward(tt.s, tt.cursor)
		if got != tt.want {
			t.Errorf("findWordBackward(%q, %d) = %d, want %d", tt.s, tt.cursor, got, tt.want)
		}
	}
}
