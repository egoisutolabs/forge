package tui

import "time"

// ThrottledValue prevents fast-cycling text from flickering by enforcing a
// minimum display duration before allowing a new value to appear.
// Use for spinner messages, tool status text, and other rapidly-changing UI text.
type ThrottledValue[T any] struct {
	current    T
	pending    *T
	lastShown  time.Time
	minDisplay time.Duration
}

// NewThrottledValue creates a ThrottledValue with the given minimum display
// duration and initial value.
func NewThrottledValue[T any](minDisplay time.Duration, initial T) *ThrottledValue[T] {
	return &ThrottledValue[T]{
		current:    initial,
		minDisplay: minDisplay,
		lastShown:  time.Now(),
	}
}

// Set attempts to update the displayed value. If the minimum display time
// has elapsed, the new value is shown immediately. Otherwise, it's stored
// as pending and will be activated on the next Tick().
// Returns the value that should actually be displayed right now.
func (tv *ThrottledValue[T]) Set(val T) T {
	now := time.Now()
	if now.Sub(tv.lastShown) >= tv.minDisplay {
		tv.current = val
		tv.pending = nil
		tv.lastShown = now
		return tv.current
	}
	// Store as pending — will activate on next Tick after minDisplay elapses.
	tv.pending = &val
	return tv.current
}

// Tick checks if a pending value should now be activated because the minimum
// display time has elapsed. Returns the current displayed value.
func (tv *ThrottledValue[T]) Tick() T {
	if tv.pending == nil {
		return tv.current
	}
	now := time.Now()
	if now.Sub(tv.lastShown) >= tv.minDisplay {
		tv.current = *tv.pending
		tv.pending = nil
		tv.lastShown = now
	}
	return tv.current
}

// Current returns the value currently being displayed.
func (tv *ThrottledValue[T]) Current() T {
	return tv.current
}

// HasPending returns true if there is a value waiting to be shown.
func (tv *ThrottledValue[T]) HasPending() bool {
	return tv.pending != nil
}

// Remaining returns the time remaining before a pending value can be shown.
// Returns 0 if there is no pending value or the min display time has elapsed.
func (tv *ThrottledValue[T]) Remaining() time.Duration {
	if tv.pending == nil {
		return 0
	}
	elapsed := time.Since(tv.lastShown)
	if elapsed >= tv.minDisplay {
		return 0
	}
	return tv.minDisplay - elapsed
}
