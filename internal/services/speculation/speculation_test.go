package speculation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/models"
)

// mockCaller is a minimal Caller that returns a canned assistant message.
type mockCaller struct {
	delay    time.Duration
	response string
}

func (m *mockCaller) Stream(ctx context.Context, params api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 1)
	go func() {
		defer close(ch)
		if m.delay > 0 {
			select {
			case <-time.After(m.delay):
			case <-ctx.Done():
				ch <- api.StreamEvent{Type: "error", Err: ctx.Err()}
				return
			}
		}
		text := m.response
		if text == "" {
			text = "Done."
		}
		msg := &models.Message{
			ID:         "msg-test",
			Role:       models.RoleAssistant,
			Content:    []models.Block{{Type: models.BlockText, Text: text}},
			StopReason: models.StopEndTurn,
			Usage:      &models.Usage{InputTokens: 100, OutputTokens: 50},
			Timestamp:  time.Now(),
		}
		ch <- api.StreamEvent{Type: "message_done", Message: msg}
	}()
	return ch
}

// --- Tests ---

func TestNewSpeculator(t *testing.T) {
	s := NewSpeculator(Config{
		Model: "claude-sonnet-4-6",
	}, &mockCaller{})

	if s == nil {
		t.Fatal("NewSpeculator returned nil")
	}
	if len(s.active) != 0 {
		t.Fatalf("expected empty active map, got %d entries", len(s.active))
	}
}

func TestSpeculateLaunchesBackground(t *testing.T) {
	caller := &mockCaller{delay: 50 * time.Millisecond, response: "speculated result"}
	s := NewSpeculator(Config{
		Model:    "claude-sonnet-4-6",
		MaxTurns: 5,
	}, caller)

	id, err := s.Speculate(context.Background(), "run tests")
	if err != nil {
		t.Fatalf("Speculate error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty speculation ID")
	}

	// Should be in active map with pending or running status.
	s.mu.Lock()
	spec, ok := s.active[id]
	var status SpecStatus
	if ok {
		status = spec.Status
	}
	s.mu.Unlock()
	if !ok {
		t.Fatal("speculation not found in active map")
	}
	if status != StatusPending && status != StatusRunning {
		t.Fatalf("expected pending or running, got %s", status)
	}

	// Wait for completion
	waitForStatus(t, s, id, StatusCompleted, 3*time.Second)

	s.mu.Lock()
	result := s.active[id].Result
	s.mu.Unlock()
	if result == nil {
		t.Fatal("expected non-nil result after completion")
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message in result")
	}
}

func TestAcceptAppliesFileDiffs(t *testing.T) {
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6"}, &mockCaller{})

	// Create a temp dir for the test workspace
	tmpDir := t.TempDir()

	// Manually inject a completed speculation with file diffs
	specID := "test-accept"
	targetFile := filepath.Join(tmpDir, "hello.txt")

	s.mu.Lock()
	s.active[specID] = &Speculation{
		ID:     specID,
		Prompt: "write hello",
		Status: StatusCompleted,
		Result: &SpeculationResult{
			Messages: []*models.Message{
				{
					ID:      "msg-1",
					Role:    models.RoleAssistant,
					Content: []models.Block{{Type: models.BlockText, Text: "Created hello.txt"}},
				},
			},
			FileDiffs: []FileDiff{
				{Path: targetFile, Before: "", After: "hello world\n"},
			},
			TotalUsage: models.Usage{InputTokens: 50, OutputTokens: 20},
		},
		WorkDir: t.TempDir(), // separate workdir that should be cleaned up
	}
	s.mu.Unlock()

	result, err := s.Accept(specID)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}

	// File should have been written
	content, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Fatalf("unexpected file content: %q", string(content))
	}

	// Speculation should be removed from active map
	s.mu.Lock()
	_, exists := s.active[specID]
	s.mu.Unlock()
	if exists {
		t.Fatal("expected speculation to be removed from active map after accept")
	}
}

func TestRejectCleansUp(t *testing.T) {
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6"}, &mockCaller{})

	// Create temp workdir
	workDir := t.TempDir()
	markerFile := filepath.Join(workDir, "marker.txt")
	os.WriteFile(markerFile, []byte("temp"), 0644)

	specID := "test-reject"
	s.mu.Lock()
	s.active[specID] = &Speculation{
		ID:      specID,
		Prompt:  "something",
		Status:  StatusCompleted,
		WorkDir: workDir,
		Result: &SpeculationResult{
			Messages: []*models.Message{
				{ID: "msg-1", Role: models.RoleAssistant, Content: []models.Block{{Type: models.BlockText, Text: "done"}}},
			},
		},
	}
	s.mu.Unlock()

	err := s.Reject(specID)
	if err != nil {
		t.Fatalf("Reject error: %v", err)
	}

	// Should be removed from active map
	s.mu.Lock()
	_, exists := s.active[specID]
	s.mu.Unlock()
	if exists {
		t.Fatal("expected speculation removed after reject")
	}
}

