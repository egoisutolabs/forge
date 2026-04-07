package tui

import (
	"testing"
	"time"
)

func TestSpinnerTips_PickTipOneShot(t *testing.T) {
	st := NewSpinnerTips()

	// First pick should set a tip
	st.PickTip(nil)
	if st.Current() == "" {
		t.Fatal("expected a tip after PickTip, got empty")
	}
	if !st.Picked() {
		t.Fatal("expected Picked() to be true after PickTip")
	}

	// Second pick in same turn should be no-op
	firstTip := st.Current()
	st.PickTip(nil)
	if st.Current() != firstTip {
		t.Fatalf("expected tip to remain %q, got %q (one-shot guard failed)", firstTip, st.Current())
	}
}

func TestSpinnerTips_ResetClearsOneShot(t *testing.T) {
	st := NewSpinnerTips()
	st.PickTip(nil)
	if !st.Picked() {
		t.Fatal("expected Picked()=true")
	}

	st.Reset()
	if st.Picked() {
		t.Fatal("expected Picked()=false after Reset")
	}
	if st.Current() != "" {
		t.Fatal("expected Current()=\"\" after Reset")
	}
}

func TestSpinnerTips_ContextAware(t *testing.T) {
	st := NewSpinnerTips()

	// Pick with a Bash tool active — should get bash-related tip
	tools := []ActiveToolInfo{
		{Name: "Bash", ID: "t1", StartTime: time.Now()},
	}
	st.PickTip(tools)

	tip := st.Current()
	expected := contextTips["Bash"]
	if tip != expected {
		t.Fatalf("expected context tip %q, got %q", expected, tip)
	}
}

func TestSpinnerTips_RotateIfDue(t *testing.T) {
	st := NewSpinnerTips()
	st.PickTip(nil)
	firstTip := st.Current()

	// Not due yet (just picked)
	changed := st.RotateIfDue(time.Now(), nil)
	if changed {
		t.Fatal("expected no rotation immediately after pick")
	}

	// Simulate time passing beyond rotation interval
	st.lastRotation = time.Now().Add(-tipRotateInterval - time.Second)
	changed = st.RotateIfDue(time.Now(), nil)
	if !changed {
		t.Fatal("expected rotation after interval elapsed")
	}

	// Tip should have changed (extremely unlikely to pick same one)
	// We test the mechanism, not the randomness, so just check it's not empty
	if st.Current() == "" {
		t.Fatal("expected non-empty tip after rotation")
	}
	_ = firstTip // avoid unused warning
}

func TestSpinnerTips_PoolExhaustion(t *testing.T) {
	st := NewSpinnerTips()

	// Pick more tips than the pool size — should reshuffle and continue
	seen := make(map[string]bool)
	for i := 0; i < len(tipPool)*2; i++ {
		st.Reset()
		st.PickTip(nil)
		tip := st.Current()
		if tip == "" {
			t.Fatalf("got empty tip on iteration %d", i)
		}
		seen[tip] = true
	}

	// Should have seen multiple different tips
	if len(seen) < 2 {
		t.Fatalf("expected variety in tips, only saw %d unique tips", len(seen))
	}
}

func TestRenderSpinnerTip(t *testing.T) {
	theme := ResolveTheme(DarkTheme())

	// Empty tip should render nothing
	result := renderSpinnerTip("", 80, theme)
	if result != "" {
		t.Fatalf("expected empty render for empty tip, got %q", result)
	}

	// Non-empty tip should include the tip text
	result = renderSpinnerTip("Try /compact", 80, theme)
	if result == "" {
		t.Fatal("expected non-empty render for non-empty tip")
	}
}
