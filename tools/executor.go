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

// ToolCall represents a single tool invocation from Claude's response.
type ToolCall struct {
	Block models.Block
	Tool  Tool
}

// Batch is a group of tool calls that can be executed together.
type Batch struct {
	Calls           []ToolCall
	ConcurrencySafe bool
}

// PartitionBatches groups tool calls into ordered batches.
// Consecutive concurrency-safe tools share a batch.
// Any non-safe tool gets its own serial batch.
func PartitionBatches(calls []ToolCall) []Batch {
	if len(calls) == 0 {
		return nil
	}

	var batches []Batch
	for _, call := range calls {
		safe := call.Tool.IsConcurrencySafe(call.Block.Input)

		if len(batches) == 0 || !safe || !batches[len(batches)-1].ConcurrencySafe {
			batches = append(batches, Batch{ConcurrencySafe: safe})
		}
		batches[len(batches)-1].Calls = append(batches[len(batches)-1].Calls, call)
	}
	return batches
}

// ExecuteBatches runs all batches in order. Returns tool_result blocks for
// every tool call (errors included).
func ExecuteBatches(ctx context.Context, batches []Batch, tctx *ToolContext) []models.Block {
	var allResults []models.Block
	for _, batch := range batches {
		if batch.ConcurrencySafe {
			allResults = append(allResults, executeConcurrent(ctx, batch.Calls, tctx)...)
		} else {
			allResults = append(allResults, executeSerial(ctx, batch.Calls, tctx)...)
		}
	}
	return allResults
}

func executeConcurrent(ctx context.Context, calls []ToolCall, tctx *ToolContext) []models.Block {
	results := make([]models.Block, len(calls))
	var wg sync.WaitGroup
	wg.Add(len(calls))

	for i, call := range calls {
		go func(idx int, c ToolCall) {
			defer wg.Done()
			results[idx] = executeSingle(ctx, c, tctx)
		}(i, call)
	}

	wg.Wait()
	return results
}

func executeSerial(ctx context.Context, calls []ToolCall, tctx *ToolContext) []models.Block {
	results := make([]models.Block, 0, len(calls))
	for _, call := range calls {
		results = append(results, executeSingle(ctx, call, tctx))
	}
	return results
}

func executeSingle(ctx context.Context, call ToolCall, tctx *ToolContext) models.Block {
	traceID := observe.EmitToolStart(call.Block.Name, call.Block.ID, call.Block.Input, call.Tool.IsConcurrencySafe(call.Block.Input))
	toolStart := time.Now()

	result := executeSingleInner(ctx, call, tctx)

	observe.EmitToolEnd(traceID, call.Block.Name, call.Block.ID, time.Since(toolStart), result.Content, result.IsError)
	return result
}

func executeSingleInner(ctx context.Context, call ToolCall, tctx *ToolContext) models.Block {
	if err := call.Tool.ValidateInput(call.Block.Input); err != nil {
		return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Validation error: %s", err), true)
	}

	decision, err := call.Tool.CheckPermissions(call.Block.Input, tctx)
	if err != nil {
		return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Permission check error: %s", err), true)
	}

	// Overlay with session-level permission context (deny rules, allow rules, plan
	// mode, bypassPermissions). A context deny always wins; a context allow promotes
	// a PermAsk to PermAllow (e.g. bypassPermissions or an explicit allow rule).
	if tctx != nil && tctx.Permissions != nil {
		ctxDecision := tctx.Permissions.Check(call.Block.Name, call.Tool.IsReadOnly(call.Block.Input))
		if ctxDecision.Behavior == models.PermDeny || ctxDecision.Behavior == models.PermAllow {
			decision = ctxDecision
		}
	}

	switch decision.Behavior {
	case models.PermDeny:
		return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Permission denied: %s", decision.Message), true)
	case models.PermAsk:
		// PermAsk requires interactive approval. If a PermissionPrompt callback is
		// wired up, ask the user. Otherwise deny (safe default for sub-agents and
		// non-interactive runs).
		approved := tctx != nil && tctx.PermissionPrompt != nil && tctx.PermissionPrompt(decision.Message)
		if !approved {
			msg := decision.Message
			if msg == "" {
				msg = "interactive approval required"
			}
			return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Permission denied: %s", msg), true)
		}
	}

	// Run PreToolUse hooks before execution.
	if tctx != nil && len(tctx.Hooks) > 0 {
		hookInput := hooks.HookInput{
			EventName: hooks.HookEventPreToolUse,
			ToolName:  call.Block.Name,
			ToolInput: call.Block.Input,
		}
		hookResult, hookErr := hooks.ExecuteHooks(ctx, tctx.Hooks, hooks.HookEventPreToolUse, hookInput, tctx.TrustedSources)
		if hookErr != nil {
			return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Hook error: %s", hookErr), true)
		}
		if !hookResult.Continue || hookResult.Decision == "deny" {
			reason := hookResult.Reason
			if reason == "" {
				reason = "denied by hook"
			}
			return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Hook denied: %s", reason), true)
		}
		if len(hookResult.UpdatedInput) > 0 {
			call.Block.Input = hookResult.UpdatedInput
		}
	}

	result, err := call.Tool.Execute(ctx, call.Block.Input, tctx)
	if err != nil {
		return models.NewToolResultBlock(call.Block.ID, fmt.Sprintf("Error: %s", err), true)
	}

	// Run PostToolUse hooks after execution (best-effort; errors are ignored).
	if tctx != nil && len(tctx.Hooks) > 0 {
		outputJSON, _ := json.Marshal(result.Content)
		hookInput := hooks.HookInput{
			EventName:  hooks.HookEventPostToolUse,
			ToolName:   call.Block.Name,
			ToolInput:  call.Block.Input,
			ToolOutput: outputJSON,
		}
		hooks.ExecuteHooks(ctx, tctx.Hooks, hooks.HookEventPostToolUse, hookInput, tctx.TrustedSources) //nolint:errcheck
	}

	return models.NewToolResultBlock(call.Block.ID, result.Content, result.IsError)
}

// ResolveToolCalls maps tool_use blocks to their Tool implementations.
func ResolveToolCalls(blocks []models.Block, tools []Tool) (resolved []ToolCall, unknown []models.Block) {
	for _, b := range blocks {
		t := FindTool(tools, b.Name)
		if t == nil {
			unknown = append(unknown, models.NewToolResultBlock(b.ID, fmt.Sprintf("Unknown tool: %s", b.Name), true))
			continue
		}
		resolved = append(resolved, ToolCall{Block: b, Tool: t})
	}
	return
}

// ExecuteToolBlocks is the high-level entry point: resolves, partitions, executes.
func ExecuteToolBlocks(ctx context.Context, blocks []models.Block, availableTools []Tool, tctx *ToolContext) []models.Block {
	resolved, unknownResults := ResolveToolCalls(blocks, availableTools)
	batches := PartitionBatches(resolved)
	results := ExecuteBatches(ctx, batches, tctx)
	return append(unknownResults, results...)
}
