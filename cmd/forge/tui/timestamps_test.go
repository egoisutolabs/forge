package tui

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRelativeTimeSince_JustNow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-5 * time.Second)
	got := formatRelativeTimeSince(ts, now)
	if got != "just now" {
		t.Fatalf("expected 'just now', got %q", got)
	}
}

func TestFormatRelativeTimeSince_OneMinute(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-60 * time.Second)
	got := formatRelativeTimeSince(ts, now)
	if got != "1m ago" {
		t.Fatalf("expected '1m ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_Minutes(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-15 * time.Minute)
	got := formatRelativeTimeSince(ts, now)
	if got != "15m ago" {
		t.Fatalf("expected '15m ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_OneHour(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-90 * time.Minute)
	got := formatRelativeTimeSince(ts, now)
	if got != "1h ago" {
		t.Fatalf("expected '1h ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_Hours(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-5 * time.Hour)
	got := formatRelativeTimeSince(ts, now)
	if got != "5h ago" {
		t.Fatalf("expected '5h ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_OneDay(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-36 * time.Hour)
	got := formatRelativeTimeSince(ts, now)
	if got != "1d ago" {
		t.Fatalf("expected '1d ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_Days(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ts := now.Add(-72 * time.Hour)
	got := formatRelativeTimeSince(ts, now)
	if got != "3d ago" {
		t.Fatalf("expected '3d ago', got %q", got)
	}
}

func TestFormatRelativeTimeSince_Zero(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	got := formatRelativeTimeSince(time.Time{}, now)
	if got != "" {
		t.Fatalf("expected empty string for zero time, got %q", got)
	}
}

func TestRenderTimestamp_NonZero(t *testing.T) {
	ts := time.Now().Add(-2 * time.Minute)
	got := renderTimestamp(ts)
	if got == "" {
		t.Fatal("expected non-empty rendered timestamp")
	}
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "ago") {
		t.Fatalf("expected 'ago' in rendered timestamp, got %q", stripped)
	}
}

func TestRenderTimestamp_Zero(t *testing.T) {
	got := renderTimestamp(time.Time{})
	if got != "" {
		t.Fatalf("expected empty string for zero timestamp, got %q", got)
	}
}
