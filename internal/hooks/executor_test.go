package hooks

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// shellEcho returns a shell command that writes the given JSON string to stdout.
func shellEcho(jsonStr string) string {
	return `printf '%s' '` + jsonStr + `'`
}

func TestRunHook_Continue(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"continue":true}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestRunHook_Deny(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"test"}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("expected Continue=false")
	}
	if result.Decision != "deny" {
		t.Errorf("expected decision=deny, got %q", result.Decision)
	}
	if result.Reason != "test" {
		t.Errorf("expected reason=test, got %q", result.Reason)
	}
}

func TestRunHook_EmptyOutput(t *testing.T) {
	cfg := HookConfig{Command: `true`} // exits 0, no output
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for empty output")
	}
}

func TestRunHook_NonZeroExit(t *testing.T) {
	cfg := HookConfig{Command: `exit 1`}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for non-zero exit (non-blocking)")
	}
}

func TestRunHook_Timeout(t *testing.T) {
	cfg := HookConfig{Command: `sleep 100`, Timeout: 1}
	start := time.Now()
	_, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestRunHook_DefaultTimeout(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{}`)} // Timeout=0 → uses default
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true (default)")
	}
}

func TestRunHook_ReceivesInput(t *testing.T) {
	// Command reads stdin and echoes something; we just verify it runs.
	cfg := HookConfig{Command: shellEcho(`{"continue":true}`)}
	input := HookInput{
		EventName: HookEventPreToolUse,
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	}
	result, err := RunHook(context.Background(), cfg, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestRunHook_UpdatedInput(t *testing.T) {
	cfg := HookConfig{Command: shellEcho(`{"continue":true,"updated_input":{"command":"echo hi"}}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPreToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
	if string(result.UpdatedInput) != `{"command":"echo hi"}` {
		t.Errorf("unexpected UpdatedInput: %s", result.UpdatedInput)
	}
}

func TestExecuteHooks_NoEvent(t *testing.T) {
	settings := HooksSettings{}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for empty settings")
	}
}

func TestExecuteHooks_MatcherNoMatch(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "^Glob$", Hooks: []HookConfig{{Command: shellEcho(`{"continue":false}`)}}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true when matcher does not match tool")
	}
}

func TestExecuteHooks_MatcherMatches_Allow(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "^Bash$", Hooks: []HookConfig{{Command: shellEcho(`{"continue":true}`)}}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestExecuteHooks_MatcherMatches_Deny(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "^Bash$", Hooks: []HookConfig{{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"blocked"}`)}}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("expected Continue=false")
	}
	if result.Decision != "deny" {
		t.Errorf("expected decision=deny, got %q", result.Decision)
	}
}

func TestExecuteHooks_EmptyMatcher_MatchesAll(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{{Command: shellEcho(`{"continue":false,"decision":"deny"}`)}}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "AnyTool"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("expected Continue=false for empty matcher (matches all)")
	}
}

func TestExecuteHooks_StopsOnFirstDeny(t *testing.T) {
	// Second hook would allow, but first denies — should stop after first.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"first"}`)},
				{Command: shellEcho(`{"continue":true}`)},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("expected Continue=false")
	}
	if result.Reason != "first" {
		t.Errorf("expected reason=first (stopped early), got %q", result.Reason)
	}
}

func TestToolMatches_EmptyPattern(t *testing.T) {
	if !toolMatches("", "Bash") {
		t.Error("empty pattern should match everything")
	}
	if !toolMatches("", "") {
		t.Error("empty pattern should match empty tool name")
	}
}

func TestToolMatches_Regex(t *testing.T) {
	if !toolMatches("^Bash$", "Bash") {
		t.Error("^Bash$ should match Bash")
	}
	if toolMatches("^Bash$", "BashTool") {
		t.Error("^Bash$ should not match BashTool")
	}
	if !toolMatches("Bash", "Bash") {
		t.Error("Bash should match Bash")
	}
	if !toolMatches("Bash", "BashTool") {
		t.Error("Bash should match BashTool (substring)")
	}
}

func TestToolMatches_InvalidRegex(t *testing.T) {
	if toolMatches("[invalid", "Bash") {
		t.Error("invalid regex should not match")
	}
}

// ─── HookConfig.If — conditional execution ────────────────────────────────────

func TestExecuteHooks_If_SkipsWhenNotMatched(t *testing.T) {
	// Hook has If="^Bash$" but tool is Read — hook should be skipped.
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
}

func TestExecuteHooks_If_RunsWhenMatched(t *testing.T) {
	// Hook has If="^Bash$" and tool is Bash — hook should run and deny.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"if-matched"}`), If: "^Bash$"},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("hook with matching If should run; expected Continue=false")
	}
	if result.Reason != "if-matched" {
		t.Errorf("Reason = %q, want %q", result.Reason, "if-matched")
	}
}

func TestExecuteHooks_If_EmptyRunsAlways(t *testing.T) {
	// Hook with empty If always runs (same as no If field).
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"system_message":"ran"}`), If: ""},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "AnyTool"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SystemMessage != "ran" {
		t.Errorf("hook with empty If should run for any tool; SystemMessage = %q, want %q", result.SystemMessage, "ran")
	}
}

// ─── HookResult.Async — fire-and-forget semantics ────────────────────────────

func TestExecuteHooks_Async_TreatedAsContinue(t *testing.T) {
	// Async hook that returns deny should be ignored — treated as Continue=true.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny","async":true}`)},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("async hook decision should be ignored; expected Continue=true")
	}
}

