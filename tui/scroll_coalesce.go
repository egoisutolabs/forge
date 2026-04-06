package tui

// ScrollCoalescer batches multiple scroll events within a single frame
// and ignores small deltas to reduce render commits during rapid scrolling.
//
// Usage: call Accumulate() for each scroll event. Call Flush() once per
// frame to get the coalesced delta and apply it. Reset() at frame boundaries.

// ScrollCoalescer batches scroll events to reduce viewport updates.
type ScrollCoalescer struct {
	accumulated int // total rows accumulated this frame
	minDelta    int // minimum absolute delta to trigger a scroll
}

// NewScrollCoalescer creates a coalescer that ignores deltas smaller
// than minDelta rows.
func NewScrollCoalescer(minDelta int) *ScrollCoalescer {
	if minDelta < 1 {
		minDelta = 1
	}
	return &ScrollCoalescer{
		minDelta: minDelta,
	}
}

// Accumulate adds a scroll delta (positive = down, negative = up).
func (sc *ScrollCoalescer) Accumulate(delta int) {
	sc.accumulated += delta
}

// Flush returns the coalesced delta if it meets the minimum threshold,
// and resets the accumulator. Returns 0 if the accumulated delta is
// below the threshold (i.e. the scroll is too small to act on).
func (sc *ScrollCoalescer) Flush() int {
	delta := sc.accumulated
	sc.accumulated = 0
	if abs(delta) < sc.minDelta {
		return 0
	}
	return delta
}

// Pending returns the currently accumulated delta without flushing.
func (sc *ScrollCoalescer) Pending() int {
	return sc.accumulated
}

// Reset discards any accumulated scroll delta.
func (sc *ScrollCoalescer) Reset() {
	sc.accumulated = 0
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
