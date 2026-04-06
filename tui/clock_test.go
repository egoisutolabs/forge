package tui

import (
	"testing"
	"time"
)

func TestAnimClock_SubscribeAutoStarts(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	if c.Running() {
		t.Fatal("clock should not be running before any subscriber")
	}

	id, ch := c.Subscribe()
	defer c.Unsubscribe(id)

	if !c.Running() {
		t.Fatal("clock should auto-start on first subscriber")
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestAnimClock_UnsubscribeAutoStops(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	id, _ := c.Subscribe()
	if !c.Running() {
		t.Fatal("clock should be running after subscribe")
	}

	c.Unsubscribe(id)
	if c.Running() {
		t.Fatal("clock should auto-stop when last subscriber leaves")
	}
}

func TestAnimClock_SubscribersReceiveTicks(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	id1, ch1 := c.Subscribe()
	id2, ch2 := c.Subscribe()
	defer c.Unsubscribe(id1)
	defer c.Unsubscribe(id2)

	// Both subscribers should receive at least one tick within 100ms.
	timeout := time.After(200 * time.Millisecond)
	var t1, t2 time.Time
	var got1, got2 bool

	for !got1 || !got2 {
		select {
		case t1 = <-ch1:
			got1 = true
		case t2 = <-ch2:
			got2 = true
		case <-timeout:
			t.Fatalf("timed out waiting for ticks (got1=%v, got2=%v)", got1, got2)
		}
	}

	_ = t1
	_ = t2
}

func TestAnimClock_BlurredModeSlower(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	if c.Interval() != normalInterval {
		t.Fatalf("expected normal interval %v, got %v", normalInterval, c.Interval())
	}

	c.SetBlurred(true)
	if c.Interval() != blurredInterval {
		t.Fatalf("expected blurred interval %v, got %v", blurredInterval, c.Interval())
	}

	c.SetBlurred(false)
	if c.Interval() != normalInterval {
		t.Fatalf("expected normal interval after unblur %v, got %v", normalInterval, c.Interval())
	}
}

func TestAnimClock_BlurredRestartsTicker(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	id, ch := c.Subscribe()
	defer c.Unsubscribe(id)

	// Drain one tick to confirm running.
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected tick before blur")
	}

	c.SetBlurred(true)
	if !c.Running() {
		t.Fatal("clock should still be running after blur")
	}
	if !c.Blurred() {
		t.Fatal("expected blurred=true")
	}

	// Should still receive ticks at the slower rate.
	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected tick after blur switch")
	}
}

func TestAnimClock_MultipleSubscribers(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	id1, _ := c.Subscribe()
	id2, _ := c.Subscribe()
	id3, _ := c.Subscribe()

	if c.SubscriberCount() != 3 {
		t.Fatalf("expected 3 subscribers, got %d", c.SubscriberCount())
	}

	c.Unsubscribe(id1)
	if c.SubscriberCount() != 2 {
		t.Fatalf("expected 2 subscribers, got %d", c.SubscriberCount())
	}
	if !c.Running() {
		t.Fatal("clock should still be running with remaining subscribers")
	}

	c.Unsubscribe(id2)
	c.Unsubscribe(id3)
	if c.Running() {
		t.Fatal("clock should stop with no subscribers")
	}
}

func TestAnimClock_StopCleansUp(t *testing.T) {
	c := NewAnimClock()

	_, ch := c.Subscribe()
	c.Stop()

	if c.Running() {
		t.Fatal("clock should not be running after Stop")
	}
	if c.SubscriberCount() != 0 {
		t.Fatalf("expected 0 subscribers after Stop, got %d", c.SubscriberCount())
	}

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Fatal("expected subscriber channel to be closed after Stop")
	}
}

func TestAnimClock_SetBlurredIdempotent(t *testing.T) {
	c := NewAnimClock()
	defer c.Stop()

	c.SetBlurred(true)
	c.SetBlurred(true) // should be a no-op
	if c.Interval() != blurredInterval {
		t.Fatal("expected blurred interval to persist")
	}
}

func TestAnimClock_ListenForClockTicks(t *testing.T) {
	ch := make(chan time.Time, 1)
	now := time.Now()
	ch <- now

	cmd := listenForClockTicks(ch)
	msg := cmd()

	tick, ok := msg.(ClockTickMsg)
	if !ok {
		t.Fatalf("expected ClockTickMsg, got %T", msg)
	}
	if time.Time(tick) != now {
		t.Fatal("expected tick time to match sent time")
	}
}

func TestAnimClock_ListenForClockTicks_ClosedChannel(t *testing.T) {
	ch := make(chan time.Time)
	close(ch)

	cmd := listenForClockTicks(ch)
	msg := cmd()

	if msg != nil {
		t.Fatalf("expected nil msg from closed channel, got %T", msg)
	}
}
