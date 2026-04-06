package tui

import (
	"testing"
	"time"
)

func TestStallState_NoStallInitially(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()

	level := s.Check(time.Now())
	if level != StallNone {
		t.Fatalf("expected StallNone, got %d", level)
	}
	if s.Stalled() {
		t.Fatal("expected not stalled")
	}
}

func TestStallState_WarningAfter3s(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-4 * time.Second)

	level := s.Check(time.Now())
	if level != StallWarning {
		t.Fatalf("expected StallWarning, got %d", level)
	}
	if !s.Stalled() {
		t.Fatal("expected stalled=true")
	}
}

func TestStallState_CriticalAfter6s(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-7 * time.Second)

	level := s.Check(time.Now())
	if level != StallCritical {
		t.Fatalf("expected StallCritical, got %d", level)
	}
}

func TestStallState_ResetOnStreamText(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-5 * time.Second)

	// Should be stalled
	level := s.Check(time.Now())
	if level != StallWarning {
		t.Fatalf("expected StallWarning before reset, got %d", level)
	}

	// New token arrives
	s.OnStreamText()

	level = s.Check(time.Now())
	if level != StallNone {
		t.Fatalf("expected StallNone after token, got %d", level)
	}
	if s.Stalled() {
		t.Fatal("expected not stalled after token")
	}
}

func TestStallState_SuppressedByActiveTools(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-10 * time.Second)
	s.OnToolStart()

	level := s.Check(time.Now())
	if level != StallNone {
		t.Fatalf("expected StallNone when tools active, got %d", level)
	}
	if s.Stalled() {
		t.Fatal("expected not stalled when tools active")
	}
}

func TestStallState_ResumeAfterToolsDone(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-5 * time.Second)
	s.OnToolStart()

	// Suppressed while tools active
	level := s.Check(time.Now())
	if level != StallNone {
		t.Fatalf("expected StallNone with active tools, got %d", level)
	}

	// Tools done, stall should resume
	s.OnToolDone(0)
	level = s.Check(time.Now())
	if level != StallWarning {
		t.Fatalf("expected StallWarning after tools done, got %d", level)
	}
}

func TestStallState_ProcessingDoneClears(t *testing.T) {
	s := NewStallState()
	s.OnProcessingStart()
	s.lastTokenTime = time.Now().Add(-5 * time.Second)
	s.Check(time.Now()) // sets stalled=true

	s.OnProcessingDone()
	if s.Stalled() {
		t.Fatal("expected not stalled after processing done")
	}
}

func TestStallSpinnerStyle(t *testing.T) {
	theme := ResolveTheme(DarkTheme())

	normal := stallSpinnerStyle(StallNone, theme)
	warning := stallSpinnerStyle(StallWarning, theme)
	critical := stallSpinnerStyle(StallCritical, theme)

	// Just verify they return different styles (not panicking)
	_ = normal.Render("test")
	_ = warning.Render("test")
	_ = critical.Render("test")
}
