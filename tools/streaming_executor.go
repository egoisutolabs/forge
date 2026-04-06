package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/observe"
)

// toolStatus tracks a tool through its lifecycle.
// Mirrors Claude Code's TrackedTool states: queued → executing → completed → yielded.
type toolStatus int

const (
	statusQueued toolStatus = iota
	statusExecuting
	statusCompleted
	statusYielded
)

// trackedTool is the internal state for a single tool in the streaming executor.
type trackedTool struct {
	id         string
	block      models.Block
	tool       Tool
	status     toolStatus
	isConcSafe bool
	result     *models.Block // the tool_result block
	done       chan struct{} // closed when execution completes
}

// StreamingExecutor runs tools as they arrive during API streaming.
//
// This is the Go equivalent of Claude Code's StreamingToolExecutor.
// Key behaviors:
//   - Tools are added incrementally via AddTool() as the API streams
//   - Concurrent-safe tools run in parallel immediately
//   - Non-concurrent tools run exclusively (nothing else executing)
//   - Results are yielded in insertion order via Results() channel
//   - Discard() cancels all pending work (for fallback recovery)
//   - Done() signals no more tools will be added
type StreamingExecutor struct {
	mu        sync.Mutex
	tools     []*trackedTool // stable insertion-order list
	available []Tool         // registered tool definitions
	tctx      *ToolContext   // shared tool context for execution

	cancel context.CancelFunc
	ctx    context.Context

	// wakeup signals that something changed (tool added, tool completed, done/discard called)
	wakeup    chan struct{}
	doneFlag  bool // no more tools will be added
	discarded bool

	// executingCount tracks how many tools are currently in statusExecuting.
	// Incremented when a tool starts executing, decremented when it completes.
	// Avoids O(n) scans in canExecuteLocked().
	executingCount    int
	executingConcSafe bool // true when all currently executing tools are concurrency-safe
}

// NewStreamingExecutor creates a new executor with the given tool definitions.
// ctx is used as the parent context for tool execution; it is cancelled on Discard().
// tctx is the shared tool context passed to each tool's Execute and CheckPermissions.
func NewStreamingExecutor(ctx context.Context, available []Tool, tctx *ToolContext) *StreamingExecutor {
	cancelCtx, cancel := context.WithCancel(ctx)
	return &StreamingExecutor{
		available: available,
		tctx:      tctx,
		ctx:       cancelCtx,
		cancel:    cancel,
		wakeup:    make(chan struct{}, 1),
	}
}

// AddTool registers a new tool_use block for execution.
// Called during API streaming as each tool_use block completes.
// Non-blocking — execution starts asynchronously.
func (se *StreamingExecutor) AddTool(block models.Block) {
	se.mu.Lock()
	defer se.mu.Unlock()

	if se.discarded {
		return
	}

	tool := FindTool(se.available, block.Name)
	if tool == nil {
		// Unknown tool — immediately completed with error
		errResult := models.NewToolResultBlock(block.ID, fmt.Sprintf("Unknown tool: %s", block.Name), true)
		se.tools = append(se.tools, &trackedTool{
			id:     block.ID,
			block:  block,
			status: statusCompleted,
			result: &errResult,
			done:   closedChan(),
		})
		se.signal()
		return
	}

	safe := tool.IsConcurrencySafe(block.Input)
	tt := &trackedTool{
		id:         block.ID,
		block:      block,
		tool:       tool,
		status:     statusQueued,
		isConcSafe: safe,
		done:       make(chan struct{}),
	}
	se.tools = append(se.tools, tt)
	se.signal()
	se.processQueueLocked()
}

// Done signals that no more tools will be added.
// Must be called after the API stream completes so Results() knows when to close.
func (se *StreamingExecutor) Done() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.doneFlag = true
	se.signal()
}

// Discard cancels all pending and in-progress tools.
// Used for streaming fallback recovery.
func (se *StreamingExecutor) Discard() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.discarded = true
	se.cancel() // cancel all in-progress tools

	// Mark queued tools as completed with error
	for _, tt := range se.tools {
		if tt.status == statusQueued {
			errResult := models.NewToolResultBlock(tt.id, "Discarded: streaming fallback", true)
			tt.result = &errResult
			tt.status = statusCompleted
			close(tt.done)
		}
	}
	se.doneFlag = true
	se.signal()
}