func TestExecuteHooks_Async_DoesNotSetUpdatedInput(t *testing.T) {
	// Async hook's UpdatedInput should not propagate.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"async":true,"updated_input":{"should":"not-appear"}}`), If: ""},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse, HookInput{ToolName: "Bash"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.UpdatedInput) > 0 {
		t.Errorf("async hook UpdatedInput should not propagate, got: %s", result.UpdatedInput)
	}
}

func TestRunHook_AsyncFieldParsed(t *testing.T) {
	// Verify Async field is correctly parsed from hook output.
	cfg := HookConfig{Command: shellEcho(`{"continue":true,"async":true}`)}
	result, err := RunHook(context.Background(), cfg, HookInput{EventName: HookEventPostToolUse})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Async {
		t.Error("Async field should be true when hook returns async:true")
	}
	if !result.Continue {
		t.Error("Continue should still default to true")
	}
}

func TestHookConfig_If_SerializesCorrectly(t *testing.T) {
	cfg := HookConfig{Command: "echo hi", Timeout: 5, If: "^Bash$"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round HookConfig
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if round.If != "^Bash$" {
		t.Errorf("If = %q, want %q", round.If, "^Bash$")
	}
}

// ─── Source trust boundary ───────────────────────────────────────────────────

func TestIsSourceTrusted_NilAllowsAll(t *testing.T) {
	if !isSourceTrusted("plugin", nil) {
		t.Error("nil trustedSources should trust all sources")
	}
	if !isSourceTrusted("user", nil) {
		t.Error("nil trustedSources should trust all sources")
	}
	if !isSourceTrusted("", nil) {
		t.Error("nil trustedSources should trust empty source")
	}
}

func TestIsSourceTrusted_EmptySourceIsUser(t *testing.T) {
	trusted := []string{"user", "builtin"}
	if !isSourceTrusted("", trusted) {
		t.Error("empty source should be treated as 'user' and be trusted")
	}
}

func TestIsSourceTrusted_PluginNotInList(t *testing.T) {
	trusted := []string{"user", "builtin"}
	if isSourceTrusted("plugin", trusted) {
		t.Error("plugin source should NOT be trusted when not in list")
	}
}

func TestIsSourceTrusted_PluginInList(t *testing.T) {
	trusted := []string{"user", "builtin", "plugin"}
	if !isSourceTrusted("plugin", trusted) {
		t.Error("plugin source should be trusted when in list")
	}
}

func TestExecuteHooks_UserHooksExecuteNormally(t *testing.T) {
	// User-sourced hooks (empty Source) should execute regardless of trustedSources.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"system_message":"user-ran"}`), Source: "user"},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse,
		HookInput{ToolName: "Bash"}, []string{"user", "builtin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SystemMessage != "user-ran" {
		t.Errorf("user hook should execute; SystemMessage = %q, want %q", result.SystemMessage, "user-ran")
	}
}

func TestExecuteHooks_PluginHooksWithoutConsentSkipped(t *testing.T) {
	// Plugin-sourced hooks should be skipped when "plugin" is not in trustedSources.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":false,"decision":"deny","reason":"should-not-run"}`), Source: "plugin"},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse,
		HookInput{ToolName: "Bash"}, []string{"user", "builtin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("plugin hook without consent should be skipped; expected Continue=true")
	}
	if result.Decision == "deny" {
		t.Error("plugin hook without consent should not produce a deny decision")
	}
}

func TestExecuteHooks_PluginHooksWithConsentExecute(t *testing.T) {
	// Plugin-sourced hooks should execute when "plugin" is in trustedSources.
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: shellEcho(`{"continue":true,"system_message":"plugin-ran"}`), Source: "plugin"},
			}},
		},
	}
	result, err := ExecuteHooks(context.Background(), settings, HookEventPreToolUse,
		HookInput{ToolName: "Bash"}, []string{"user", "builtin", "plugin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SystemMessage != "plugin-ran" {
		t.Errorf("plugin hook with consent should execute; SystemMessage = %q, want %q", result.SystemMessage, "plugin-ran")
	}
}

func TestExecuteHooks_SourcePreservedThroughLoadCycle(t *testing.T) {
	// Verify Source field serializes/deserializes correctly.
	cfg := HookConfig{Command: "echo hi", Source: "plugin"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var round HookConfig
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if round.Source != "plugin" {
		t.Errorf("Source = %q after round-trip, want %q", round.Source, "plugin")
	}
}

func TestTagSource(t *testing.T) {
	settings := HooksSettings{
		HookEventPreToolUse: {
			{Matcher: "Bash", Hooks: []HookConfig{
				{Command: "echo a"},
				{Command: "echo b"},
			}},
		},
		HookEventPostToolUse: {
			{Matcher: "", Hooks: []HookConfig{
				{Command: "echo c"},
			}},
		},
	}
	TagSource(settings, "plugin")

	for _, matchers := range settings {
		for _, m := range matchers {
			for _, h := range m.Hooks {
				if h.Source != "plugin" {
					t.Errorf("hook %q has Source=%q, want 'plugin'", h.Command, h.Source)
				}
			}
		}
	}
}
