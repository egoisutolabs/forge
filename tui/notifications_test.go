package tui

import (
	"testing"
	"time"
)

func TestNotifQueue_Push_And_Current(t *testing.T) {
	q := NewNotifQueue()

	q.Push(Notification{Key: "a", Message: "hello", Priority: NotifLow})
	if q.Len() != 1 {
		t.Fatalf("expected len 1, got %d", q.Len())
	}
	cur := q.Current()
	if cur == nil || cur.Message != "hello" {
		t.Fatalf("expected current='hello', got %v", cur)
	}
}

func TestNotifQueue_PriorityOrdering(t *testing.T) {
	q := NewNotifQueue()

	q.Push(Notification{Key: "low", Message: "low", Priority: NotifLow})
	q.Push(Notification{Key: "high", Message: "high", Priority: NotifHigh})
	q.Push(Notification{Key: "imm", Message: "imm", Priority: NotifImmediate})

	all := q.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	if all[0].Key != "imm" {
		t.Fatalf("expected first=imm, got %s", all[0].Key)
	}
	if all[1].Key != "high" {
		t.Fatalf("expected second=high, got %s", all[1].Key)
	}
	if all[2].Key != "low" {
		t.Fatalf("expected third=low, got %s", all[2].Key)
	}
}

func TestNotifQueue_DedupByKey(t *testing.T) {
	q := NewNotifQueue()

	q.Push(Notification{Key: "a", Message: "first", Priority: NotifLow})
	q.Push(Notification{Key: "a", Message: "second", Priority: NotifLow})

	if q.Len() != 1 {
		t.Fatalf("expected 1 item after dedup, got %d", q.Len())
	}
	cur := q.Current()
	if cur.Message != "second" {
		t.Fatalf("expected updated message='second', got %q", cur.Message)
	}
}

func TestNotifQueue_Dismiss(t *testing.T) {
	q := NewNotifQueue()

	q.Push(Notification{Key: "a", Message: "hello", Priority: NotifLow})
	q.Push(Notification{Key: "b", Message: "world", Priority: NotifLow})

	if !q.Dismiss("a") {
		t.Fatal("expected dismiss to return true")
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 item after dismiss, got %d", q.Len())
	}
	if q.Current().Key != "b" {
		t.Fatalf("expected current=b after dismiss, got %s", q.Current().Key)
	}

	if q.Dismiss("nonexistent") {
		t.Fatal("expected dismiss of nonexistent key to return false")
	}
}

func TestNotifQueue_AutoDismiss(t *testing.T) {
	q := NewNotifQueue()

	past := time.Now().Add(-15 * time.Second)
	q.Push(Notification{
		Key:      "old",
		Message:  "old",
		Priority: NotifLow, // 10s timeout
		Created:  past,
	})
	q.Push(Notification{
		Key:      "new",
		Message:  "new",
		Priority: NotifHigh, // 30s timeout
		Created:  time.Now(),
	})

	q.Tick(time.Now())

	if q.Len() != 1 {
		t.Fatalf("expected 1 item after tick, got %d", q.Len())
	}
	if q.Current().Key != "new" {
		t.Fatalf("expected 'new' to survive, got %s", q.Current().Key)
	}
}

func TestNotifQueue_ImmediatePreempts(t *testing.T) {
	q := NewNotifQueue()

	q.Push(Notification{Key: "bg", Message: "bg", Priority: NotifMedium})
	q.Push(Notification{Key: "urgent", Message: "urgent", Priority: NotifImmediate})

	cur := q.Current()
	if cur.Key != "urgent" {
		t.Fatalf("expected immediate to preempt, got %s", cur.Key)
	}
}

func TestNotifQueue_ImmediateNeverExpires(t *testing.T) {
	q := NewNotifQueue()

	old := time.Now().Add(-10 * time.Minute)
	q.Push(Notification{
		Key:      "persistent",
		Message:  "persistent",
		Priority: NotifImmediate,
		Created:  old,
	})

	q.Tick(time.Now())

	if q.Len() != 1 {
		t.Fatal("expected immediate notification to survive tick")
	}
}

func TestNotifQueue_CustomTimeout(t *testing.T) {
	q := NewNotifQueue()

	past := time.Now().Add(-3 * time.Second)
	q.Push(Notification{
		Key:      "short",
		Message:  "short",
		Priority: NotifHigh, // default 30s
		Timeout:  2 * time.Second,
		Created:  past,
	})

	q.Tick(time.Now())

	if q.Len() != 0 {
		t.Fatal("expected custom timeout to expire notification")
	}
}
