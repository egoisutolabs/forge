package tools

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/models"
)

// --- Test helpers ---

// delayTool sleeps for a duration then returns a result. Tracks execution order.
type delayTool struct {
	name    string
	delay   time.Duration
	result  string
	safe    bool
	execLog *[]string // shared log to track execution order
	mu      *sync.Mutex
}

func (t *delayTool) Name() string                 { return t.name }
func (t *delayTool) Description() string          { return "delay tool" }
func (t *delayTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *delayTool) Execute(ctx context.Context, _ json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	select {
	case <-time.After(t.delay):
	case <-ctx.Done():
		return &models.ToolResult{Content: "cancelled", IsError: true}, nil
	}
	if t.execLog != nil && t.mu != nil {
		t.mu.Lock()
		*t.execLog = append(*t.execLog, t.name)
		t.mu.Unlock()
	}
	return &models.ToolResult{Content: t.result}, nil
}
func (t *delayTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (t *delayTool) ValidateInput(_ json.RawMessage) error    { return nil }
func (t *delayTool) IsConcurrencySafe(_ json.RawMessage) bool { return t.safe }
func (t *delayTool) IsReadOnly(_ json.RawMessage) bool        { return t.safe }

func toolUseBlock(id, name string) models.Block {
	return models.Block{
		Type:  models.BlockToolUse,
		ID:    id,
		Name:  name,
		Input: json.RawMessage(`{}`),
	}
}

// --- Tests ---

func TestStreamingExecutor_SingleTool(t *testing.T) {
	tool := &delayTool{name: "Fast", delay: 0, result: "done", safe: true}
	exec := NewStreamingExecutor(context.Background(), []Tool{tool}, nil)

	exec.AddTool(toolUseBlock("t1", "Fast"))
	exec.Done()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ToolUseID != "t1" || results[0].Content != "done" {
		t.Errorf("unexpected result: %+v", results[0])
	}
}

func TestStreamingExecutor_ConcurrentToolsRunInParallel(t *testing.T) {
	var started atomic.Int32
	var maxConcurrent atomic.Int32

	tool := &delayTool{name: "Slow", delay: 50 * time.Millisecond, result: "ok", safe: true}

	// Wrap to track concurrency
	wrapper := &concurrencyTracker{
		inner:         tool,
		started:       &started,
		maxConcurrent: &maxConcurrent,
	}

	exec := NewStreamingExecutor(context.Background(), []Tool{wrapper}, nil)

	// Add 3 concurrent-safe tools
	exec.AddTool(toolUseBlock("t1", "Slow"))
	exec.AddTool(toolUseBlock("t2", "Slow"))
	exec.AddTool(toolUseBlock("t3", "Slow"))
	exec.Done()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// All 3 should have run concurrently
	if maxConcurrent.Load() < 2 {
		t.Errorf("expected concurrent execution, max concurrent was %d", maxConcurrent.Load())
	}
}

func TestStreamingExecutor_NonConcurrentToolRunsAlone(t *testing.T) {
	var execLog []string
	var mu sync.Mutex

	safeTool := &delayTool{name: "Safe", delay: 10 * time.Millisecond, result: "s", safe: true, execLog: &execLog, mu: &mu}
	unsafeTool := &delayTool{name: "Unsafe", delay: 10 * time.Millisecond, result: "u", safe: false, execLog: &execLog, mu: &mu}

	exec := NewStreamingExecutor(context.Background(), []Tool{safeTool, unsafeTool}, nil)

	// Safe, Safe, Unsafe, Safe
	exec.AddTool(toolUseBlock("t1", "Safe"))
	exec.AddTool(toolUseBlock("t2", "Safe"))
	exec.AddTool(toolUseBlock("t3", "Unsafe"))
	exec.AddTool(toolUseBlock("t4", "Safe"))
	exec.Done()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Results should be in insertion order
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ToolUseID
	}
	expected := []string{"t1", "t2", "t3", "t4"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("result %d: expected %s, got %s", i, expected[i], id)
		}
	}
}

func TestStreamingExecutor_ResultsInInsertionOrder(t *testing.T) {
	// Fast tool added second should still come after slow tool added first
	slowTool := &delayTool{name: "Slow", delay: 50 * time.Millisecond, result: "slow", safe: true}
	fastTool := &delayTool{name: "Fast", delay: 0, result: "fast", safe: true}

	exec := NewStreamingExecutor(context.Background(), []Tool{slowTool, fastTool}, nil)

	exec.AddTool(toolUseBlock("t1", "Slow"))
	exec.AddTool(toolUseBlock("t2", "Fast"))
	exec.Done()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Even though Fast finishes first, Slow should be yielded first (insertion order)
	if results[0].ToolUseID != "t1" {
		t.Errorf("expected first result to be t1 (Slow), got %s", results[0].ToolUseID)
	}
	if results[1].ToolUseID != "t2" {
		t.Errorf("expected second result to be t2 (Fast), got %s", results[1].ToolUseID)
	}
}

func TestStreamingExecutor_UnknownToolGetsError(t *testing.T) {
	exec := NewStreamingExecutor(context.Background(), nil, nil) // no tools registered

	exec.AddTool(toolUseBlock("t1", "NonExistent"))
	exec.Done()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error result for unknown tool")
	}
}

