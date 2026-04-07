package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHooksFromFile_Valid(t *testing.T) {
	content := `{
		"PreToolUse": [
			{"matcher": "^Bash$", "hooks": [{"command": "echo ok", "timeout": 5}]}
		],
		"PostToolUse": [
			{"matcher": "", "hooks": [{"command": "true"}]}
		]
	}`

	tmp := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	settings, err := LoadHooksFromFile(tmp)
	if err != nil {
		t.Fatalf("LoadHooksFromFile: %v", err)
	}

	pre := settings[HookEventPreToolUse]
	if len(pre) != 1 {
		t.Fatalf("expected 1 PreToolUse matcher, got %d", len(pre))
	}
	if pre[0].Matcher != "^Bash$" {
		t.Errorf("unexpected matcher: %q", pre[0].Matcher)
	}
	if len(pre[0].Hooks) != 1 || pre[0].Hooks[0].Command != "echo ok" {
		t.Errorf("unexpected hooks: %+v", pre[0].Hooks)
	}
	if pre[0].Hooks[0].Timeout != 5 {
		t.Errorf("expected timeout=5, got %d", pre[0].Hooks[0].Timeout)
	}

	post := settings[HookEventPostToolUse]
	if len(post) != 1 {
		t.Fatalf("expected 1 PostToolUse matcher, got %d", len(post))
	}
}

func TestLoadHooksFromFile_NotFound(t *testing.T) {
	_, err := LoadHooksFromFile("/nonexistent/path/hooks.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadHooksFromFile_InvalidJSON(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmp, []byte(`{bad json`), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadHooksFromFile(tmp)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMergeHooks_Empty(t *testing.T) {
	result := MergeHooks(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestMergeHooks_OneSided(t *testing.T) {
	a := HooksSettings{
		HookEventPreToolUse: {{Matcher: "A", Hooks: []HookConfig{{Command: "echo a"}}}},
	}
	result := MergeHooks(a, nil)
	if len(result[HookEventPreToolUse]) != 1 {
		t.Errorf("expected 1 matcher, got %d", len(result[HookEventPreToolUse]))
	}
}

func TestMergeHooks_Combines(t *testing.T) {
	a := HooksSettings{
		HookEventPreToolUse: {{Matcher: "A", Hooks: []HookConfig{{Command: "echo a"}}}},
	}
	b := HooksSettings{
		HookEventPreToolUse:  {{Matcher: "B", Hooks: []HookConfig{{Command: "echo b"}}}},
		HookEventPostToolUse: {{Matcher: "", Hooks: []HookConfig{{Command: "echo post"}}}},
	}
	result := MergeHooks(a, b)

	pre := result[HookEventPreToolUse]
	if len(pre) != 2 {
		t.Fatalf("expected 2 PreToolUse matchers, got %d", len(pre))
	}
	if pre[0].Matcher != "A" || pre[1].Matcher != "B" {
		t.Errorf("unexpected matchers: %q %q", pre[0].Matcher, pre[1].Matcher)
	}

	post := result[HookEventPostToolUse]
	if len(post) != 1 {
		t.Fatalf("expected 1 PostToolUse matcher, got %d", len(post))
	}
}

// ── Regex validation at load time (SEC-6) ────────────────────────────────────

func TestLoadHooksFromFile_InvalidRegex_ReturnsError(t *testing.T) {
	content := `{
		"PreToolUse": [
			{"matcher": "[invalid", "hooks": [{"command": "echo ok"}]}
		]
	}`
	tmp := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadHooksFromFile(tmp)
	if err == nil {
		t.Error("expected error for invalid regex pattern")
	}
}

func TestLoadHooksFromFile_EmptyMatcher_IsValid(t *testing.T) {
	// Empty matcher means "match everything" — must not be treated as invalid regex.
	content := `{
		"PreToolUse": [
			{"matcher": "", "hooks": [{"command": "echo ok"}]}
		]
	}`
	tmp := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadHooksFromFile(tmp)
	if err != nil {
		t.Errorf("empty matcher should be valid, got error: %v", err)
	}
}

func TestLoadHooksFromFile_ValidRegex_NoError(t *testing.T) {
	content := `{
		"PreToolUse": [
			{"matcher": "^(Bash|Glob)$", "hooks": [{"command": "echo ok"}]}
		]
	}`
	tmp := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := LoadHooksFromFile(tmp)
	if err != nil {
		t.Errorf("valid regex should load without error, got: %v", err)
	}
}

func TestMergeHooks_DoesNotMutateInputs(t *testing.T) {
	a := HooksSettings{
		HookEventPreToolUse: {{Matcher: "A"}},
	}
	b := HooksSettings{
		HookEventPreToolUse: {{Matcher: "B"}},
	}
	MergeHooks(a, b)

	if len(a[HookEventPreToolUse]) != 1 {
		t.Error("MergeHooks must not mutate a")
	}
	if len(b[HookEventPreToolUse]) != 1 {
		t.Error("MergeHooks must not mutate b")
	}
}
