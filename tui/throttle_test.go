package tui

import (
	"testing"
	"time"
)

func TestThrottledValue_ImmediateFirstSet(t *testing.T) {
	tv := NewThrottledValue(100*time.Millisecond, "initial")

	// After minDisplay, Set should update immediately.
	time.Sleep(110 * time.Millisecond)
	got := tv.Set("updated")
	if got != "updated" {
		t.Fatalf("expected 'updated', got %q", got)
	}
	if tv.Current() != "updated" {
		t.Fatalf("expected current='updated', got %q", tv.Current())
	}
}

func TestThrottledValue_HoldsMinDuration(t *testing.T) {
	tv := NewThrottledValue(200*time.Millisecond, "first")

	// Immediately set a new value — should be held as pending.
	got := tv.Set("second")
	if got != "first" {
		t.Fatalf("expected 'first' (held), got %q", got)
	}
	if tv.Current() != "first" {
		t.Fatalf("expected current='first', got %q", tv.Current())
	}
	if !tv.HasPending() {
		t.Fatal("expected pending value")
	}
}

func TestThrottledValue_PendingActivatesOnTick(t *testing.T) {
	tv := NewThrottledValue(50*time.Millisecond, "first")

	// Set immediately — goes to pending.
	tv.Set("second")
	if tv.Current() != "first" {
		t.Fatalf("expected 'first' before timeout, got %q", tv.Current())
	}

	// Wait for minDisplay to elapse.
	time.Sleep(60 * time.Millisecond)
	got := tv.Tick()
	if got != "second" {
		t.Fatalf("expected 'second' after tick, got %q", got)
	}
	if tv.HasPending() {
		t.Fatal("expected no pending after tick activated it")
	}
}

func TestThrottledValue_TickBeforeMinDisplay(t *testing.T) {
	tv := NewThrottledValue(500*time.Millisecond, "first")

	tv.Set("second")
	// Tick immediately — min display hasn't elapsed.
	got := tv.Tick()
	if got != "first" {
		t.Fatalf("expected 'first' before minDisplay, got %q", got)
	}
	if !tv.HasPending() {
		t.Fatal("pending should still be there")
	}
}

func TestThrottledValue_PendingReplaced(t *testing.T) {
	tv := NewThrottledValue(200*time.Millisecond, "first")

	// Rapid-fire updates — only the latest should be pending.
	tv.Set("second")
	tv.Set("third")
	tv.Set("fourth")

	if tv.Current() != "first" {
		t.Fatalf("expected 'first' while holding, got %q", tv.Current())
	}

	// Wait and tick — should show "fourth" (latest pending).
	time.Sleep(210 * time.Millisecond)
	got := tv.Tick()
	if got != "fourth" {
		t.Fatalf("expected 'fourth' (latest pending), got %q", got)
	}
}

func TestThrottledValue_NoPending_TickNoOp(t *testing.T) {
	tv := NewThrottledValue(100*time.Millisecond, "stable")

	got := tv.Tick()
	if got != "stable" {
		t.Fatalf("expected 'stable', got %q", got)
	}
}

func TestThrottledValue_Remaining(t *testing.T) {
	tv := NewThrottledValue(200*time.Millisecond, "first")

	// No pending → remaining should be 0.
	if tv.Remaining() != 0 {
		t.Fatalf("expected 0 remaining with no pending, got %v", tv.Remaining())
	}

	// Set pending → remaining should be > 0.
	tv.Set("second")
	rem := tv.Remaining()
	if rem <= 0 {
		t.Fatalf("expected positive remaining, got %v", rem)
	}
	if rem > 200*time.Millisecond {
		t.Fatalf("remaining should not exceed minDisplay, got %v", rem)
	}
}

func TestThrottledValue_IntType(t *testing.T) {
	tv := NewThrottledValue(50*time.Millisecond, 0)

	time.Sleep(60 * time.Millisecond)
	got := tv.Set(42)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestThrottledValue_SetAfterMinDisplay(t *testing.T) {
	tv := NewThrottledValue(30*time.Millisecond, "a")

	// Wait past minDisplay.
	time.Sleep(40 * time.Millisecond)
	got := tv.Set("b")
	if got != "b" {
		t.Fatalf("expected immediate update to 'b', got %q", got)
	}
	if tv.HasPending() {
		t.Fatal("should have no pending after immediate set")
	}

	// Set again immediately — should go to pending since lastShown was just reset.
	got = tv.Set("c")
	if got != "b" {
		t.Fatalf("expected held at 'b', got %q", got)
	}
	if !tv.HasPending() {
		t.Fatal("expected pending value 'c'")
	}
}
