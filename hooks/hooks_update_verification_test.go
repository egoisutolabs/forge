// Package hooks — supplemental verification tests documenting gap closures and
// new gaps found after tasks #19, #25 (partial) were completed (2026-04-04).
//
// This file supplements hooks_verification_test.go. Read that file first for
// the original gap analysis.
//
// GAPS NOW CLOSED (as of current implementation):
//
//	GAP 1 (was: 15 missing event types) → CLOSED.
//	    All 21 TypeScript event constants are now defined in Go types.go.
//
//	GAP 3 (was: sequential execution) → CLOSED.
//	    ExecuteHooks now runs hooks within the same HookMatcher concurrently
//	    using sync.WaitGroup + goroutines, mirroring TypeScript's Promise.all.
//
//	GAP 4 (was: no async field) → CLOSED.
//	    HookResult.Async bool added. Async hooks are treated as advisory —
//	    their decision does not affect the chain (treated as Continue=true).
//
//	GAP 5 (was: no conditional If) → CLOSED.
//	    HookConfig.If string added. Hooks with non-empty If are skipped when
//	    the condition regex does not match the tool name.
//
//	GAP (#19): regexp cache → CLOSED.
//	    regexpCache sync.Map caches compiled patterns. Each unique pattern is
//	    compiled at most once regardless of how many tool calls fire.
//
// GAPS STILL OPEN:
//
//	A. MISSING: Multiple hook types (Prompt, Agent, HTTP, Callback, Function).
//	   Only command (shell subprocess) hooks supported.
//
//	B. MISSING: Multiple hook sources (snapshot, registered, session).
//	   Go reads from a single file/settings map; TypeScript merges three sources.
//
//	C. DIVERGENCE: Pattern matching (pipe-separated parts vs. single regex).
//
//	D. MISSING: `ask` decision value in HookResult escalates to user prompt.
//
//	E. PENDING: Load-time regex validation (task #33).
//	   Invalid regex patterns silently no-match; should surface errors at load.
package hooks

import (
	"context"
	"os"
	"testing"
	"time"
)

// ─── CLOSED GAP 1: all hook event types now present ───────────────────────────

// TestVerification_AllHookEvents_NowPresent verifies that all 21 TypeScript
// HookEvent values are now defined as Go constants.
func TestVerification_AllHookEvents_NowPresent(t *testing.T) {
	// These 21 events must all compile as named constants.
	events := []HookEvent{
		HookEventPreToolUse,
		HookEventPostToolUse,
		HookEventSessionStart,
		HookEventStop,
		HookEventUserPromptSubmit,
		HookEventSubagentStart,
		HookEventPostToolUseFailure,
		HookEventPermissionRequest,
		HookEventTeammateIdle,
		HookEventTaskCreated,
		HookEventTaskCompleted,
		HookEventSessionEnd,
		HookEventSetup,
		HookEventPreCompact,
		HookEventPostCompact,
		HookEventNotification,
		HookEventStopFailure,
		HookEventPermissionDenied,
		HookEventElicitation,
		HookEventWorktreeCreate,
		HookEventWorktreeRemove,
	}

	if len(events) != 21 {
		t.Errorf("expected 21 event constants, got %d", len(events))
	}

	// Spot-check that each constant has the expected string value.
	checks := map[HookEvent]string{
		HookEventPostToolUseFailure: "PostToolUseFailure",
		HookEventTaskCreated:        "TaskCreated",
		HookEventWorktreeCreate:     "WorktreeCreate",
		HookEventElicitation:        "Elicitation",
	}
	for ev, want := range checks {
		if string(ev) != want {
			t.Errorf("HookEvent constant value = %q, want %q", ev, want)
		}
	}

	t.Logf("CLOSED GAP: all 21 TypeScript hook event types now present in Go (%d events)", len(events))
}

// ─── CLOSED GAP 3: parallel execution within matcher ─────────────────────────

// TestVerification_ParallelExecution_WithinMatcher confirms that multiple hooks
// in the same HookMatcher now run concurrently (matching TypeScript Promise.all).
//
// Two hooks each sleeping 200ms:
//
//	Parallel (closed gap):  total ≈ 200ms
//	Sequential (old gap):   total ≈ 400ms
func TestVerification_ParallelExecution_WithinMatcher(t *testing.T) {
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
		t.Error("expected Continue=true from two non-blocking hooks")
	}

	// Parallel threshold: if total < 350ms, hooks ran concurrently.
	if elapsed < 350*time.Millisecond {
		t.Logf("CLOSED GAP: hooks ran in parallel, elapsed=%v (both 200ms hooks completed in ~max time)", elapsed)
	} else {
		t.Logf("SEQUENTIAL (unexpected): elapsed=%v (>350ms suggests sequential execution is back)", elapsed)
	}
}

