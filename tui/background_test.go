package tui

import (
	"sort"
	"testing"
	"time"
)

// --- BackgroundState basic lifecycle ---

func TestBackgroundState_InitialState(t *testing.T) {
	bs := NewBackgroundState()
	if bs.IsBackgrounded() {
		t.Fatal("expected not backgrounded initially")
	}
	if bs.AgentCount() != 0 {
		t.Fatalf("expected 0 agents, got %d", bs.AgentCount())
	}
}

func TestBackgroundState_BackgroundAndForeground(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("test-agent")

	if !bs.IsBackgrounded() {
		t.Fatal("expected backgrounded after Background()")
	}
	if bs.AgentCount() != 1 {
		t.Fatalf("expected 1 agent, got %d", bs.AgentCount())
	}

	bs.Foreground()
	if bs.IsBackgrounded() {
		t.Fatal("expected not backgrounded after Foreground()")
	}
	// Agent should still be tracked
	if bs.AgentCount() != 1 {
		t.Fatalf("expected 1 agent still tracked, got %d", bs.AgentCount())
	}
}

func TestBackgroundState_ProcessingStartResets(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.OnProcessingStart()

	if bs.IsBackgrounded() {
		t.Fatal("expected not backgrounded after OnProcessingStart()")
	}
}

func TestBackgroundState_ProcessingDoneResets(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.OnProcessingDone()

	if bs.IsBackgrounded() {
		t.Fatal("expected not backgrounded after OnProcessingDone()")
	}
}

// --- Auto-background timeout ---

func TestBackgroundState_AutoBackground_NotTriggeredEarly(t *testing.T) {
	bs := NewBackgroundState()
	bs.OnProcessingStart()

	// 30 seconds later — should not trigger
	now := time.Now().Add(30 * time.Second)
	if bs.CheckAutoBackground(now) {
		t.Fatal("expected no auto-background at 30s")
	}
}

func TestBackgroundState_AutoBackground_TriggeredAfterTimeout(t *testing.T) {
	bs := NewBackgroundState()
	bs.OnProcessingStart()

	// 61 seconds later — should trigger
	now := time.Now().Add(61 * time.Second)
	if !bs.CheckAutoBackground(now) {
		t.Fatal("expected auto-background at 61s")
	}
}

func TestBackgroundState_AutoBackground_ResetByInteraction(t *testing.T) {
	bs := NewBackgroundState()
	bs.OnProcessingStart()

	// 50 seconds in, user interacts
	bs.RecordInteraction()

	// 61 seconds from start (but only 11s from interaction)
	now := time.Now().Add(61 * time.Second)
	// Need to set lastInteraction precisely — 11 seconds before "now", not before real time.Now()
	bs.lastInteraction = now.Add(-11 * time.Second)
	if bs.CheckAutoBackground(now) {
		t.Fatal("expected no auto-background — recent interaction")
	}
}

func TestBackgroundState_AutoBackground_CustomTimeout(t *testing.T) {
	bs := NewBackgroundState()
	bs.AutoTimeout = 10 * time.Second
	bs.OnProcessingStart()

	// 11 seconds later — should trigger with custom timeout
	now := time.Now().Add(11 * time.Second)
	if !bs.CheckAutoBackground(now) {
		t.Fatal("expected auto-background at 11s with 10s timeout")
	}
}

func TestBackgroundState_AutoBackground_NotTriggeredWhenAlreadyBackgrounded(t *testing.T) {
	bs := NewBackgroundState()
	bs.OnProcessingStart()
	bs.Background("agent1")

	now := time.Now().Add(120 * time.Second)
	if bs.CheckAutoBackground(now) {
		t.Fatal("expected no auto-background when already backgrounded")
	}
}

// --- Stall watchdog ---

func TestBackgroundState_StallDetection_NoStallEarly(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")

	// Output just grew
	bs.UpdateOutput("agent1", 100, "some output")

	now := time.Now().Add(10 * time.Second)
	stalled := bs.CheckStalls(now)
	if len(stalled) != 0 {
		t.Fatalf("expected no stalls at 10s, got %v", stalled)
	}
}

func TestBackgroundState_StallDetection_StalledWithInteractivePattern(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:       "agent1",
		StartTime:  now,
		OutputLen:  100,
		LastGrowth: now.Add(-50 * time.Second), // 50s since last growth
		LastOutput: "Do you want to continue? (y/n)",
	}

	stalled := bs.CheckStalls(now)
	if len(stalled) != 1 || stalled[0] != "agent1" {
		t.Fatalf("expected agent1 to be stalled, got %v", stalled)
	}
}

