package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewAgentRegistry_Empty(t *testing.T) {
	r := NewAgentRegistry()
	if agents := r.List(); len(agents) != 0 {
		t.Errorf("new registry should be empty, got %d agents", len(agents))
	}
}

func TestAgentRegistry_RegisterAndGet(t *testing.T) {
	r := NewAgentRegistry()

	ba := &BackgroundAgent{
		AgentID:     "test-abc-123",
		Description: "test task",
		Status:      AgentStatusRunning,
		OutputFile:  "/tmp/test.output",
		StartedAt:   time.Now(),
	}
	r.Register(ba)

	got := r.Get("test-abc-123")
	if got == nil {
		t.Fatal("Register then Get should return the agent")
	}
	if got.AgentID != "test-abc-123" {
		t.Errorf("AgentID = %q, want test-abc-123", got.AgentID)
	}
	if got.Status != AgentStatusRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
}

func TestAgentRegistry_GetUnknown(t *testing.T) {
	r := NewAgentRegistry()
	got := r.Get("no-such-id")
	if got != nil {
		t.Errorf("Get unknown ID should return nil, got %+v", got)
	}
}

func TestAgentRegistry_Complete(t *testing.T) {
	dir := t.TempDir()
	r := NewAgentRegistry()

	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:    "comp-1",
		Status:     AgentStatusRunning,
		OutputFile: outFile,
		StartedAt:  time.Now(),
	})

	r.Complete("comp-1", "the final answer")

	got := r.Get("comp-1")
	if got.Status != AgentStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Result != "the final answer" {
		t.Errorf("Result = %q, want %q", got.Result, "the final answer")
	}
	if got.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set after Complete")
	}

	// Output file should contain the result
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(data) != "the final answer" {
		t.Errorf("output file = %q, want %q", string(data), "the final answer")
	}
}

func TestAgentRegistry_Fail(t *testing.T) {
	dir := t.TempDir()
	r := NewAgentRegistry()

	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:    "fail-1",
		Status:     AgentStatusRunning,
		OutputFile: outFile,
		StartedAt:  time.Now(),
	})

	r.Fail("fail-1", "network timeout")

	got := r.Get("fail-1")
	if got.Status != AgentStatusFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Error != "network timeout" {
		t.Errorf("Error = %q, want %q", got.Error, "network timeout")
	}
	if got.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set after Fail")
	}

	// Output file should contain error marker
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(data) != "ERROR: network timeout" {
		t.Errorf("output file = %q", string(data))
	}
}

func TestAgentRegistry_CompleteUnknown(t *testing.T) {
	r := NewAgentRegistry()
	// Should not panic for unknown IDs
	r.Complete("unknown-id", "result")
	r.Fail("unknown-id", "error")
}

func TestAgentRegistry_List(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&BackgroundAgent{AgentID: "a1", Status: AgentStatusRunning})
	r.Register(&BackgroundAgent{AgentID: "a2", Status: AgentStatusCompleted})
	r.Register(&BackgroundAgent{AgentID: "a3", Status: AgentStatusFailed})

	agents := r.List()
	if len(agents) != 3 {
		t.Errorf("List() = %d agents, want 3", len(agents))
	}
}

func TestAgentRegistry_Concurrency(t *testing.T) {
	r := NewAgentRegistry()

	// Register agents concurrently — should not race
	done := make(chan struct{})
	for i := range 10 {
		go func(id string) {
			r.Register(&BackgroundAgent{
				AgentID:    id,
				OutputFile: "/tmp/" + id,
				StartedAt:  time.Now(),
			})
			done <- struct{}{}
		}(string(rune('a' + i)))
	}
	for range 10 {
		<-done
	}
	if len(r.List()) != 10 {
		t.Errorf("expected 10 agents after concurrent register, got %d", len(r.List()))
	}
}

// ── Output file permissions (SEC-8) ──────────────────────────────────────────

func TestAgentRegistry_Complete_OutputFile_0600(t *testing.T) {
	dir := t.TempDir()
	r := NewAgentRegistry()
	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{AgentID: "perm-1", Status: AgentStatusRunning, OutputFile: outFile, StartedAt: time.Now()})
	r.Complete("perm-1", "result")

	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("Complete output file mode = %04o, want 0600", mode)
	}
}

