package tui

import (
	"testing"
	"time"
)

func TestIdleState_NoTriggerBeforeThreshold(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = false
	s.lastActivityTime = time.Now().Add(-10 * time.Minute)

	idle, _ := s.CheckIdle(time.Now())
	if idle {
		t.Fatal("expected not idle after only 10 minutes")
	}
}

func TestIdleState_TriggersAfterThreshold(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = false
	s.lastActivityTime = time.Now().Add(-45 * time.Minute)

	idle, duration := s.CheckIdle(time.Now())
	if !idle {
		t.Fatal("expected idle after 45 minutes")
	}
	if duration < 44*time.Minute {
		t.Fatalf("expected duration ~45m, got %v", duration)
	}
}

func TestIdleState_NeverAskSuppresses(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = true
	s.lastActivityTime = time.Now().Add(-2 * time.Hour)

	idle, _ := s.CheckIdle(time.Now())
	if idle {
		t.Fatal("expected not idle when neverAsk is set")
	}
}

func TestIdleState_RecordActivityResetsTimer(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = false
	s.lastActivityTime = time.Now().Add(-45 * time.Minute)

	s.RecordActivity()

	idle, _ := s.CheckIdle(time.Now())
	if idle {
		t.Fatal("expected not idle after RecordActivity")
	}
}

func TestIdleState_DialogBlocking(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = false
	s.lastActivityTime = time.Now().Add(-45 * time.Minute)

	theme := ResolveTheme(DarkTheme())
	s.ShowDialog(45*time.Minute, theme)

	if !s.IsDialogShowing() {
		t.Fatal("expected dialog to be showing")
	}

	// While dialog is showing, CheckIdle should not trigger again
	idle, _ := s.CheckIdle(time.Now())
	if idle {
		t.Fatal("expected not idle while dialog is showing")
	}
}

func TestIdleState_DismissDialog(t *testing.T) {
	s := NewIdleState()
	s.neverAsk = false

	theme := ResolveTheme(DarkTheme())
	s.ShowDialog(45*time.Minute, theme)
	s.DismissDialog()

	if s.IsDialogShowing() {
		t.Fatal("expected dialog to be dismissed")
	}
}

func TestFormatIdleDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{45 * time.Minute, "45m"},
		{60 * time.Minute, "1h"},
		{90 * time.Minute, "1h30m"},
		{2 * time.Hour, "2h"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
	}

	for _, tt := range tests {
		got := formatIdleDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatIdleDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestRenderIdleDialog_Empty(t *testing.T) {
	s := NewIdleState()
	theme := ResolveTheme(DarkTheme())

	result := renderIdleDialog(s, 80, theme)
	if result != "" {
		t.Fatal("expected empty render when dialog not showing")
	}
}
