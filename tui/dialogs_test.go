package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---- DialogQueue tests ----

func TestDialogQueue_PushActivatesFirst(t *testing.T) {
	dq := NewDialogQueue()

	dq.Push(QueuedDialog{
		Priority: DialogNotification,
		ID:       "notif-1",
	})

	if !dq.HasActive() {
		t.Fatal("expected active dialog after first push")
	}
	if dq.ActiveID() != "notif-1" {
		t.Fatalf("expected active ID %q, got %q", "notif-1", dq.ActiveID())
	}
	if dq.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", dq.PendingCount())
	}
}

func TestDialogQueue_HigherPriorityPreempts(t *testing.T) {
	dq := NewDialogQueue()

	// Push low-priority dialog
	dq.Push(QueuedDialog{
		Priority: DialogNotification,
		ID:       "notif",
	})

	// Push high-priority dialog — should preempt
	dq.Push(QueuedDialog{
		Priority: DialogPermission,
		ID:       "perm",
	})

	if dq.ActiveID() != "perm" {
		t.Fatalf("expected active ID %q, got %q", "perm", dq.ActiveID())
	}
	if dq.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", dq.PendingCount())
	}
}

func TestDialogQueue_LowerPriorityQueued(t *testing.T) {
	dq := NewDialogQueue()

	// Push high-priority dialog
	dq.Push(QueuedDialog{
		Priority: DialogPermission,
		ID:       "perm",
	})

	// Push low-priority dialog — should be queued
	dq.Push(QueuedDialog{
		Priority: DialogNotification,
		ID:       "notif",
	})

	if dq.ActiveID() != "perm" {
		t.Fatalf("expected active ID %q, got %q", "perm", dq.ActiveID())
	}
	if dq.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", dq.PendingCount())
	}
}

func TestDialogQueue_CloseDrainsInPriorityOrder(t *testing.T) {
	dq := NewDialogQueue()

	// Push highest priority first
	dq.Push(QueuedDialog{Priority: DialogPermission, ID: "perm"})
	// Queue lower priority
	dq.Push(QueuedDialog{Priority: DialogCostWarning, ID: "cost"})
	dq.Push(QueuedDialog{Priority: DialogNotification, ID: "notif"})
	dq.Push(QueuedDialog{Priority: DialogAskUser, ID: "ask"})

	// Active should be perm (highest priority)
	if dq.ActiveID() != "perm" {
		t.Fatalf("expected perm active, got %q", dq.ActiveID())
	}

	// Close perm — ask should become active (next highest)
	dq.Close()
	if dq.ActiveID() != "ask" {
		t.Fatalf("expected ask after close, got %q", dq.ActiveID())
	}

	// Close ask — cost should become active
	dq.Close()
	if dq.ActiveID() != "cost" {
		t.Fatalf("expected cost after close, got %q", dq.ActiveID())
	}

	// Close cost — notif should become active
	dq.Close()
	if dq.ActiveID() != "notif" {
		t.Fatalf("expected notif after close, got %q", dq.ActiveID())
	}

	// Close notif — queue should be empty
	dq.Close()
	if dq.HasActive() {
		t.Fatal("expected no active dialog after draining queue")
	}
}

func TestDialogQueue_CloseReturnsInitCmd(t *testing.T) {
	dq := NewDialogQueue()

	cmdCalled := false
	dq.Push(QueuedDialog{Priority: DialogPermission, ID: "perm"})
	dq.Push(QueuedDialog{
		Priority: DialogNotification,
		ID:       "notif",
		InitCmd: func() tea.Msg {
			cmdCalled = true
			return nil
		},
	})

	cmd := dq.Close()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Close")
	}
	// Execute the command
	cmd()
	if !cmdCalled {
		t.Fatal("expected InitCmd to be called")
	}
}

func TestDialogQueue_RemoveByID(t *testing.T) {
	dq := NewDialogQueue()

	dq.Push(QueuedDialog{Priority: DialogPermission, ID: "perm"})
	dq.Push(QueuedDialog{Priority: DialogNotification, ID: "notif"})
	dq.Push(QueuedDialog{Priority: DialogCostWarning, ID: "cost"})

	if dq.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", dq.PendingCount())
	}

	dq.RemoveByID("cost")
	if dq.PendingCount() != 1 {
		t.Fatalf("expected 1 pending after remove, got %d", dq.PendingCount())
	}
}

// ---- FooterNav tests ----

func TestFooterNav_BuildPillsFromAgents(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(3, StatusInfo{Model: "claude-sonnet-4-6"})

	// 3 agent pills + 1 session pill
	if len(fn.Pills) != 4 {
		t.Fatalf("expected 4 pills, got %d", len(fn.Pills))
	}

	for i := 0; i < 3; i++ {
		if fn.Pills[i].Type != "agent" {
			t.Fatalf("pill %d: expected type agent, got %q", i, fn.Pills[i].Type)
		}
		if !fn.Pills[i].Active {
			t.Fatalf("pill %d: expected active", i)
		}
	}

	if fn.Pills[3].Type != "session" {
		t.Fatalf("last pill: expected type session, got %q", fn.Pills[3].Type)
	}
}