// Results returns a channel that yields tool_result blocks in insertion order.
// The channel is closed when all tools have been yielded and Done()/Discard() was called.
//
// This combines Claude Code's getCompletedResults() and getRemainingResults()
// into a single channel — Go's natural model for this.
func (se *StreamingExecutor) Results() <-chan models.Block {
	out := make(chan models.Block)

	go func() {
		defer close(out)
		nextIdx := 0 // index of next tool to yield (insertion order)

		for {
			se.mu.Lock()
			// Try to yield completed tools in order
			for nextIdx < len(se.tools) {
				tt := se.tools[nextIdx]
				if tt.status == statusCompleted || tt.status == statusYielded {
					if tt.status == statusCompleted && tt.result != nil {
						tt.status = statusYielded
						result := *tt.result
						se.mu.Unlock()
						out <- result
						se.mu.Lock()
					}
					nextIdx++
					continue
				}
				if tt.status == statusExecuting && !tt.isConcSafe {
					// Non-concurrent tool still executing — block yielding past it
					break
				}
				if tt.status == statusExecuting || tt.status == statusQueued {
					break // can't yield yet
				}
			}

			// Check if we're done
			allDone := se.doneFlag && nextIdx >= len(se.tools)
			discarded := se.discarded && nextIdx >= len(se.tools)
			se.mu.Unlock()

			if allDone || discarded {
				return
			}

			// Wait for something to change
			se.waitForChange(nextIdx)
		}
	}()

	return out
}

// processQueueLocked starts queued tools that are eligible to run.
// Must be called with se.mu held.
func (se *StreamingExecutor) processQueueLocked() {
	for _, tt := range se.tools {
		if tt.status != statusQueued {
			continue
		}
		if se.canExecuteLocked(tt.isConcSafe) {
			tt.status = statusExecuting
			se.executingCount++
			if se.executingCount == 1 {
				se.executingConcSafe = tt.isConcSafe
			} else {
				se.executingConcSafe = se.executingConcSafe && tt.isConcSafe
			}
			go se.executeTool(tt)
		} else if !tt.isConcSafe {
			break // non-concurrent tool blocks queue
		}
	}
}

// canExecuteLocked checks if a tool with the given concurrency safety can run now.
// Must be called with se.mu held.
// Uses executingCount/executingConcSafe counters — O(1) instead of scanning all tools.
func (se *StreamingExecutor) canExecuteLocked(isConcSafe bool) bool {
	if se.executingCount == 0 {
		return true
	}
	// Can only add if both the new tool and all currently executing tools are conc-safe.
	return isConcSafe && se.executingConcSafe
}