func TestBackgroundState_StallDetection_NotStalledWithoutInteractivePattern(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:       "agent1",
		StartTime:  now,
		OutputLen:  100,
		LastGrowth: now.Add(-50 * time.Second),
		LastOutput: "Processing files...", // no interactive pattern
	}

	stalled := bs.CheckStalls(now)
	if len(stalled) != 0 {
		t.Fatalf("expected no stalls without interactive pattern, got %v", stalled)
	}
}

func TestBackgroundState_StallDetection_OneShotNotification(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:       "agent1",
		StartTime:  now,
		OutputLen:  100,
		LastGrowth: now.Add(-50 * time.Second),
		LastOutput: "Continue? [Y/n]",
	}

	// First check — should fire
	stalled := bs.CheckStalls(now)
	if len(stalled) != 1 {
		t.Fatal("expected stall notification on first check")
	}

	// Second check — should not fire (one-shot)
	stalled = bs.CheckStalls(now)
	if len(stalled) != 0 {
		t.Fatal("expected no stall notification on second check (one-shot)")
	}
}

func TestBackgroundState_StallDetection_ResetOnNewOutput(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:          "agent1",
		StartTime:     now,
		OutputLen:     100,
		LastGrowth:    now.Add(-50 * time.Second),
		LastOutput:    "Continue? [Y/n]",
		StallNotified: true,
	}

	// New output arrives
	bs.UpdateOutput("agent1", 200, "More output...")

	// StallNotified should be reset
	agent := bs.Agents["agent1"]
	if agent.StallNotified {
		t.Fatal("expected StallNotified reset after new output")
	}
}

func TestBackgroundState_StallDetection_SkipsCompleted(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:       "agent1",
		StartTime:  now,
		OutputLen:  100,
		LastGrowth: now.Add(-50 * time.Second),
		LastOutput: "Continue? [Y/n]",
		Completed:  true,
	}

	stalled := bs.CheckStalls(now)
	if len(stalled) != 0 {
		t.Fatal("expected no stalls for completed agents")
	}
}

// --- Panel eviction ---

func TestBackgroundState_Eviction_NotEvictedDuringGrace(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:        "agent1",
		Completed:   true,
		CompletedAt: now.Add(-10 * time.Second), // 10s ago
	}

	evicted := bs.EvictCompleted(now)
	if len(evicted) != 0 {
		t.Fatal("expected no eviction during 30s grace period")
	}
	if _, exists := bs.Agents["agent1"]; !exists {
		t.Fatal("agent should still be tracked")
	}
}

func TestBackgroundState_Eviction_EvictedAfterGrace(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:        "agent1",
		Completed:   true,
		CompletedAt: now.Add(-31 * time.Second), // 31s ago — past grace
	}

	evicted := bs.EvictCompleted(now)
	if len(evicted) != 1 || evicted[0] != "agent1" {
		t.Fatalf("expected agent1 evicted, got %v", evicted)
	}
	if _, exists := bs.Agents["agent1"]; exists {
		t.Fatal("agent should be removed after eviction")
	}
}

func TestBackgroundState_Eviction_ViewingPreventsEviction(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:        "agent1",
		Completed:   true,
		CompletedAt: now.Add(-60 * time.Second), // well past grace
		Viewing:     true,                       // user is viewing
	}

	evicted := bs.EvictCompleted(now)
	if len(evicted) != 0 {
		t.Fatal("expected no eviction while user is viewing")
	}
	if _, exists := bs.Agents["agent1"]; !exists {
		t.Fatal("agent should still be tracked while viewing")
	}
}

func TestBackgroundState_Eviction_ViewingThenStopViewing(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["agent1"] = &BackgroundAgent{
		Name:        "agent1",
		Completed:   true,
		CompletedAt: now.Add(-60 * time.Second),
		Viewing:     true,
	}

	// First check: viewing — no eviction
	evicted := bs.EvictCompleted(now)
	if len(evicted) != 0 {
		t.Fatal("expected no eviction while viewing")
	}

	// User stops viewing
	bs.SetViewing("agent1", false)

	// Now should be evicted
	evicted = bs.EvictCompleted(now)
	if len(evicted) != 1 {
		t.Fatal("expected eviction after stop viewing")
	}
}

func TestBackgroundState_Eviction_OnlyCompletedAgents(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["active"] = &BackgroundAgent{
		Name:      "active",
		Completed: false, // still running
	}
	bs.Agents["done"] = &BackgroundAgent{
		Name:        "done",
		Completed:   true,
		CompletedAt: now.Add(-60 * time.Second),
	}

	evicted := bs.EvictCompleted(now)
	if len(evicted) != 1 || evicted[0] != "done" {
		t.Fatalf("expected only 'done' evicted, got %v", evicted)
	}
	if _, exists := bs.Agents["active"]; !exists {
		t.Fatal("active agent should still be tracked")
	}
}