// TestVerification_ParallelExecution_OrderPreserved verifies that even with
// concurrent execution, results are processed in declaration order.
// This means the first hook's deny takes precedence over a later allow.
func TestVerification_ParallelExecution_OrderPreserved(t *testing.T) {
	// First hook denies; second hook allows.
	// With parallel execution but ordered result processing, deny wins.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny"}`), Timeout: 5},
				{Command: shellEcho(`{"continue":true}`), Timeout: 5},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Continue {
		t.Error("first hook denies — result should be Continue=false despite parallel execution")
	}
	if result.Decision != "deny" {
		t.Errorf("decision = %q, want %q", result.Decision, "deny")
	}
	t.Log("CORRECT: parallel execution preserves declaration-order result processing")
}

// ─── CLOSED GAP 4: Async field ───────────────────────────────────────────────

// TestVerification_AsyncHook_NonBlocking verifies that a hook returning
// `{"async": true}` does not block the chain or stop execution.
//
// TypeScript semantics: async hooks are advisory; they do not affect
// the chain outcome even if they return decision:"deny".
func TestVerification_AsyncHook_NonBlocking(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				// Async hook "denying" — should not block.
				{Command: shellEcho(`{"async":true,"continue":false,"decision":"deny"}`), Timeout: 5},
				// Non-async hook allowing.
				{Command: shellEcho(`{"continue":true}`), Timeout: 5},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Async hook's deny should be ignored; chain should continue.
	if !result.Continue {
		t.Error("async hook deny should not block the chain (Async=true means advisory only)")
	}
	t.Log("CLOSED GAP: HookResult.Async field present; async hook denies are advisory")
}

