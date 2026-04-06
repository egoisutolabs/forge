package tui

import "testing"

func TestScrollCoalescer_AccumulateAndFlush(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(1)
	sc.Accumulate(1)
	sc.Accumulate(1)

	delta := sc.Flush()
	if delta != 3 {
		t.Fatalf("expected delta=3, got %d", delta)
	}
}

func TestScrollCoalescer_IgnoresSmallDelta(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(1)
	sc.Accumulate(1) // total = 2, below threshold

	delta := sc.Flush()
	if delta != 0 {
		t.Fatalf("expected delta=0 (below threshold), got %d", delta)
	}
}

func TestScrollCoalescer_NegativeDelta(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(-2)
	sc.Accumulate(-2) // total = -4, abs >= 3

	delta := sc.Flush()
	if delta != -4 {
		t.Fatalf("expected delta=-4, got %d", delta)
	}
}

func TestScrollCoalescer_FlushResets(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(5)
	sc.Flush()

	// After flush, accumulator should be zero
	delta := sc.Flush()
	if delta != 0 {
		t.Fatalf("expected delta=0 after second flush, got %d", delta)
	}
}

func TestScrollCoalescer_Reset(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(10)
	sc.Reset()

	if sc.Pending() != 0 {
		t.Fatalf("expected pending=0 after reset, got %d", sc.Pending())
	}
}

func TestScrollCoalescer_Pending(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(2)
	if sc.Pending() != 2 {
		t.Fatalf("expected pending=2, got %d", sc.Pending())
	}

	sc.Accumulate(3)
	if sc.Pending() != 5 {
		t.Fatalf("expected pending=5, got %d", sc.Pending())
	}
}

func TestScrollCoalescer_MixedDirections(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(5)
	sc.Accumulate(-2) // net = 3

	delta := sc.Flush()
	if delta != 3 {
		t.Fatalf("expected delta=3, got %d", delta)
	}
}

func TestScrollCoalescer_MixedDirectionsBelowThreshold(t *testing.T) {
	sc := NewScrollCoalescer(3)

	sc.Accumulate(5)
	sc.Accumulate(-4) // net = 1, below threshold

	delta := sc.Flush()
	if delta != 0 {
		t.Fatalf("expected delta=0, got %d", delta)
	}
}

func TestScrollCoalescer_MinDeltaClampedToOne(t *testing.T) {
	sc := NewScrollCoalescer(0) // should clamp to 1

	sc.Accumulate(1)
	delta := sc.Flush()
	if delta != 1 {
		t.Fatalf("expected delta=1 with minDelta clamped to 1, got %d", delta)
	}
}