func TestStreamingExecutor_Discard(t *testing.T) {
	slowTool := &delayTool{name: "Slow", delay: 5 * time.Second, result: "never", safe: true}
	exec := NewStreamingExecutor(context.Background(), []Tool{slowTool}, nil)

	exec.AddTool(toolUseBlock("t1", "Slow"))

	// Discard immediately — should not hang
	time.Sleep(10 * time.Millisecond) // let tool start
	exec.Discard()

	var results []models.Block
	done := make(chan struct{})
	go func() {
		for result := range exec.Results() {
			results = append(results, result)
		}
		close(done)
	}()

	select {
	case <-done:
		// Good — completed without hanging
	case <-time.After(2 * time.Second):
		t.Fatal("Discard did not unblock Results()")
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (error), got %d", len(results))
	}
	if !results[0].IsError {
		t.Error("expected error result after discard")
	}
}

func TestStreamingExecutor_AddToolsDuringExecution(t *testing.T) {
	// Simulate streaming: add tools with delays between them
	tool := &delayTool{name: "Quick", delay: 10 * time.Millisecond, result: "ok", safe: true}
	exec := NewStreamingExecutor(context.Background(), []Tool{tool}, nil)

	// Add first tool
	exec.AddTool(toolUseBlock("t1", "Quick"))

	// Add second tool after a small delay (simulating streaming)
	go func() {
		time.Sleep(20 * time.Millisecond)
		exec.AddTool(toolUseBlock("t2", "Quick"))
		time.Sleep(20 * time.Millisecond)
		exec.Done() // signal no more tools coming
	}()

	var results []models.Block
	for result := range exec.Results() {
		results = append(results, result)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestStreamingExecutor_ExecutingCount_AccurateAfterCompletion(t *testing.T) {
	tool := &delayTool{name: "Quick", delay: 10 * time.Millisecond, result: "ok", safe: true}
	exec := NewStreamingExecutor(context.Background(), []Tool{tool}, nil)

	exec.AddTool(toolUseBlock("t1", "Quick"))
	exec.AddTool(toolUseBlock("t2", "Quick"))
	exec.Done()

	// Drain all results
	for range exec.Results() {
	}

	// After all tools complete, executingCount must be zero
	exec.mu.Lock()
	count := exec.executingCount
	exec.mu.Unlock()

	if count != 0 {
		t.Errorf("expected executingCount=0 after all tools complete, got %d", count)
	}
}

func TestStreamingExecutor_ExecutingCount_MixedConcurrency(t *testing.T) {
	safeTool := &delayTool{name: "Safe", delay: 10 * time.Millisecond, result: "s", safe: true}
	unsafeTool := &delayTool{name: "Unsafe", delay: 10 * time.Millisecond, result: "u", safe: false}

	exec := NewStreamingExecutor(context.Background(), []Tool{safeTool, unsafeTool}, nil)

	exec.AddTool(toolUseBlock("t1", "Safe"))
	exec.AddTool(toolUseBlock("t2", "Unsafe"))
	exec.AddTool(toolUseBlock("t3", "Safe"))
	exec.Done()

	for range exec.Results() {
	}

	exec.mu.Lock()
	count := exec.executingCount
	exec.mu.Unlock()

	if count != 0 {
		t.Errorf("expected executingCount=0 after mixed tools complete, got %d", count)
	}
}

func TestStreamingExecutor_ExecutingCount_AfterDiscard(t *testing.T) {
	slowTool := &delayTool{name: "Slow", delay: 5 * time.Second, result: "never", safe: true}
	exec := NewStreamingExecutor(context.Background(), []Tool{slowTool}, nil)

	exec.AddTool(toolUseBlock("t1", "Slow"))
	time.Sleep(10 * time.Millisecond) // let tool start
	exec.Discard()

	for range exec.Results() {
	}

	// Wait briefly for the deferred cleanup in executeTool
	time.Sleep(50 * time.Millisecond)

	exec.mu.Lock()
	count := exec.executingCount
	exec.mu.Unlock()

	if count != 0 {
		t.Errorf("expected executingCount=0 after discard, got %d", count)
	}
}

// --- Concurrency tracker helper ---

type concurrencyTracker struct {
	inner         Tool
	started       *atomic.Int32
	maxConcurrent *atomic.Int32
}

func (c *concurrencyTracker) Name() string                 { return c.inner.Name() }
func (c *concurrencyTracker) Description() string          { return c.inner.Description() }
func (c *concurrencyTracker) InputSchema() json.RawMessage { return c.inner.InputSchema() }
func (c *concurrencyTracker) Execute(ctx context.Context, input json.RawMessage, tctx *ToolContext) (*models.ToolResult, error) {
	cur := c.started.Add(1)
	for {
		old := c.maxConcurrent.Load()
		if cur > old {
			if c.maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		} else {
			break
		}
	}
	defer c.started.Add(-1)
	return c.inner.Execute(ctx, input, tctx)
}
func (c *concurrencyTracker) CheckPermissions(input json.RawMessage, tctx *ToolContext) (*models.PermissionDecision, error) {
	return c.inner.CheckPermissions(input, tctx)
}
func (c *concurrencyTracker) ValidateInput(input json.RawMessage) error {
	return c.inner.ValidateInput(input)
}
func (c *concurrencyTracker) IsConcurrencySafe(input json.RawMessage) bool {
	return c.inner.IsConcurrencySafe(input)
}
func (c *concurrencyTracker) IsReadOnly(input json.RawMessage) bool { return c.inner.IsReadOnly(input) }