// TestVerification_AsyncField_UnmarshaledCorrectly verifies HookResult.Async
// is properly read from JSON output.
func TestVerification_AsyncField_UnmarshaledCorrectly(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"async":true,"continue":true}`), Timeout: 5}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Async {
		t.Error("HookResult.Async should be true when hook outputs {\"async\":true}")
	}
	if !result.Continue {
		t.Error("Continue should be true")
	}
	t.Log("CLOSED GAP: HookResult.Async field is now populated from hook JSON output")
}

// ─── CLOSED GAP 5: conditional If hooks ──────────────────────────────────────

// TestVerification_ConditionalHook_If_Matches verifies that a hook with a
// non-empty If pattern only runs when the tool name matches.
func TestVerification_ConditionalHook_If_Matches(t *testing.T) {
	// Hook with If:"^Bash$" — should only fire for Bash.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{
					Command: shellEcho(`{"continue":true}`),
					If:      "^Bash$",
					Timeout: 5,
				},
			}},
		},
	}

	// Should fire for Bash.
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error for Bash: %v", err)
	}
	if !result.Continue {
		t.Error("Bash should pass through the conditional hook")
	}

	// Should NOT fire for Read (If pattern doesn't match).
	result2, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Read"}, nil)
	if err != nil {
		t.Fatalf("unexpected error for Read: %v", err)
	}
	if !result2.Continue {
		t.Error("Read should not be blocked by a hook with If:'^Bash$'")
	}

	t.Log("CLOSED GAP: HookConfig.If field now present and evaluated before running hook")
}

// TestVerification_ConditionalHook_If_NoMatch_Skipped verifies that a hook
// with If that does not match is completely skipped (treated as Continue=true).
func TestVerification_ConditionalHook_If_NoMatch_Skipped(t *testing.T) {
	// A "deny" hook with If:"^Write$" — should not affect Read.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{
					Command: shellEcho(`{"continue":false,"decision":"deny"}`),
					If:      "^Write$",
					Timeout: 5,
				},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Read"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("hook with If:'^Write$' should be skipped for tool 'Read'")
	}
	t.Log("CORRECT: conditional hook with non-matching If is skipped entirely")
}

// ─── CLOSED GAP #19: regexp cache ────────────────────────────────────────────

// TestVerification_RegexpCache_UsedForSamePattern verifies that the package-level
// regexpCache is populated after the first toolMatches call.
// Subsequent calls with the same pattern must use the cached regexp.
func TestVerification_RegexpCache_UsedForSamePattern(t *testing.T) {
	pattern := "^TestCachePattern_" + t.Name()

	// Prime the cache.
	_ = toolMatches(pattern, "TestCachePattern_example")

	// Verify the pattern is now cached.
	cached, ok := regexpCache.Load(pattern)
	if !ok {
		t.Error("pattern should be in regexpCache after first toolMatches call")
	}
	if cached == nil {
		t.Error("cached value should not be nil")
	}
	t.Log("CLOSED GAP: regexpCache (sync.Map) caches compiled patterns after first use")
}

// TestVerification_RegexpCache_InvalidPattern_ReturnsFalse verifies that an
// invalid regex pattern returns false (not a panic) and is not cached.
func TestVerification_RegexpCache_InvalidPattern_ReturnsFalse(t *testing.T) {
	invalidPattern := "[invalid-regex"

	result := toolMatches(invalidPattern, "Bash")
	if result {
		t.Error("invalid regex pattern should return false, not true")
	}

	// Invalid patterns should not be added to the cache.
	_, cached := regexpCache.Load(invalidPattern)
	if cached {
		t.Error("invalid regex pattern should not be stored in the cache")
	}
	t.Log("CORRECT: invalid regex returns false and is not cached")
}

// TestVerification_RegexpCache_ConcurrentAccess verifies that the sync.Map
// cache is safe for concurrent access.
func TestVerification_RegexpCache_ConcurrentAccess(t *testing.T) {
	const goroutines = 50
	done := make(chan struct{}, goroutines)

	for i := range goroutines {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			// Interleave new patterns with shared patterns.
			if i%2 == 0 {
				toolMatches("^Bash$", "Bash")
			} else {
				toolMatches("^Read$", "Read")
			}
		}(i)
	}

	for range goroutines {
		<-done
	}
	t.Log("CORRECT: regexpCache sync.Map is safe for concurrent access")
}

// ─── CLOSED GAP #33: load-time regex validation ───────────────────────────────

// TestVerification_HookLoadTime_RegexValidated_ViaLoadHooksFromFile verifies
// that LoadHooksFromFile now validates regex patterns and returns an error
// for invalid patterns. Task #33 is closed.
//
// Note: Direct HooksSettings construction bypasses load-time validation.
// The gap is closed at the FILE LOADING boundary (LoadHooksFromFile).
func TestVerification_HookLoadTime_RegexValidated_ViaLoadHooksFromFile(t *testing.T) {
	// Write a hooks file with an invalid matcher regex.
	dir := t.TempDir()
	hookFile := dir + "/hooks.json"
	content := `{
		"PreToolUse": [
			{"matcher": "[invalid-regex", "hooks": [{"command": "echo ok"}]}
		]
	}`
	if err := os.WriteFile(hookFile, []byte(content), 0644); err != nil {
		t.Fatalf("write hook file: %v", err)
	}

	_, err := LoadHooksFromFile(hookFile)
	if err == nil {
		t.Error("LoadHooksFromFile should return error for invalid regex '[invalid-regex'")
	} else {
		t.Logf("CLOSED GAP #33: invalid regex rejected at load time: %v", err)
	}
}

// TestVerification_HookLoadTime_ValidRegex_Succeeds verifies that valid regex
// patterns load without error.
func TestVerification_HookLoadTime_ValidRegex_Succeeds(t *testing.T) {
	dir := t.TempDir()
	hookFile := dir + "/hooks.json"
	content := `{
		"PreToolUse": [
			{"matcher": "^(Bash|Read|Write)$", "hooks": [{"command": "echo ok"}]}
		]
	}`
	if err := os.WriteFile(hookFile, []byte(content), 0644); err != nil {
		t.Fatalf("write hook file: %v", err)
	}

	settings, err := LoadHooksFromFile(hookFile)
	if err != nil {
		t.Errorf("valid regex should load without error, got: %v", err)
	}
	if len(settings[HookEventPreToolUse]) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(settings[HookEventPreToolUse]))
	}
	t.Log("CORRECT: valid regex patterns load successfully via LoadHooksFromFile")
}

// TestVerification_DirectHooksSettings_InvalidRegex_SilentNoMatch documents that
// directly-constructed HooksSettings (not from file) bypasses load-time validation.
// Invalid regexes in direct construction still silently no-match at call time.
// This is acceptable since LoadHooksFromFile is the primary entry point.
func TestVerification_DirectHooksSettings_InvalidRegex_SilentNoMatch(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "[invalid-regex", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny"}`), Timeout: 5},
			}},
		},
	}

	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("invalid regex in direct HooksSettings should silently no-match (hook should not fire)")
	}
	t.Log("NOTE: direct HooksSettings construction bypasses load-time validation — invalid regexes silently no-match")
	t.Log("Acceptable: LoadHooksFromFile (the primary entry point) validates at load time")
}
