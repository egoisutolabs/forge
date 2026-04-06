package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/hooks"
)

// encodeBashInput encodes a Bash tool command as JSON ToolInput.
func encodeBashInput(command string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"command": command})
	return b
}

// encodeWriteInput encodes a Write tool input as JSON.
func encodeWriteInput(filePath string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"file_path": filePath})
	return b
}

// hookInput builds a minimal HookInput for a Bash pre-tool-use event.
func hookInputBash(command string) hooks.HookInput {
	return hooks.HookInput{
		EventName: hooks.HookEventPreToolUse,
		ToolName:  "Bash",
		ToolInput: encodeBashInput(command),
	}
}

// hookInputWrite builds a minimal HookInput for a Write pre-tool-use event.
func hookInputWrite(path string) hooks.HookInput {
	return hooks.HookInput{
		EventName: hooks.HookEventPreToolUse,
		ToolName:  "Write",
		ToolInput: encodeWriteInput(path),
	}
}

// TestForgeHooks_ReturnsSettings verifies that ForgeHooks returns a non-empty settings map.
func TestForgeHooks_ReturnsSettings(t *testing.T) {
	s := ForgeHooks()
	if len(s) == 0 {
		t.Fatal("ForgeHooks returned empty settings")
	}
}

// TestForgeHooks_HasPreToolUseMatchers verifies that both Bash and Write matchers are present.
func TestForgeHooks_HasPreToolUseMatchers(t *testing.T) {
	s := ForgeHooks()
	matchers, ok := s[hooks.HookEventPreToolUse]
	if !ok {
		t.Fatal("PreToolUse matchers missing from ForgeHooks")
	}

	var hasBash, hasWrite bool
	for _, m := range matchers {
		switch m.Matcher {
		case "Bash":
			hasBash = true
		case "Write":
			hasWrite = true
		}
	}
	if !hasBash {
		t.Error("expected Bash matcher in PreToolUse")
	}
	if !hasWrite {
		t.Error("expected Write matcher in PreToolUse")
	}
}

// TestForgeHooks_HasPostToolUseMatcher verifies the Write|Edit post-hook.
func TestForgeHooks_HasPostToolUseMatcher(t *testing.T) {
	s := ForgeHooks()
	matchers, ok := s[hooks.HookEventPostToolUse]
	if !ok {
		t.Fatal("PostToolUse matchers missing from ForgeHooks")
	}
	if len(matchers) == 0 {
		t.Fatal("PostToolUse matchers empty")
	}
}

// ─── blockDestructiveCommand ────────────────────────────────────────────────

func TestBlockDestructive_RmRfRoot(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("rm -rf / --no-preserve-root"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("rm -rf / should be blocked")
	}
	if result.Decision != "deny" {
		t.Errorf("decision = %q, want deny", result.Decision)
	}
}

func TestBlockDestructive_RmRfDot(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("rm -rf ."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("rm -rf . should be blocked")
	}
}

func TestBlockDestructive_GitClean(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("git clean -fdx"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("git clean -fdx should be blocked")
	}
}

func TestBlockDestructive_GitResetHard(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("git reset --hard HEAD~1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("git reset --hard should be blocked")
	}
}

func TestBlockDestructive_DropTable(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("psql -c 'DROP TABLE users'"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("DROP TABLE should be blocked")
	}
}

func TestBlockDestructive_SafeCommand(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("go test ./..."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Errorf("safe command blocked: reason=%q", result.Reason)
	}
}

func TestBlockDestructive_SafeRm(t *testing.T) {
	result, err := blockDestructiveCommand(hookInputBash("rm /tmp/forge-tmp-abc123"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Errorf("rm /tmp/specific-file should not be blocked: reason=%q", result.Reason)
	}
}

func TestBlockDestructive_InvalidJSON(t *testing.T) {
	input := hooks.HookInput{ToolInput: json.RawMessage(`not json`)}
	result, err := blockDestructiveCommand(input)
	if err != nil {
		t.Fatalf("invalid JSON should not error: %v", err)
	}
	if !result.Continue {
		t.Error("invalid JSON input should be allowed (fail-open)")
	}
}

// ─── blockSensitiveWrite ────────────────────────────────────────────────────

func TestBlockSensitiveWrite_DotEnv(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/project/.env"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error(".env write should be blocked")
	}
}

