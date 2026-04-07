package tui

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ClockTickMsg is sent on each animation frame from the central clock.
// All subscribers see the same time value per frame.
type ClockTickMsg time.Time

// AnimClock is a central animation clock that all animations subscribe to.
// It ticks at 60fps (16ms) normally and 30fps (32ms) when the terminal is blurred.
// Auto-starts when the first subscriber joins, auto-stops when the last leaves.
type AnimClock struct {
	mu          sync.Mutex
	subscribers map[int]chan time.Time
	nextID      int
	ticker      *time.Ticker
	running     bool
	interval    time.Duration
	blurred     bool
	stopCh      chan struct{}
}

const (
	normalInterval  = time.Second / 60 // ~16ms, 60fps
	blurredInterval = time.Second / 30 // ~32ms, 30fps
)

// NewAnimClock creates a new animation clock (not yet running).
func NewAnimClock() *AnimClock {
	return &AnimClock{
		subscribers: make(map[int]chan time.Time),
		interval:    normalInterval,
	}
}

// Subscribe adds a new subscriber and returns an ID and a receive-only channel.
// The clock auto-starts if this is the first subscriber.
func (c *AnimClock) Subscribe() (id int, ch <-chan time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id = c.nextID
	c.nextID++
	// Buffered channel so the clock goroutine never blocks on a slow subscriber.
	sub := make(chan time.Time, 1)
	c.subscribers[id] = sub

	if !c.running {
		c.start()
	}

	return id, sub
}

// Unsubscribe removes a subscriber. Auto-stops the clock if no subscribers remain.
func (c *AnimClock) Unsubscribe(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.subscribers[id]; ok {
		close(ch)
		delete(c.subscribers, id)
	}

	if len(c.subscribers) == 0 && c.running {
		c.stop()
	}
}

// SetBlurred adjusts the tick interval. When blurred, the clock ticks at
// 30fps (32ms) instead of 60fps (16ms) to save CPU.
func (c *AnimClock) SetBlurred(blurred bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.blurred == blurred {
		return
	}
	c.blurred = blurred

	if blurred {
		c.interval = blurredInterval
	} else {
		c.interval = normalInterval
	}

	// Restart ticker with new interval if running.
	if c.running {
		c.stop()
		c.start()
	}
}

// Blurred returns whether the clock is in blurred (low-power) mode.
func (c *AnimClock) Blurred() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blurred
}

// Running returns whether the clock goroutine is active.
func (c *AnimClock) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// SubscriberCount returns the current number of subscribers.
func (c *AnimClock) SubscriberCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.subscribers)
}

// Interval returns the current tick interval.
func (c *AnimClock) Interval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interval
}

// start begins the background ticker goroutine. Must be called with mu held.
func (c *AnimClock) start() {
	c.ticker = time.NewTicker(c.interval)
	c.stopCh = make(chan struct{})
	c.running = true

	// Capture stopCh and ticker locally so the goroutine doesn't race
	// with a subsequent stop()+start() cycle that overwrites these fields.
	stopCh := c.stopCh
	ticker := c.ticker

	go func() {
		for {
			select {
			case <-stopCh:
				return
			case t := <-ticker.C:
				c.mu.Lock()
				for _, ch := range c.subscribers {
					// Non-blocking send: drop frame if subscriber is behind.
					select {
					case ch <- t:
					default:
					}
				}
				c.mu.Unlock()
			}
		}
	}()
}

// stop halts the background ticker goroutine. Must be called with mu held.
func (c *AnimClock) stop() {
	if c.ticker != nil {
		c.ticker.Stop()
	}
	if c.stopCh != nil {
		close(c.stopCh)
	}
	c.running = false
}

// Stop stops the clock and unsubscribes all subscribers. Call on shutdown.
func (c *AnimClock) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		c.stop()
	}
	// Take ownership of the map and replace it to avoid closing channels
	// that may have already been closed by a concurrent Unsubscribe call.
	subs := c.subscribers
	c.subscribers = make(map[int]chan time.Time)
	for _, ch := range subs {
		close(ch)
	}
}

// listenForClockTicks returns a tea.Cmd that waits for the next clock tick
// and converts it to a ClockTickMsg for the Bubbletea Update loop.
func listenForClockTicks(ch <-chan time.Time) tea.Cmd {
	return func() tea.Msg {
		t, ok := <-ch
		if !ok {
			return nil
		}
		return ClockTickMsg(t)
	}
}