func TestFooterNav_NavigationWraps(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(2, StatusInfo{Model: "test"})
	fn.Enter()

	if fn.Selected != 0 {
		t.Fatalf("expected selected=0, got %d", fn.Selected)
	}

	fn.Next()
	if fn.Selected != 1 {
		t.Fatalf("expected selected=1, got %d", fn.Selected)
	}

	fn.Next()
	fn.Next() // 1 -> 2 -> wraps to 0
	if fn.Selected != 0 {
		t.Fatalf("expected selected=0 after wrap, got %d", fn.Selected)
	}

	fn.Prev() // wraps back to 2
	if fn.Selected != 2 {
		t.Fatalf("expected selected=2, got %d", fn.Selected)
	}
}

func TestFooterNav_TypingExitsMode(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(1, StatusInfo{Model: "test"})
	fn.Enter()

	if !fn.Active() {
		t.Fatal("expected active after Enter")
	}

	// Simulate typing
	handled, _ := fn.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if handled {
		t.Fatal("typing should not be handled (pass through to input)")
	}
	if fn.Active() {
		t.Fatal("expected inactive after typing")
	}
}

func TestFooterNav_ArrowKeysNavigate(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(2, StatusInfo{Model: "test"})
	fn.Enter()

	handled, _ := fn.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("expected right arrow handled")
	}
	if fn.Selected != 1 {
		t.Fatalf("expected selected=1, got %d", fn.Selected)
	}

	handled, _ = fn.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("expected left arrow handled")
	}
	if fn.Selected != 0 {
		t.Fatalf("expected selected=0, got %d", fn.Selected)
	}
}

func TestFooterNav_EnterReturnsAction(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(1, StatusInfo{Model: "test"})
	fn.Enter()

	handled, action := fn.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("expected enter handled")
	}
	if action != "agent:agent-0" {
		t.Fatalf("expected action %q, got %q", "agent:agent-0", action)
	}
}

func TestFooterNav_EmptyPillsNoEnter(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(0, StatusInfo{})
	fn.Enter()

	if fn.Active() {
		t.Fatal("should not enter footer mode with no pills")
	}
}

func TestFooterNav_RenderPills(t *testing.T) {
	fn := NewFooterNav()
	fn.BuildPills(1, StatusInfo{Model: "claude-sonnet-4-6"})
	fn.Enter()

	theme := InitTheme()
	rendered := RenderFooterPills(fn, 80, theme)
	if rendered == "" {
		t.Fatal("expected non-empty rendered footer pills")
	}
}

// ---- CostThresholdState tests ----

func TestCostThreshold_TriggersOnCrossing(t *testing.T) {
	cs := NewCostThresholdState()

	// Below first threshold
	if t1 := cs.CheckThreshold(0.5); t1 != 0 {
		t.Fatalf("expected 0, got %f", t1)
	}

	// Cross $1 threshold
	if t1 := cs.CheckThreshold(1.0); t1 != 1.0 {
		t.Fatalf("expected 1.0, got %f", t1)
	}

	// Same threshold again — should not trigger
	if t1 := cs.CheckThreshold(1.5); t1 != 0 {
		t.Fatalf("expected 0 (already shown), got %f", t1)
	}

	// Cross $5 threshold
	if t1 := cs.CheckThreshold(5.5); t1 != 5.0 {
		t.Fatalf("expected 5.0, got %f", t1)
	}

	// Cross $10 threshold
	if t1 := cs.CheckThreshold(10.1); t1 != 10.0 {
		t.Fatalf("expected 10.0, got %f", t1)
	}

	// No more thresholds
	if t1 := cs.CheckThreshold(100.0); t1 != 0 {
		t.Fatalf("expected 0 (no more thresholds), got %f", t1)
	}
}

func TestCostThreshold_ShownOnlyOnce(t *testing.T) {
	cs := NewCostThresholdState()

	// Trigger $1
	cs.CheckThreshold(1.0)

	// Try again — should not trigger
	for i := 0; i < 5; i++ {
		if t1 := cs.CheckThreshold(1.0 + float64(i)*0.1); t1 != 0 {
			t.Fatalf("iteration %d: threshold %f should not re-trigger", i, t1)
		}
	}
}

func TestCostDialog_New(t *testing.T) {
	theme := InitTheme()
	cd := NewCostDialog(5.50, 5.0, theme)

	if cd == nil {
		t.Fatal("expected non-nil CostDialog")
	}
	if cd.costUSD != 5.50 {
		t.Fatalf("expected costUSD=5.50, got %f", cd.costUSD)
	}
	if cd.threshold != 5.0 {
		t.Fatalf("expected threshold=5.0, got %f", cd.threshold)
	}
	if cd.choice != "continue" {
		t.Fatalf("expected default choice %q, got %q", "continue", cd.choice)
	}
}

// ---- DialogPriority ordering tests ----

func TestDialogPriority_Ordering(t *testing.T) {
	if DialogPermission >= DialogAskUser {
		t.Fatal("Permission should have highest priority (lowest value)")
	}
	if DialogAskUser >= DialogCostWarning {
		t.Fatal("AskUser should be higher priority than CostWarning")
	}
	if DialogCostWarning >= DialogIdleReturn {
		t.Fatal("CostWarning should be higher priority than IdleReturn")
	}
	if DialogIdleReturn >= DialogNotification {
		t.Fatal("IdleReturn should be higher priority than Notification")
	}
}
