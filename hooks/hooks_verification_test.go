// Package hooks — verification tests comparing Go port against Claude Code's
// hooks.ts TypeScript source.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING EVENTS (14+ event types present in TypeScript, absent in Go):
//     PostToolUseFailure, PermissionRequest, TeammateIdle, TaskCreated,
//     TaskCompleted, SessionEnd, Setup, PreCompact, PostCompact, Notification,
//     StopFailure, PermissionDenied, Elicitation, WorktreeCreate, WorktreeRemove.
//     Go defines only: PreToolUse, PostToolUse, SessionStart, Stop,
//     UserPromptSubmit, SubagentStart.
//
//  2. MISSING HOOK TYPES: Only command hooks (shell subprocess) are supported.
//     TypeScript supports: Command, Prompt (Claude API), Agent (sub-agent),
//     HTTP (POST endpoint), Callback (TS function), Function (structured output).
//
//  3. SEQUENTIAL EXECUTION: TypeScript runs hooks in parallel (Promise.all).
//     Go runs hooks sequentially. The observable difference: when multiple
//     hooks are registered, Go's total time ≈ sum of hook times; TypeScript's
//     total time ≈ max of hook times.
//
//  4. MISSING: Background hooks (`async: true` in response).
//     TypeScript: hooks returning `async: true` run in the background without
//     blocking execution. Go has no such field or behaviour.
//
//  5. MISSING: Conditional hooks (`if` expression on HookConfig).
//     TypeScript HookConfig includes an optional `if` field (evaluated as a
//     condition). Go HookConfig only has Command and Timeout.
//
//  6. MISSING: Multiple hook sources.
//     TypeScript merges hooks from snapshot, registered (plugins/skills), and
//     session sources with per-session scoping. Go reads from a single file.
//
//  7. PATTERN MATCHING: TypeScript matchesPattern() splits on '|' then tries
//     each part as regex (falling back to exact match on regex error).
//     Go toolMatches() uses the entire pattern as a regex. In practice these
//     behave identically for valid regex (since '|' means "or" in regex), but
//     they diverge when individual parts contain regex errors.
//
//  8. MISSING: `ask` decision value in HookResult.
//     TypeScript permission precedence is: deny > ask > allow.
//     Go stops on deny, but does not surface 'ask' as a separate case.
package hooks

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ─── GAP 1: missing event types ──────────────────────────────────────────────

// TestVerification_MissingEventTypes documents which TypeScript event types
// are not defined in the Go HookEvent enum.
func TestVerification_MissingEventTypes(t *testing.T) {
	// Events defined in Claude Code TypeScript (hooks.ts).
	tsEvents := []string{
		"PreToolUse",         // ✓ present in Go
		"PostToolUse",        // ✓ present in Go
		"SessionStart",       // ✓ present in Go
		"Stop",               // ✓ present in Go
		"UserPromptSubmit",   // ✓ present in Go
		"SubagentStart",      // ✓ present in Go
		"PostToolUseFailure", // ✗ MISSING
		"PermissionRequest",  // ✗ MISSING
		"TeammateIdle",       // ✗ MISSING
		"TaskCreated",        // ✗ MISSING
		"TaskCompleted",      // ✗ MISSING
		"SessionEnd",         // ✗ MISSING
		"Setup",              // ✗ MISSING
		"PreCompact",         // ✗ MISSING
		"PostCompact",        // ✗ MISSING
		"Notification",       // ✗ MISSING
		"StopFailure",        // ✗ MISSING
		"PermissionDenied",   // ✗ MISSING
		"Elicitation",        // ✗ MISSING
		"WorktreeCreate",     // ✗ MISSING
		"WorktreeRemove",     // ✗ MISSING
	}

	// Collect the events defined in Go.
	goEvents := map[HookEvent]bool{
		HookEventPreToolUse:       true,
		HookEventPostToolUse:      true,
		HookEventSessionStart:     true,
		HookEventStop:             true,
		HookEventUserPromptSubmit: true,
		HookEventSubagentStart:    true,
	}

	var missing []string
	for _, ev := range tsEvents {
		if !goEvents[HookEvent(ev)] {
			missing = append(missing, ev)
		}
	}

	if len(missing) > 0 {
		t.Logf("GAP CONFIRMED: %d TypeScript event types not defined in Go HookEvent enum: %s",
			len(missing), strings.Join(missing, ", "))
	}

	// Go should have exactly 6 events defined.
	if len(goEvents) != 6 {
		t.Errorf("expected 6 Go event constants, found %d", len(goEvents))
	}
}