// executeTool runs a single tool to completion.
func (se *StreamingExecutor) executeTool(tt *trackedTool) {
	traceID := observe.EmitToolStart(tt.block.Name, tt.id, tt.block.Input, tt.isConcSafe)
	toolStart := time.Now()

	defer func() {
		// Emit tool_call_end before releasing the slot.
		se.mu.Lock()
		result := tt.result
		se.mu.Unlock()
		var output string
		var isError bool
		if result != nil {
			output = result.Content
			isError = result.IsError
		}
		observe.EmitToolEnd(traceID, tt.block.Name, tt.id, time.Since(toolStart), output, isError)

		close(tt.done)
		se.mu.Lock()
		se.executingCount--
		if se.executingCount == 0 {
			se.executingConcSafe = false
		}
		se.processQueueLocked() // kick off next tools
		se.mu.Unlock()
		se.signal()
	}()

	// Validate
	if err := tt.tool.ValidateInput(tt.block.Input); err != nil {
		errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Validation error: %s", err), true)
		se.mu.Lock()
		tt.result = &errResult
		tt.status = statusCompleted
		se.mu.Unlock()
		return
	}

	// Permissions
	decision, err := tt.tool.CheckPermissions(tt.block.Input, se.tctx)
	if err != nil {
		errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Permission error: %s", err), true)
		se.mu.Lock()
		tt.result = &errResult
		tt.status = statusCompleted
		se.mu.Unlock()
		return
	}
	if decision.Behavior == models.PermDeny {
		errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Permission denied: %s", decision.Message), true)
		se.mu.Lock()
		tt.result = &errResult
		tt.status = statusCompleted
		se.mu.Unlock()
		return
	}
	if decision.Behavior == models.PermAsk {
		// PermAsk requires interactive approval. If a PermissionPrompt callback is
		// wired up, ask the user. Otherwise deny (safe default for sub-agents and
		// non-interactive runs).
		approved := se.tctx != nil && se.tctx.PermissionPrompt != nil && se.tctx.PermissionPrompt(decision.Message)
		if !approved {
			msg := decision.Message
			if msg == "" {
				msg = "interactive approval required"
			}
			errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Permission denied: %s", msg), true)
			se.mu.Lock()
			tt.result = &errResult
			tt.status = statusCompleted
			se.mu.Unlock()
			return
		}
	}

	// Run PreToolUse hooks before execution.
	if se.tctx != nil && len(se.tctx.Hooks) > 0 {
		hookInput := hooks.HookInput{
			EventName: hooks.HookEventPreToolUse,
			ToolName:  tt.block.Name,
			ToolInput: tt.block.Input,
		}
		hookResult, hookErr := hooks.ExecuteHooks(se.ctx, se.tctx.Hooks, hooks.HookEventPreToolUse, hookInput, se.tctx.TrustedSources)
		if hookErr != nil {
			errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Hook error: %s", hookErr), true)
			se.mu.Lock()
			tt.result = &errResult
			tt.status = statusCompleted
			se.mu.Unlock()
			return
		}
		if !hookResult.Continue || hookResult.Decision == "deny" {
			reason := hookResult.Reason
			if reason == "" {
				reason = "denied by hook"
			}
			errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Hook denied: %s", reason), true)
			se.mu.Lock()
			tt.result = &errResult
			tt.status = statusCompleted
			se.mu.Unlock()
			return
		}
		if len(hookResult.UpdatedInput) > 0 {
			tt.block.Input = hookResult.UpdatedInput
		}
	}

	// Execute
	result, err := tt.tool.Execute(se.ctx, tt.block.Input, se.tctx)
	if err != nil {
		errResult := models.NewToolResultBlock(tt.id, fmt.Sprintf("Error: %s", err), true)
		se.mu.Lock()
		tt.result = &errResult
		tt.status = statusCompleted
		se.mu.Unlock()
		return
	}

	// Run PostToolUse hooks after execution (best-effort; errors are ignored).
	if se.tctx != nil && len(se.tctx.Hooks) > 0 {
		outputJSON, _ := json.Marshal(result.Content)
		hookInput := hooks.HookInput{
			EventName:  hooks.HookEventPostToolUse,
			ToolName:   tt.block.Name,
			ToolInput:  tt.block.Input,
			ToolOutput: outputJSON,
		}
		hooks.ExecuteHooks(se.ctx, se.tctx.Hooks, hooks.HookEventPostToolUse, hookInput, se.tctx.TrustedSources) //nolint:errcheck
	}

	resultBlock := models.NewToolResultBlock(tt.id, result.Content, result.IsError)
	se.mu.Lock()
	tt.result = &resultBlock
	tt.status = statusCompleted
	se.mu.Unlock()
}

// signal wakes up the Results() goroutine.
func (se *StreamingExecutor) signal() {
	select {
	case se.wakeup <- struct{}{}:
	default:
	}
}

// waitForChange blocks until a tool completes or a signal arrives.
//
// executeTool calls signal() after every completion, and AddTool/Done/Discard
// each call signal() too — so simply waiting on wakeup covers all cases without
// building a per-call waitChans slice or acquiring the lock.
func (se *StreamingExecutor) waitForChange(_ int) {
	<-se.wakeup
}

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
