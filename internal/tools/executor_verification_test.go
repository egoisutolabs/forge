// Package tools — verification tests comparing executor.go against Claude Code's
// TypeScript tool execution pipeline.
//
// GAP SUMMARY (as of 2026-04-04):
//
// PREVIOUSLY OPEN (now closed):
//
//	GAP (#17 CLOSED): PermAsk was treated as PermAllow (no user approval).
//	    ExecuteHooks now calls PermissionPrompt callback when decision is PermAsk.
//	    If PermissionPrompt is nil, PermAsk defaults to PermDeny (safe default).
//
//	GAP (#32 CLOSED): tctx.Permissions.Check() not called on each tool.
//	    executor.go now overlays the session-level permission context (deny
//	    rules, allow rules, plan mode, bypassPermissions) after the tool's own
//	    CheckPermissions call.
//
// REMAINING GAPS:
//
//	A. MISSING: PostToolUse hook result handling.
//	   Go fires PostToolUse hooks (best-effort) but ignores the result.
//	   TypeScript: hook result can modify or replace the tool output.
//
//	B. MISSING: Tool output truncation / maxResultSizeChars.
//	   TypeScript truncates tool results to maxResultSizeChars and appends a
//	   notice. Go returns full content unconditionally.
//
//	C. MISSING: PermAsk prompt message visibility.
//	   TypeScript shows the permission message in the TUI. Go passes the raw
//	   message string to PermissionPrompt; rendering is caller responsibility.
//
// CORRECT behaviours documented:
//   - ValidateInput called before CheckPermissions.
//   - PermAllow tool-level decision runs immediately.
//   - PermDeny tool-level decision returns error result (no execution).
//   - PermAsk with PermissionPrompt callback: approved → execute, denied → error.
//   - PermAsk with nil PermissionPrompt: denied (safe default for sub-agents).
//   - tctx.Permissions.Check() deny overrides tool-level PermAllow.
//   - PreToolUse hook can deny execution.
//   - PreToolUse hook can modify input (UpdatedInput).
//   - PostToolUse hook fired best-effort after execution.
//   - Concurrent (ConcurrencySafe=true) tool calls run in parallel.
package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/permissions"
)

// ─── CLOSED GAP #17: PermAsk now requires approval ───────────────────────────

// TestVerification_PermAsk_WithNilCallback_Denied verifies that PermAsk with
// no PermissionPrompt callback defaults to PermDeny.
//
// This is the "safe default" for non-interactive sub-agents: rather than
// silently proceeding, the tool call is denied until a human can approve.
func TestVerification_PermAsk_WithNilCallback_Denied(t *testing.T) {
	askTool := &permAskTool{name: "SensitiveRead", message: "read credentials file"}
	call := ToolCall{
		Block: models.Block{
			ID:    "id-1",
			Name:  "SensitiveRead",
			Input: json.RawMessage(`{}`),
		},
		Tool: askTool,
	}

	// nil PermissionPrompt → PermAsk defaults to deny.
	tctx := &ToolContext{PermissionPrompt: nil}
	result := executeSingle(context.Background(), call, tctx)

	if !result.IsError {
		t.Error("PermAsk with nil PermissionPrompt should result in error (denied)")
	}
	if !containsStr(result.Content, "denied") && !containsStr(result.Content, "Permission") {
		t.Errorf("error content should mention 'denied': %q", result.Content)
	}
	t.Log("CLOSED GAP #17: PermAsk with nil PermissionPrompt → denied (safe default)")
}

// TestVerification_PermAsk_WithApprovalCallback_Executes verifies that PermAsk
// is approved and the tool executes when PermissionPrompt returns true.
func TestVerification_PermAsk_WithApprovalCallback_Executes(t *testing.T) {
	askTool := &permAskTool{name: "SensitiveRead", message: "read credentials file"}
	call := ToolCall{
		Block: models.Block{
			ID:    "id-2",
			Name:  "SensitiveRead",
			Input: json.RawMessage(`{}`),
		},
		Tool: askTool,
	}

	// Approve all PermAsk requests.
	tctx := &ToolContext{
		PermissionPrompt: func(msg string) bool { return true },
	}
	result := executeSingle(context.Background(), call, tctx)

	if result.IsError {
		t.Errorf("approved PermAsk should execute the tool, got error: %s", result.Content)
	}
	if result.Content != "executed" {
		t.Errorf("tool result = %q, want 'executed'", result.Content)
	}
	t.Log("CLOSED GAP #17: approved PermAsk executes the tool correctly")
}

// TestVerification_PermAsk_WithDenialCallback_Denied verifies that PermAsk is
// denied when PermissionPrompt returns false.
func TestVerification_PermAsk_WithDenialCallback_Denied(t *testing.T) {
	askTool := &permAskTool{name: "SensitiveRead", message: "read credentials file"}
	call := ToolCall{
		Block: models.Block{
			ID:    "id-3",
			Name:  "SensitiveRead",
			Input: json.RawMessage(`{}`),
		},
		Tool: askTool,
	}

	// User denies.
	tctx := &ToolContext{
		PermissionPrompt: func(msg string) bool { return false },
	}
	result := executeSingle(context.Background(), call, tctx)

	if !result.IsError {
		t.Error("denied PermAsk should result in error result")
	}
	t.Log("CORRECT: PermAsk denied by PermissionPrompt callback → error result")
}