func TestAgentRegistry_Fail_OutputFile_0600(t *testing.T) {
	dir := t.TempDir()
	r := NewAgentRegistry()
	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{AgentID: "perm-2", Status: AgentStatusRunning, OutputFile: outFile, StartedAt: time.Now()})
	r.Fail("perm-2", "some error")

	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("stat output file: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("Fail output file mode = %04o, want 0600", mode)
	}
}

// ── Notification channel tests ──────────────────────────────────────────────

func TestAgentRegistry_Complete_SendsNotification(t *testing.T) {
	dir := t.TempDir()
	ch := make(chan string, 8)
	r := NewAgentRegistry()
	r.NotifyCh = ch

	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:     "notif-1",
		Description: "search task",
		Status:      AgentStatusRunning,
		OutputFile:  outFile,
		StartedAt:   time.Now(),
	})
	r.Complete("notif-1", "found the answer")

	select {
	case msg := <-ch:
		if !strings.Contains(msg, "task-notification") {
			t.Errorf("notification missing <task-notification> tag: %s", msg)
		}
		if !strings.Contains(msg, "search task") {
			t.Errorf("notification missing description: %s", msg)
		}
		if !strings.Contains(msg, "completed") {
			t.Errorf("notification missing 'completed': %s", msg)
		}
		if !strings.Contains(msg, "found the answer") {
			t.Errorf("notification missing result: %s", msg)
		}
	default:
		t.Error("expected notification on channel after Complete")
	}
}

func TestAgentRegistry_Fail_SendsNotification(t *testing.T) {
	dir := t.TempDir()
	ch := make(chan string, 8)
	r := NewAgentRegistry()
	r.NotifyCh = ch

	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:     "notif-2",
		Description: "build task",
		Status:      AgentStatusRunning,
		OutputFile:  outFile,
		StartedAt:   time.Now(),
	})
	r.Fail("notif-2", "compile error")

	select {
	case msg := <-ch:
		if !strings.Contains(msg, "task-notification") {
			t.Errorf("notification missing <task-notification> tag: %s", msg)
		}
		if !strings.Contains(msg, "build task") {
			t.Errorf("notification missing description: %s", msg)
		}
		if !strings.Contains(msg, "failed") {
			t.Errorf("notification missing 'failed': %s", msg)
		}
		if !strings.Contains(msg, "compile error") {
			t.Errorf("notification missing error message: %s", msg)
		}
	default:
		t.Error("expected notification on channel after Fail")
	}
}

func TestAgentRegistry_NoNotifyCh_NoPanic(t *testing.T) {
	dir := t.TempDir()
	r := NewAgentRegistry()
	// NotifyCh is nil — should not panic
	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:    "notif-3",
		Status:     AgentStatusRunning,
		OutputFile: outFile,
		StartedAt:  time.Now(),
	})
	r.Complete("notif-3", "result") // should not panic
}

func TestAgentRegistry_NotifyCh_FullChannel_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	ch := make(chan string, 1) // buffer size 1
	r := NewAgentRegistry()
	r.NotifyCh = ch

	// Fill the channel
	ch <- "blocking message"

	outFile := filepath.Join(dir, "agent.output")
	r.Register(&BackgroundAgent{
		AgentID:    "notif-4",
		Status:     AgentStatusRunning,
		OutputFile: outFile,
		StartedAt:  time.Now(),
	})

	// This should not block even though channel is full
	done := make(chan struct{})
	go func() {
		r.Complete("notif-4", "result")
		close(done)
	}()

	select {
	case <-done:
		// OK — non-blocking
	case <-time.After(2 * time.Second):
		t.Fatal("Complete blocked on full notification channel")
	}
}

func TestAgentStatusConstants(t *testing.T) {
	if AgentStatusRunning != "running" {
		t.Errorf("AgentStatusRunning = %q", AgentStatusRunning)
	}
	if AgentStatusCompleted != "completed" {
		t.Errorf("AgentStatusCompleted = %q", AgentStatusCompleted)
	}
	if AgentStatusFailed != "failed" {
		t.Errorf("AgentStatusFailed = %q", AgentStatusFailed)
	}
}