func TestBackgroundState_Eviction_MultipleAgents(t *testing.T) {
	bs := NewBackgroundState()
	now := time.Now()
	bs.Agents["a"] = &BackgroundAgent{
		Name:        "a",
		Completed:   true,
		CompletedAt: now.Add(-60 * time.Second),
	}
	bs.Agents["b"] = &BackgroundAgent{
		Name:        "b",
		Completed:   true,
		CompletedAt: now.Add(-60 * time.Second),
	}
	bs.Agents["c"] = &BackgroundAgent{
		Name:        "c",
		Completed:   true,
		CompletedAt: now.Add(-10 * time.Second), // still in grace
	}

	evicted := bs.EvictCompleted(now)
	sort.Strings(evicted)
	if len(evicted) != 2 {
		t.Fatalf("expected 2 evicted, got %d", len(evicted))
	}
	if evicted[0] != "a" || evicted[1] != "b" {
		t.Fatalf("expected [a, b] evicted, got %v", evicted)
	}
}

// --- Interactive pattern matching ---

func TestInteractivePattern_Matches(t *testing.T) {
	cases := []struct {
		input string
		match bool
	}{
		{"Do you want to continue? (y/n)", true},
		{"Continue?", true},
		{"Proceed?", true},
		{"Confirm?", true},
		{"Are you sure you want to delete?", true},
		{"Press Enter to continue", true},
		{"Select option [Y]es or [N]o", true},
		{"Processing files...", false},
		{"Done!", false},
		{"Writing output to disk", false},
	}

	for _, tc := range cases {
		got := interactivePattern.MatchString(tc.input)
		if got != tc.match {
			t.Errorf("interactivePattern(%q) = %v, want %v", tc.input, got, tc.match)
		}
	}
}

// --- Agent lifecycle ---

func TestBackgroundState_MarkCompleted(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.MarkCompleted("agent1")

	agent := bs.Agents["agent1"]
	if !agent.Completed {
		t.Fatal("expected agent to be marked completed")
	}
	if agent.CompletedAt.IsZero() {
		t.Fatal("expected CompletedAt to be set")
	}
}

func TestBackgroundState_MarkCompleted_Nonexistent(t *testing.T) {
	bs := NewBackgroundState()
	// Should not panic
	bs.MarkCompleted("nonexistent")
}

func TestBackgroundState_Remove(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.Remove("agent1")

	if bs.AgentCount() != 0 {
		t.Fatal("expected 0 agents after remove")
	}
}

func TestBackgroundState_AllAgents(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("a")
	bs.Background("b")

	agents := bs.AllAgents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
}

func TestBackgroundState_AgentCount_ExcludesCompleted(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("a")
	bs.Background("b")
	bs.MarkCompleted("b")

	if bs.AgentCount() != 1 {
		t.Fatalf("expected 1 active agent, got %d", bs.AgentCount())
	}
}

func TestBackgroundState_DuplicateBackground(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.Background("agent1") // should not create duplicate

	if len(bs.Agents) != 1 {
		t.Fatalf("expected 1 agent after duplicate Background(), got %d", len(bs.Agents))
	}
}

func TestBackgroundState_SetViewing(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")

	bs.SetViewing("agent1", true)
	if !bs.Agents["agent1"].Viewing {
		t.Fatal("expected Viewing=true")
	}

	bs.SetViewing("agent1", false)
	if bs.Agents["agent1"].Viewing {
		t.Fatal("expected Viewing=false")
	}

	// Should not panic for nonexistent
	bs.SetViewing("nonexistent", true)
}

func TestBackgroundState_UpdateOutput_Nonexistent(t *testing.T) {
	bs := NewBackgroundState()
	// Should not panic
	bs.UpdateOutput("nonexistent", 100, "output")
}

func TestBackgroundState_UpdateOutput_NoGrowth(t *testing.T) {
	bs := NewBackgroundState()
	bs.Background("agent1")
	bs.UpdateOutput("agent1", 100, "output")

	oldGrowth := bs.Agents["agent1"].LastGrowth

	// Same length — should not update
	time.Sleep(time.Millisecond) // ensure time difference
	bs.UpdateOutput("agent1", 100, "output")

	if bs.Agents["agent1"].LastGrowth != oldGrowth {
		t.Fatal("expected LastGrowth unchanged when output didn't grow")
	}
}
