package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/hooks"
	"github.com/egoisutolabs/forge/models"
)

// shellEchoHook returns a HookConfig whose command prints the given JSON to stdout.
func shellEchoHook(jsonStr string) hooks.HookConfig {
	return hooks.HookConfig{Command: `printf '%s' '` + jsonStr + `'`}
}

func tctxWithHooks(settings hooks.HooksSettings) *ToolContext {
	return &ToolContext{Hooks: settings}
}

// TestExecuteSingle_PreToolUse_Deny verifies that a denying PreToolUse hook
// prevents the tool from executing and returns an error block.
func TestExecuteSingle_PreToolUse_Deny(t *testing.T) {
	tool := &countingTool{}
	settings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			{Matcher: "", Hooks: []hooks.HookConfig{
				shellEchoHook(`{"continue":false,"decision":"deny","reason":"blocked by policy"}`),
			}},
		},
	}

	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Counter", Input: json.RawMessage(`{"text":"hello"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, tctxWithHooks(settings))

	if !result.IsError {
		t.Error("expected error block when hook denies")
	}
	if tool.execCount.Load() != 0 {
		t.Error("tool must not execute when PreToolUse hook denies")
	}
}

// TestExecuteSingle_PreToolUse_UpdatedInput verifies that the updated_input
// from a PreToolUse hook replaces the original input passed to the tool.
func TestExecuteSingle_PreToolUse_UpdatedInput(t *testing.T) {
	tool := &echoTool{}
	settings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			{Matcher: "", Hooks: []hooks.HookConfig{
				shellEchoHook(`{"continue":true,"updated_input":{"text":"modified"}}`),
			}},
		},
	}

	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"original"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, tctxWithHooks(settings))

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "modified" {
		t.Errorf("expected tool to receive modified input, got %q", result.Content)
	}
}

// TestExecuteSingle_PreToolUse_Allow verifies that an allowing PreToolUse hook
// lets execution proceed normally.
func TestExecuteSingle_PreToolUse_Allow(t *testing.T) {
	tool := &echoTool{}
	settings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			{Matcher: "^Echo$", Hooks: []hooks.HookConfig{
				shellEchoHook(`{"continue":true}`),
			}},
		},
	}

	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"hello"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, tctxWithHooks(settings))

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "hello" {
		t.Errorf("expected result %q, got %q", "hello", result.Content)
	}
}

// TestExecuteSingle_PostToolUse_Runs verifies that PostToolUse hooks execute
// without blocking the tool result.
func TestExecuteSingle_PostToolUse_Runs(t *testing.T) {
	tool := &echoTool{}
	// A PostToolUse hook that returns deny should NOT block the result —
	// post-hooks are best-effort and their decision is not acted on.
	settings := hooks.HooksSettings{
		hooks.HookEventPostToolUse: {
			{Matcher: "", Hooks: []hooks.HookConfig{
				shellEchoHook(`{"continue":true}`),
			}},
		},
	}

	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"world"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, tctxWithHooks(settings))

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "world" {
		t.Errorf("expected result %q, got %q", "world", result.Content)
	}
}

// TestExecuteSingle_NoHooks verifies baseline behaviour is unchanged when
// ToolContext has no hooks configured.
func TestExecuteSingle_NoHooks(t *testing.T) {
	tool := &echoTool{}
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"baseline"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, &ToolContext{})

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "baseline" {
		t.Errorf("expected result %q, got %q", "baseline", result.Content)
	}
}

// TestExecuteSingle_NilTctx verifies that a nil ToolContext (no hooks) still works.
func TestExecuteSingle_NilTctx(t *testing.T) {
	tool := &echoTool{}
	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"nil-tctx"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, nil)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "nil-tctx" {
		t.Errorf("expected result %q, got %q", "nil-tctx", result.Content)
	}
}

// TestExecuteSingle_PreToolUse_MatcherSkipsOtherTools verifies that a hook
// registered for one tool name does not affect other tools.
func TestExecuteSingle_PreToolUse_MatcherSkipsOtherTools(t *testing.T) {
	tool := &echoTool{}
	settings := hooks.HooksSettings{
		hooks.HookEventPreToolUse: {
			// Deny only for "Bash"; Echo should pass through.
			{Matcher: "^Bash$", Hooks: []hooks.HookConfig{
				shellEchoHook(`{"continue":false,"decision":"deny"}`),
			}},
		},
	}

	call := ToolCall{
		Block: models.Block{ID: "t1", Name: "Echo", Input: json.RawMessage(`{"text":"pass"}`)},
		Tool:  tool,
	}
	result := executeSingle(context.Background(), call, tctxWithHooks(settings))

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "pass" {
		t.Errorf("expected result %q, got %q", "pass", result.Content)
	}
}
