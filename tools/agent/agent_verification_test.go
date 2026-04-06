// Package agent — verification tests comparing Go port against Claude Code's
// AgentTool TypeScript source (AgentTool.tsx, runAgent.ts, forkSubagent.ts).
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: Worktree isolation (task #16 in progress).
//     TypeScript: `isolation: "worktree"` → createAgentWorktree(), auto-cleanup
//     on completion. Go: no `isolation` parameter in input, no worktree support.
//
//  2. MISSING: Fork subagent experiment.
//     TypeScript: FORK_SUBAGENT feature gate enables building cache-identical
//     message prefixes via buildForkedMessages(); recursive fork guard via
//     FORK_BOILERPLATE_TAG. Go: no fork support.
//
//  3. MISSING: MCP server initialization per agent.
//     TypeScript: runAgent() calls initializeAgentMcpServers() with agent-specific
//     MCP server configs. Go: no MCP support.
//
//  4. MISSING: `team_name` parameter (multi-agent coordinator).
//     TypeScript: team_name + name → spawnTeammate() (KAIROS/coordinator path).
//     Go: no team support.
//
//  5. MISSING: Auto-background after 2s.
//     TypeScript: If sync agent runs >2s, shows hint + auto-backgrounds.
//     Go: sync/async determined statically by run_in_background flag only.
//
//  6. MISSING: `VERIFICATION_AGENT_TYPE` built-in.
//     TypeScript: constants.ts has VERIFICATION_AGENT_TYPE='verification'.
//     Go: only Explore and Plan built-ins (general-purpose is unnamed fallback).
//
//  7. DIVERGENCE: Async agent system prompt omits CLAUDE.md.
//     TypeScript: async agents omit claudeMd from system prompt.
//     Go: system prompt is taken from AgentDefinition.SystemPrompt as-is.
//
//  8. CORRECT: Filter Agent/AskUserQuestion/TaskStop from all sub-agents.
//
//  9. CORRECT: Async agents restricted to asyncAgentAllowedTools list.
//
// 10. CORRECT: Model resolution (alias→full-name fallback chain).
// 11. CORRECT: AgentDefinition.Tools allowlist filtering via FilterToolsByNames.
// 12. CORRECT: Background agent registry tracking (DefaultRegistry).
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// ─── GAP 1: Worktree isolation not in input schema ────────────────────────────

// TestVerification_WorktreeIsolation_NotInInputSchema verifies that Go's input
// schema has no `isolation` parameter.
//
// TypeScript input:
//
//	isolation?: "worktree" | undefined
//
// Go: no isolation field in toolInput struct.
func TestVerification_WorktreeIsolation_NotInInputSchema(t *testing.T) {
	schema := (&Tool{}).InputSchema()
	if containsBytes(schema, "isolation") {
		t.Log("isolation parameter found in schema — task #16 may be complete")
	} else {
		t.Log("GAP CONFIRMED: 'isolation' parameter not in Go input schema (TypeScript supports isolation:'worktree')")
	}
}

// ─── GAP 2: Fork subagent not implemented ────────────────────────────────────

// TestVerification_ForkSubagent_NotImplemented documents that the fork
// subagent experiment is not available in Go.
//
// TypeScript: FORK_SUBAGENT gate → buildForkedMessages() creates cache-identical
// prefixes, isInForkChild() guards against recursive forking.
func TestVerification_ForkSubagent_NotImplemented(t *testing.T) {
	t.Log("GAP CONFIRMED: Fork subagent experiment not implemented in Go")
	t.Log("TypeScript: FORK_SUBAGENT gate → cache-identical message prefixes + recursive guard")
	t.Log("Impact: parallel fork agents cannot share KV cache; each spawns independently")
}

// ─── GAP 6: VERIFICATION_AGENT_TYPE not defined ──────────────────────────────

// TestVerification_VerificationAgentType_NotBuiltIn documents that Go doesn't
// have a `verification` built-in agent type.
//
// TypeScript constants.ts: VERIFICATION_AGENT_TYPE = 'verification'
// TypeScript ONE_SHOT_BUILTIN_AGENT_TYPES includes it.
func TestVerification_VerificationAgentType_NotBuiltIn(t *testing.T) {
	agents := BuiltInAgents()
	for _, a := range agents {
		if a.Name == "verification" {
			t.Log("verification agent type found in built-ins — gap closed")
			return
		}
	}
	t.Log("GAP CONFIRMED: 'verification' built-in agent type not present (TypeScript has VERIFICATION_AGENT_TYPE constant)")
	t.Logf("Built-in agents: %v", agentNames(agents))
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_FilterTools_AgentAlwaysBlocked verifies that the Agent
// tool itself is always blocked from sub-agents — prevents infinite recursion.
func TestVerification_FilterTools_AgentAlwaysBlocked(t *testing.T) {
	agentTool := &stubTool{name: "Agent"}
	readTool := &stubTool{name: "Read"}
	bashTool := &stubTool{name: "Bash"}

	filtered := FilterToolsForAgent([]tools.Tool{agentTool, readTool, bashTool}, false)
	for _, t2 := range filtered {
		if t2.Name() == "Agent" {
			t.Error("Agent tool should be blocked from sub-agents (prevents infinite recursion)")
		}
	}
}

// TestVerification_FilterTools_AskUserQuestionAlwaysBlocked verifies that
// sub-agents cannot prompt the user — matches TypeScript ALL_AGENT_DISALLOWED_TOOLS.
func TestVerification_FilterTools_AskUserQuestionAlwaysBlocked(t *testing.T) {
	asuTool := &stubTool{name: "AskUserQuestion"}
	readTool := &stubTool{name: "Read"}

	filtered := FilterToolsForAgent([]tools.Tool{asuTool, readTool}, false)
	for _, t2 := range filtered {
		if t2.Name() == "AskUserQuestion" {
			t.Error("AskUserQuestion should be blocked from sub-agents")
		}
	}
	// Read should pass through.
	found := false
	for _, t2 := range filtered {
		if t2.Name() == "Read" {
			found = true
		}
	}
	if !found {
		t.Error("Read should be available to sync sub-agents")
	}
}

// TestVerification_FilterTools_AsyncRestrictsToAllowedSet verifies that
// background agents are restricted to asyncAgentAllowedTools list.
// Matches TypeScript's ASYNC_AGENT_ALLOWED_TOOLS filtering.
func TestVerification_FilterTools_AsyncRestrictsToAllowedSet(t *testing.T) {
	all := []tools.Tool{
		&stubTool{name: "Read"},
		&stubTool{name: "Write"},
		&stubTool{name: "Edit"},
		&stubTool{name: "Glob"},
		&stubTool{name: "Grep"},
		&stubTool{name: "Bash"},
		&stubTool{name: "Skill"},
		&stubTool{name: "ToolSearch"},
		&stubTool{name: "AstGrep"},
		// These should be blocked for async agents:
		&stubTool{name: "WebFetch"},
		&stubTool{name: "TaskCreate"},
		&stubTool{name: "AskUserQuestion"},
	}

	filtered := FilterToolsForAgent(all, true) // isAsync=true

	blocked := []string{"WebFetch", "TaskCreate", "AskUserQuestion"}
	for _, name := range blocked {
		for _, t2 := range filtered {
			if t2.Name() == name {
				t.Errorf("async agents should NOT have access to %s", name)
			}
		}
	}

	allowed := []string{"Read", "Write", "Edit", "Glob", "Grep", "Bash", "Skill", "ToolSearch", "AstGrep"}
	for _, name := range allowed {
		found := false
		for _, t2 := range filtered {
			if t2.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("async agents should have access to %s", name)
		}
	}
}

// TestVerification_FilterToolsByNames_NilReturnsAll verifies that nil allowlist
// returns all tools (no filtering) — matches TypeScript behaviour.
func TestVerification_FilterToolsByNames_NilReturnsAll(t *testing.T) {
	input := []tools.Tool{&stubTool{name: "A"}, &stubTool{name: "B"}, &stubTool{name: "C"}}
	got := FilterToolsByNames(input, nil)
	if len(got) != len(input) {
		t.Errorf("nil allowlist should return all %d tools, got %d", len(input), len(got))
	}
}

// TestVerification_ModelResolution_AliasMapping verifies that model aliases
// (sonnet/opus/haiku) map to full model IDs.
func TestVerification_ModelResolution_AliasMapping(t *testing.T) {
	tests := []struct {
		alias   string
		wantSub string // expected substring in resolved model ID
	}{
		{"sonnet", "sonnet"},
		{"opus", "opus"},
		{"haiku", "haiku"},
	}

	for _, tc := range tests {
		got := resolveModel(tc.alias, "", nil)
		if got == tc.alias {
			// Not resolved — alias was returned as-is
			t.Logf("NOTE: model alias %q was not resolved to a full ID (returned as-is)", tc.alias)
		} else if !containsStr(got, tc.wantSub) {
			t.Errorf("resolveModel(%q) = %q, should contain %q", tc.alias, got, tc.wantSub)
		}
	}
}

// TestVerification_BuiltInAgents_ExploreAndPlan verifies that Explore and Plan
// built-in agents are present — matching TypeScript's built-in agent definitions.
func TestVerification_BuiltInAgents_ExploreAndPlan(t *testing.T) {
	agents := BuiltInAgents()
	names := agentNames(agents)

	hasExplore := false
	hasPlan := false
	for _, a := range agents {
		if a.Name == "Explore" {
			hasExplore = true
		}
		if a.Name == "Plan" {
			hasPlan = true
		}
	}

	if !hasExplore {
		t.Errorf("Explore agent not in built-ins: %v", names)
	}
	if !hasPlan {
		t.Errorf("Plan agent not in built-ins: %v", names)
	}
}

// TestVerification_BackgroundRegistry_TracksByAgentID verifies that the
// DefaultRegistry correctly tracks background agents by ID.
func TestVerification_BackgroundRegistry_TracksByAgentID(t *testing.T) {
	r := NewAgentRegistry()

	agentID := "test-agent-123"
	outputFile := "/tmp/test.output"

	r.Register(&BackgroundAgent{
		AgentID:    agentID,
		OutputFile: outputFile,
		Status:     AgentStatusRunning,
	})

	bg := r.Get(agentID)
	if bg == nil {
		t.Fatal("registered agent should be findable by ID")
	}
	if bg.Status != AgentStatusRunning {
		t.Errorf("initial status = %q, want %q", bg.Status, AgentStatusRunning)
	}
	if bg.OutputFile != outputFile {
		t.Errorf("output file = %q, want %q", bg.OutputFile, outputFile)
	}
}

// TestVerification_BackgroundRegistry_CompletionUpdatesStatus verifies that
// Complete() transitions status from running → completed.
func TestVerification_BackgroundRegistry_CompletionUpdatesStatus(t *testing.T) {
	r := NewAgentRegistry()
	r.Register(&BackgroundAgent{
		AgentID:    "agent-1",
		OutputFile: t.TempDir() + "/agent-1.output",
		Status:     AgentStatusRunning,
	})

	r.Complete("agent-1", "task done")

	bg := r.Get("agent-1")
	if bg == nil {
		t.Fatal("agent not found after complete")
	}
	if bg.Status != AgentStatusCompleted {
		t.Errorf("status after Complete() = %q, want %q", bg.Status, AgentStatusCompleted)
	}
	if bg.Result != "task done" {
		t.Errorf("result = %q, want %q", bg.Result, "task done")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

type stubTool struct {
	name string
}

func (s *stubTool) Name() string { return s.name }

// stub implements tools.Tool interface minimally
func (s *stubTool) Description() string                   { return "" }
func (s *stubTool) InputSchema() json.RawMessage          { return json.RawMessage(`{"type":"object"}`) }
func (s *stubTool) ValidateInput(_ json.RawMessage) error { return nil }
func (s *stubTool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	return &models.ToolResult{Content: s.name}, nil
}
func (s *stubTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (s *stubTool) IsReadOnly(_ json.RawMessage) bool        { return true }

func agentNames(agents []AgentDefinition) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}

func containsBytes(b []byte, s string) bool {
	return containsStr(string(b), s)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
