package tui

import "testing"

func TestSessionLoadState_EmptyMessages(t *testing.T) {
	sl := NewSessionLoadState(nil, 10)
	if !sl.Done() {
		t.Fatal("expected done for empty messages")
	}
	if sl.Progress() != 1.0 {
		t.Fatalf("expected progress=1.0, got %f", sl.Progress())
	}
	batch := sl.NextBatch()
	if batch != nil {
		t.Fatalf("expected nil batch, got %v", batch)
	}
}

func TestSessionLoadState_SingleBatch(t *testing.T) {
	msgs := makeTestMessages(5)
	sl := NewSessionLoadState(msgs, 10)

	batch := sl.NextBatch()
	if len(batch) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(batch))
	}
	if !sl.Done() {
		t.Fatal("expected done after loading all messages")
	}
	if sl.Progress() != 1.0 {
		t.Fatalf("expected progress=1.0, got %f", sl.Progress())
	}
}

func TestSessionLoadState_MultipleBatches(t *testing.T) {
	msgs := makeTestMessages(25)
	sl := NewSessionLoadState(msgs, 10)

	// First batch: 10 messages
	batch1 := sl.NextBatch()
	if len(batch1) != 10 {
		t.Fatalf("expected 10 in first batch, got %d", len(batch1))
	}
	if sl.Done() {
		t.Fatal("should not be done after first batch")
	}
	if sl.Loaded() != 10 {
		t.Fatalf("expected loaded=10, got %d", sl.Loaded())
	}

	// Second batch: 10 messages
	batch2 := sl.NextBatch()
	if len(batch2) != 10 {
		t.Fatalf("expected 10 in second batch, got %d", len(batch2))
	}
	if sl.Loaded() != 20 {
		t.Fatalf("expected loaded=20, got %d", sl.Loaded())
	}

	// Third batch: 5 remaining
	batch3 := sl.NextBatch()
	if len(batch3) != 5 {
		t.Fatalf("expected 5 in third batch, got %d", len(batch3))
	}
	if !sl.Done() {
		t.Fatal("expected done after loading all messages")
	}
	if sl.Total() != 25 {
		t.Fatalf("expected total=25, got %d", sl.Total())
	}
}

func TestSessionLoadState_Progress(t *testing.T) {
	msgs := makeTestMessages(20)
	sl := NewSessionLoadState(msgs, 10)

	if sl.Progress() != 0.0 {
		t.Fatalf("expected initial progress=0.0, got %f", sl.Progress())
	}

	sl.NextBatch() // load 10/20
	p := sl.Progress()
	if p != 0.5 {
		t.Fatalf("expected progress=0.5, got %f", p)
	}

	sl.NextBatch() // load 20/20
	if sl.Progress() != 1.0 {
		t.Fatalf("expected progress=1.0, got %f", sl.Progress())
	}
}

func TestSessionLoadState_NoBatchAfterDone(t *testing.T) {
	msgs := makeTestMessages(3)
	sl := NewSessionLoadState(msgs, 10)

	sl.NextBatch() // loads all 3
	batch := sl.NextBatch()
	if batch != nil {
		t.Fatalf("expected nil batch after done, got %v", batch)
	}
}

func TestSessionLoadState_BatchSizeClampedToOne(t *testing.T) {
	msgs := makeTestMessages(3)
	sl := NewSessionLoadState(msgs, 0) // should clamp to 10 (default)

	batch := sl.NextBatch()
	if len(batch) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(batch))
	}
}

func TestSessionLoadState_Elapsed(t *testing.T) {
	sl := NewSessionLoadState(makeTestMessages(1), 10)
	if sl.Elapsed() < 0 {
		t.Fatal("elapsed should be non-negative")
	}
}

// makeTestMessages creates n DisplayMessages for testing.
func makeTestMessages(n int) []DisplayMessage {
	msgs := make([]DisplayMessage, n)
	for i := range msgs {
		msgs[i] = DisplayMessage{
			Role:    "assistant",
			Content: "message",
		}
	}
	return msgs
}