func TestCancelStopsInflight(t *testing.T) {
	// Use a slow caller so the speculation is still running when we cancel
	caller := &mockCaller{delay: 5 * time.Second}
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6", MaxTurns: 5}, caller)

	id, err := s.Speculate(context.Background(), "slow task")
	if err != nil {
		t.Fatalf("Speculate error: %v", err)
	}

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	s.Cancel()

	// Wait a bit for the cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	s.mu.Lock()
	spec, ok := s.active[id]
	status := StatusCancelled
	if ok {
		status = spec.Status
	}
	s.mu.Unlock()

	if ok && status != StatusCancelled && status != StatusFailed {
		t.Fatalf("expected cancelled or failed status after Cancel, got %s", status)
	}
}

func TestConcurrentAccess(t *testing.T) {
	caller := &mockCaller{delay: 10 * time.Millisecond, response: "ok"}
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6", MaxTurns: 3}, caller)

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	// Launch 10 speculations concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Speculate(context.Background(), "task")
			if err != nil {
				errs <- err
			}
		}(i)
	}

	// Concurrently cancel
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(30 * time.Millisecond)
		s.Cancel()
	}()

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
}

func TestSuggestReturnsFollowUps(t *testing.T) {
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6"}, &mockCaller{})

	tests := []struct {
		name     string
		messages []*models.Message
		wantAny  []string // at least one of these should appear in suggestions
	}{
		{
			name: "after code edit suggests tests",
			messages: []*models.Message{
				{Role: models.RoleAssistant, Content: []models.Block{
					{Type: models.BlockToolUse, Name: "Edit"},
				}},
			},
			wantAny: []string{"test", "run"},
		},
		{
			name: "after test failure suggests fix",
			messages: []*models.Message{
				{Role: models.RoleUser, Content: []models.Block{
					{Type: models.BlockToolResult, Content: "FAIL: TestFoo", IsError: true},
				}},
			},
			wantAny: []string{"fix", "error"},
		},
		{
			name: "after write suggests commit",
			messages: []*models.Message{
				{Role: models.RoleAssistant, Content: []models.Block{
					{Type: models.BlockToolUse, Name: "Write"},
				}},
			},
			wantAny: []string{"commit", "test"},
		},
		{
			name:     "empty messages returns empty",
			messages: nil,
			wantAny:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := s.Suggest(tt.messages)
			if tt.wantAny == nil {
				if len(suggestions) != 0 {
					t.Fatalf("expected no suggestions, got %v", suggestions)
				}
				return
			}

			found := false
			for _, sug := range suggestions {
				lower := strings.ToLower(sug)
				for _, want := range tt.wantAny {
					if strings.Contains(lower, want) {
						found = true
						break
					}
				}
			}
			if !found {
				t.Fatalf("expected at least one suggestion containing %v, got %v", tt.wantAny, suggestions)
			}
		})
	}
}

func TestSpeculateNotFound(t *testing.T) {
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6"}, &mockCaller{})

	_, err := s.Accept("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent speculation")
	}

	err = s.Reject("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent speculation")
	}
}

func TestAcceptNonCompleted(t *testing.T) {
	s := NewSpeculator(Config{Model: "claude-sonnet-4-6"}, &mockCaller{})

	s.mu.Lock()
	s.active["running-one"] = &Speculation{
		ID:     "running-one",
		Prompt: "task",
		Status: StatusRunning,
	}
	s.mu.Unlock()

	_, err := s.Accept("running-one")
	if err == nil {
		t.Fatal("expected error when accepting non-completed speculation")
	}
}

// waitForStatus polls until the speculation reaches the expected status or times out.
func waitForStatus(t *testing.T, s *Speculator, id string, want SpecStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			s.mu.Lock()
			spec, ok := s.active[id]
			var got SpecStatus
			if ok {
				got = spec.Status
			}
			s.mu.Unlock()
			t.Fatalf("timed out waiting for status %s, current: %s", want, got)
		default:
			s.mu.Lock()
			spec, ok := s.active[id]
			var got SpecStatus
			if ok {
				got = spec.Status
			}
			s.mu.Unlock()
			if got == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