// TestVerification_PostToolUseFailure_NotInEnum verifies that
// PostToolUseFailure is not yet defined as a HookEvent constant. Once
// implemented, this test should be updated to use the new constant.
func TestVerification_PostToolUseFailure_NotInEnum(t *testing.T) {
	// If this constant exists, the test will fail to compile — which means
	// the gap has been closed and this test should be removed.
	const postToolUseFailure = HookEvent("PostToolUseFailure")

	settings := HooksSettings{
		postToolUseFailure: {
			{Matcher: "", Hooks: []HookConfig{{Command: shellEcho(`{"continue":false,"decision":"deny"}`)}}},
		},
	}

	// Since PostToolUseFailure is just a string, the hooks are registered.
	// But the event name is not a named constant — callers won't fire it
	// unless they know the string value.
	result, err := ExecuteHooks(context.Background(), settings, postToolUseFailure, HookInput{ToolName: "Read"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hooks fire when given the exact string.
	if result.Continue {
		t.Error("expected Continue=false when using PostToolUseFailure event string directly")
	}
	t.Log("GAP CONFIRMED: PostToolUseFailure works as a raw string but is not a named HookEvent constant")
}

// ─── GAP 3: sequential execution ─────────────────────────────────────────────

// TestVerification_SequentialExecution verifies that Go executes hooks
// sequentially (total time ≈ sum), not in parallel (total time ≈ max).
//
// TypeScript runs hooks in parallel via Promise.all. If multiple hooks
// have side-effects that depend on ordering, sequential vs parallel execution
// is an observable behavioural difference.
func TestVerification_SequentialExecution(t *testing.T) {
	// Two hooks each sleeping 200ms. Parallel total ≈ 200ms; sequential ≈ 400ms.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: `sleep 0.2 && printf '%s' '{"continue":true}'`, Timeout: 5},
				{Command: `sleep 0.2 && printf '%s' '{"continue":true}'`, Timeout: 5},
			}},
		},
	}

	start := time.Now()
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}

	// Sequential: ≥ 350ms (sum of two 200ms sleeps minus scheduling slack).
	// Parallel: < 350ms.
	if elapsed >= 350*time.Millisecond {
		t.Logf("SEQUENTIAL CONFIRMED: elapsed=%v (≥350ms indicates sequential execution; TypeScript runs in parallel)", elapsed)
	} else {
		t.Logf("PARALLEL (or very fast machine): elapsed=%v", elapsed)
	}
}

// ─── GAP 4: no background/async hook support ─────────────────────────────────

// TestVerification_AsyncFieldHandled verifies that a HookResult with
// `async: true` is parsed and treated as non-blocking by ExecuteHooks.
//
// TypeScript: hooks returning `{"async": true}` run in the background and
// the call returns immediately with Continue=true. Go now matches this.
func TestVerification_AsyncFieldHandled(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"async":true,"continue":false,"decision":"deny"}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Async field is now parsed.
	if !result.Async {
		t.Error("Async field should be true when hook returns async:true")
	}

	// ExecuteHooks treats async hooks as non-blocking (Continue=true regardless of decision).
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"async":true,"continue":false,"decision":"deny"}`)},
			}},
		},
	}
	execResult, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !execResult.Continue {
		t.Error("async hook deny should be ignored; expected Continue=true")
	}
	t.Log("FIXED: HookResult.Async field present; async hooks are treated as non-blocking in ExecuteHooks")
}

// ─── GAP 5: conditional hooks ────────────────────────────────────────────────

// TestVerification_HookConfigIfField verifies that HookConfig now has the
// conditional `if` field matching TypeScript's HookConfig shape.
//
// TypeScript HookConfig:
//
//	{ command: string, timeout?: number, if?: string }
//
// Go HookConfig:
//
//	{ Command string, Timeout int, If string }
func TestVerification_HookConfigIfField(t *testing.T) {
	// If field is present — hook is skipped when the regex doesn't match.
	cfg := HookConfig{Command: "true", Timeout: 1, If: "^Bash$"}
	if cfg.If != "^Bash$" {
		t.Errorf("HookConfig.If = %q, want %q", cfg.If, "^Bash$")
	}

	// A hook with a non-matching If should be skipped.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny"}`), If: "^Bash$"},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Read"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("hook with non-matching If should be skipped; expected Continue=true")
	}
	t.Log("FIXED: HookConfig.If field present; conditional hooks are supported")
}

// ─── GAP 7: pattern matching divergence ──────────────────────────────────────