func TestBlockSensitiveWrite_PemFile(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/certs/server.pem"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error(".pem write should be blocked")
	}
}

func TestBlockSensitiveWrite_IdRsa(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/home/user/.ssh/id_rsa"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error("id_rsa write should be blocked")
	}
}

func TestBlockSensitiveWrite_AwsCredentials(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/home/user/.aws/credentials"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Continue {
		t.Error(".aws/credentials write should be blocked")
	}
}

func TestBlockSensitiveWrite_SafeGoFile(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/project/main.go"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Errorf("main.go should not be blocked: reason=%q", result.Reason)
	}
}

func TestBlockSensitiveWrite_SafeMarkdown(t *testing.T) {
	result, err := blockSensitiveWrite(hookInputWrite("/project/README.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Errorf("README.md should not be blocked: reason=%q", result.Reason)
	}
}

func TestBlockSensitiveWrite_InvalidJSON(t *testing.T) {
	input := hooks.HookInput{ToolInput: json.RawMessage(`bad`)}
	result, err := blockSensitiveWrite(input)
	if err != nil {
		t.Fatalf("invalid JSON should not error: %v", err)
	}
	if !result.Continue {
		t.Error("invalid JSON input should be allowed (fail-open)")
	}
}

// ─── formatEditedFile ───────────────────────────────────────────────────────

func TestFormatEditedFile_NonGoFile(t *testing.T) {
	input := hooks.HookInput{
		EventName: hooks.HookEventPostToolUse,
		ToolName:  "Write",
		ToolInput: encodeWriteInput("/project/notes.md"),
	}
	result, err := formatEditedFile(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("format of non-Go file should always continue")
	}
}

func TestFormatEditedFile_GoFile_Continues(t *testing.T) {
	// gofmt may not be in PATH in all test envs, but hook must always continue.
	input := hooks.HookInput{
		EventName: hooks.HookEventPostToolUse,
		ToolName:  "Write",
		ToolInput: encodeWriteInput("/nonexistent/path/main.go"),
	}
	result, err := formatEditedFile(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("formatEditedFile must always return Continue=true (best-effort)")
	}
}

func TestFormatEditedFile_InvalidJSON_Continues(t *testing.T) {
	input := hooks.HookInput{ToolInput: json.RawMessage(`bad`)}
	result, err := formatEditedFile(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("invalid JSON should continue (fail-open)")
	}
}

// ─── RegisterInternalHook (via ForgeHooks) ─────────────────────────────────

func TestInternalHook_Dispatch(t *testing.T) {
	// Register a custom internal hook and verify it's dispatched.
	hooks.RegisterInternalHook("test-hook", func(input hooks.HookInput) (*hooks.HookResult, error) {
		return &hooks.HookResult{Continue: false, Decision: "deny", Reason: "test"}, nil
	})

	cfg := hooks.HookConfig{Command: "forge-internal:test-hook"}
	result, err := hooks.RunHook(context.Background(), cfg, hooks.HookInput{})
	if err != nil {
		t.Fatalf("RunHook: %v", err)
	}
	if result.Continue {
		t.Error("internal hook should have returned Continue=false")
	}
	if result.Decision != "deny" {
		t.Errorf("decision = %q, want deny", result.Decision)
	}
}

func TestInternalHook_UnknownName_NoOp(t *testing.T) {
	cfg := hooks.HookConfig{Command: "forge-internal:does-not-exist-xyz"}
	result, err := hooks.RunHook(context.Background(), cfg, hooks.HookInput{})
	if err != nil {
		t.Fatalf("RunHook: %v", err)
	}
	if !result.Continue {
		t.Error("unknown internal hook should be a no-op (Continue=true)")
	}
}