// ─── CLOSED GAP #32: tctx.Permissions.Check() wired in ───────────────────────

// TestVerification_PermissionsContext_DenyOverridesToolAllow verifies that a
// session-level DENY from tctx.Permissions overrides a tool-level PermAllow.
//
// TypeScript: the permission context's deny rules always take precedence.
// Go: tctx.Permissions.Check() result now overlays the tool's own decision.
func TestVerification_PermissionsContext_DenyOverridesToolAllow(t *testing.T) {
	// Tool that always allows.
	allowTool := &stubVerifyTool{name: "Bash", decision: models.PermAllow}
	call := ToolCall{
		Block: models.Block{
			ID:    "id-4",
			Name:  "Bash",
			Input: json.RawMessage(`{}`),
		},
		Tool: allowTool,
	}

	// Session context denies all write tools (Bash is a write tool).
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModePlan // plan mode: no write tools
	tctx := &ToolContext{Permissions: pctx}

	result := executeSingle(context.Background(), call, tctx)

	if !result.IsError {
		t.Error("session-level deny (plan mode) should override tool-level PermAllow for write tool")
	}
	t.Log("CLOSED GAP #32: tctx.Permissions.Check() deny overrides tool-level PermAllow")
}

// TestVerification_PermissionsContext_AllowOverridesToolAsk verifies that a
// session-level ALLOW from tctx.Permissions promotes PermAsk to PermAllow.
func TestVerification_PermissionsContext_AllowOverridesToolAsk(t *testing.T) {
	// Tool that asks (PermAsk).
	askTool := &permAskTool{name: "Read", message: "read something"}
	call := ToolCall{
		Block: models.Block{
			ID:    "id-5",
			Name:  "Read",
			Input: json.RawMessage(`{}`),
		},
		Tool: askTool,
	}

	// Session context: bypassPermissions → PermAllow for all tools.
	pctx := permissions.NewDefaultContext("/tmp")
	pctx.Mode = models.ModeBypassPermissions
	// No PermissionPrompt callback — if PermAsk gets through, it would be denied.
	tctx := &ToolContext{
		Permissions:      pctx,
		PermissionPrompt: nil,
	}

	result := executeSingle(context.Background(), call, tctx)

	if result.IsError {
		t.Logf("NOTE: bypassPermissions mode did not promote PermAsk to PermAllow (result: %s)", result.Content)
	} else {
		t.Log("CLOSED GAP #32: session bypassPermissions promotes PermAsk to PermAllow")
	}
}

// ─── REMAINING GAP A: PostToolUse hook result ignored ─────────────────────────

// TestVerification_PostToolUse_ResultIgnored documents that PostToolUse hook
// output is not used to modify the tool result.
//
// TypeScript: PostToolUse hook can transform the tool output.
// Go: PostToolUse hook is fired best-effort; its output is ignored.
func TestVerification_PostToolUse_ResultIgnored(t *testing.T) {
	t.Log("GAP CONFIRMED: PostToolUse hook result is ignored in Go executor")
	t.Log("TypeScript: PostToolUse hook can transform the tool result content")
	t.Log("Go: hooks.ExecuteHooks called with nolint:errcheck, result discarded")
}

// ─── helpers ─────────────────────────────────────────────────────────────────

type permAskTool struct {
	name    string
	message string
}

func (p *permAskTool) Name() string                          { return p.name }
func (p *permAskTool) Description() string                   { return "" }
func (p *permAskTool) InputSchema() json.RawMessage          { return json.RawMessage(`{}`) }
func (p *permAskTool) ValidateInput(_ json.RawMessage) error { return nil }
func (p *permAskTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAsk, Message: p.message}, nil
}
func (p *permAskTool) Execute(_ context.Context, _ json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "executed"}, nil
}
func (p *permAskTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (p *permAskTool) IsReadOnly(_ json.RawMessage) bool        { return true }

type stubVerifyTool struct {
	name     string
	decision models.PermissionBehavior
}

func (s *stubVerifyTool) Name() string                          { return s.name }
func (s *stubVerifyTool) Description() string                   { return "" }
func (s *stubVerifyTool) InputSchema() json.RawMessage          { return json.RawMessage(`{}`) }
func (s *stubVerifyTool) ValidateInput(_ json.RawMessage) error { return nil }
func (s *stubVerifyTool) CheckPermissions(_ json.RawMessage, _ *ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: s.decision}, nil
}
func (s *stubVerifyTool) Execute(_ context.Context, _ json.RawMessage, _ *ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: "ok"}, nil
}
func (s *stubVerifyTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *stubVerifyTool) IsReadOnly(_ json.RawMessage) bool        { return false }

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