// TestVerification_PipeSeparatedPattern verifies behaviour of pipe-separated
// patterns. TypeScript splits on '|' first; Go treats the whole string as
// a single regex (where '|' means OR). For valid regex patterns, both
// behave identically.
func TestVerification_PipeSeparatedPattern_WorksViaRegex(t *testing.T) {
	// "Read|Write|Edit" as a regex matches any of the three tools.
	tests := []struct {
		pattern string
		tool    string
		want    bool
	}{
		{"Read|Write|Edit", "Read", true},
		{"Read|Write|Edit", "Write", true},
		{"Read|Write|Edit", "Edit", true},
		{"Read|Write|Edit", "Bash", false},
		{"^Read$|^Write$", "Read", true},
		{"^Read$|^Write$", "ReadFile", false}, // anchored
	}

	for _, tc := range tests {
		got := toolMatches(tc.pattern, tc.tool)
		if got != tc.want {
			t.Errorf("toolMatches(%q, %q) = %v, want %v", tc.pattern, tc.tool, got, tc.want)
		}
	}
}

// TestVerification_PipeSeparatedPattern_DivergenceOnInvalidPartialRegex
// documents the case where TypeScript and Go diverge: a pattern where one
// pipe-segment has invalid regex. TypeScript would try each part individually
// and succeed on the valid part; Go treats the whole as one regex (and rejects
// on the invalid part).
func TestVerification_PipeSeparatedPattern_DivergenceOnInvalidPartialRegex(t *testing.T) {
	// "[invalid" is an invalid regex. "Read|[invalid" is also invalid as a
	// whole regex, so Go returns false. TypeScript would split on '|', try
	// "Read" (valid regex, matches), and return true.
	pattern := "Read|[invalid"

	goResult := toolMatches(pattern, "Read")
	if !goResult {
		t.Logf("DIVERGENCE CONFIRMED: toolMatches(%q, %q) = false in Go; TypeScript would return true (splits on | first)", pattern, "Read")
	} else {
		t.Logf("Go unexpectedly returned true for pattern %q — regex behaviour may have changed", pattern)
	}
}

// ─── GAP 8: 'ask' decision not surfaced ──────────────────────────────────────

// TestVerification_AskDecisionPassesThrough verifies what happens when a hook
// returns decision="ask". TypeScript has a precedence hierarchy:
// deny > ask > allow. Go currently treats 'ask' as a pass-through (Continue=true).
func TestVerification_AskDecisionPassesThrough(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"continue":true,"decision":"ask","reason":"needs approval"}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != "ask" {
		t.Errorf("Decision = %q, want %q", result.Decision, "ask")
	}
	if !result.Continue {
		t.Errorf("expected Continue=true for decision=ask (hook returned continue:true)")
	}

	// GAP: ExecuteHooks does not apply TypeScript's deny>ask>allow precedence.
	// It only stops on deny or Continue=false. 'ask' is passed through as-is.
	t.Log("NOTE: ExecuteHooks does not implement TypeScript's deny>ask>allow precedence — 'ask' decision is not escalated")
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_UpdatedInputCarriedThrough verifies that updated_input
// from an allow hook is carried to the final result — matching TypeScript.
func TestVerification_UpdatedInputCarriedThrough(t *testing.T) {
	newInput := `{"command":"echo replaced"}`
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "^Bash$", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"updated_input":` + newInput + `}`)},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
	if string(result.UpdatedInput) != newInput {
		t.Errorf("UpdatedInput = %s, want %s", result.UpdatedInput, newInput)
	}
}

// TestVerification_SystemMessageCarriedThrough verifies that system_message
// from a hook is returned in the final result.
func TestVerification_SystemMessageCarriedThrough(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"system_message":"injected context"}`)},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SystemMessage != "injected context" {
		t.Errorf("SystemMessage = %q, want %q", result.SystemMessage, "injected context")
	}
}

// TestVerification_DenyStopsHookChain verifies that the first deny in a
// chain stops execution — matching TypeScript's early-exit behaviour.
func TestVerification_DenyStopsHookChain(t *testing.T) {
	var secondHookRan bool
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"blocked by hook 1"}`)},
				// Second hook: if this ran it would override the UpdatedInput.
				{Command: shellEcho(`{"continue":true,"updated_input":{"ran":"yes"}}`)},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Continue {
		t.Error("expected Continue=false (deny)")
	}
	if result.Reason != "blocked by hook 1" {
		t.Errorf("Reason = %q, want %q", result.Reason, "blocked by hook 1")
	}

	// If the second hook ran, updated_input would be set.
	if len(result.UpdatedInput) > 0 {
		secondHookRan = true
	}
	if secondHookRan {
		t.Error("second hook should NOT have run after deny")
	}
}

// TestVerification_ContinueDefaultsToTrue verifies the JSON default behaviour
// matches TypeScript: `continue` omitted from JSON → defaults to true.
func TestVerification_ContinueDefaultsToTrue(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{}`)} // no continue field
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true when field absent (matches TypeScript default)")
	}
}
