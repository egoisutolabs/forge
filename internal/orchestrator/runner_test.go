package orchestrator_test

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/orchestrator"
	"github.com/egoisutolabs/forge/internal/orchestrator/agents"
)

// ---- parsePhaseResult -------------------------------------------------------

func TestParsePhaseResult_StatusNormalization(t *testing.T) {
	cases := []struct {
		raw        string
		wantStatus string
		wantMsg    string
	}{
		{"done - plan ready", "done", "plan ready"},
		{"done - prepare ready", "done", "prepare ready"},
		{"done - wrote 3 test files with 12 test cases", "done", "wrote 3 test files with 12 test cases"},
		{"blocked - planning input required", "blocked", "planning input required"},
		{"pass", "pass", ""},
		{"fail - 3 test failures, 0 scope violations, 0 structural violations", "fail", "3 test failures, 0 scope violations, 0 structural violations"},
		{"DONE - uppercase", "done", "uppercase"},
		{"Pass", "pass", ""},
		{"BLOCKED - something", "blocked", "something"},
		{"FAIL - oops", "fail", "oops"},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			result := orchestrator.ParsePhaseResultForTest(c.raw)
			if result.Status != c.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, c.wantStatus)
			}
			if result.Message != c.wantMsg {
				t.Errorf("Message = %q, want %q", result.Message, c.wantMsg)
			}
			if result.Raw != c.raw {
				t.Errorf("Raw = %q, want %q", result.Raw, c.raw)
			}
		})
	}
}

func TestParsePhaseResult_EmptyInput(t *testing.T) {
	result := orchestrator.ParsePhaseResultForTest("")
	if result.Status != "" {
		t.Errorf("empty input: Status = %q, want empty", result.Status)
	}
	if result.Raw != "" {
		t.Errorf("empty input: Raw = %q, want empty", result.Raw)
	}
}

// ---- buildSystemPrompt (via BuildSystemPrompt) ------------------------------

func TestBuildSystemPrompt_ContainsAgentContent(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	runner := &orchestrator.PhaseRunner{
		Model:     "claude-sonnet-4-6",
		AgentDefs: defs,
	}

	phase, err := orchestrator.PhaseByName("plan")
	if err != nil {
		t.Fatalf("PhaseByName: %v", err)
	}

	prompt, err := runner.BuildSystemPrompt(phase)
	if err != nil {
		t.Fatalf("BuildSystemPrompt: %v", err)
	}
	if prompt == "" {
		t.Fatal("BuildSystemPrompt returned empty string")
	}
	// common-rules content injected via {{contracts.common-rules}} placeholder.
	if !strings.Contains(prompt, "Artifact Discipline") {
		t.Error("system prompt missing 'Artifact Discipline' from common-rules contract")
	}
	// Agent body present.
	if !strings.Contains(prompt, "Forge Plan") {
		t.Error("system prompt missing 'Forge Plan' agent heading")
	}
}

func TestBuildSystemPrompt_AllPhases(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents: %v", err)
	}
	runner := &orchestrator.PhaseRunner{
		Model:     "claude-sonnet-4-6",
		AgentDefs: defs,
	}

	for _, phase := range orchestrator.PhaseRegistry {
		t.Run(phase.Name, func(t *testing.T) {
			prompt, err := runner.BuildSystemPrompt(phase)
			if err != nil {
				t.Fatalf("BuildSystemPrompt(%s): %v", phase.Name, err)
			}
			if prompt == "" {
				t.Errorf("BuildSystemPrompt(%s): empty prompt", phase.Name)
			}
			if strings.Contains(prompt, "{{contracts.") {
				t.Errorf("BuildSystemPrompt(%s): unresolved {{contracts.*}} in prompt", phase.Name)
			}
		})
	}
}

func TestBuildSystemPrompt_UnknownAgent(t *testing.T) {
	defs, _ := agents.LoadAgents()
	runner := &orchestrator.PhaseRunner{AgentDefs: defs}
	phase := orchestrator.Phase{Name: "unknown", AgentDef: "no-such-agent.md"}
	_, err := runner.BuildSystemPrompt(phase)
	if err == nil {
		t.Error("BuildSystemPrompt with unknown agent should return error")
	}
}

// ---- tool filtering per phase -----------------------------------------------

func TestPhaseToolFiltering(t *testing.T) {
	cases := []struct {
		phase   string
		allowed []string
		denied  []string
	}{
		{
			phase:   "plan",
			allowed: []string{"Read", "Glob", "AstGrep", "Grep", "Bash", "Browser", "WebFetch", "WebSearch"},
			denied:  []string{"Write", "Edit", "Agent", "AskUserQuestion"},
		},
		{
			phase:   "prepare",
			allowed: []string{"Read", "Write", "Glob", "AstGrep", "Grep", "Bash", "Browser", "WebFetch", "WebSearch"},
			denied:  []string{"Edit", "Agent"},
		},
		{
			phase:   "test",
			allowed: []string{"Read", "Write", "Glob", "AstGrep", "Grep", "Bash"},
			denied:  []string{"Edit", "WebFetch", "Agent"},
		},
		{
			phase:   "implement",
			allowed: []string{"Read", "Write", "Edit", "Glob", "AstGrep", "Grep", "Bash"},
			denied:  []string{"WebFetch", "WebSearch", "Agent"},
		},
		{
			phase:   "verify",
			allowed: []string{"Read", "Glob", "AstGrep", "Grep", "Bash"},
			denied:  []string{"Write", "Edit", "WebFetch", "Agent"},
		},
	}

	for _, c := range cases {
		t.Run(c.phase, func(t *testing.T) {
			allowed, denied := orchestrator.PhaseToolsForTest(c.phase)
			allowSet := make(map[string]bool, len(allowed))
			for _, n := range allowed {
				allowSet[n] = true
			}
			denySet := make(map[string]bool, len(denied))
			for _, n := range denied {
				denySet[n] = true
			}

			for _, name := range c.allowed {
				if !allowSet[name] {
					t.Errorf("tool %q should be allowed in phase %s", name, c.phase)
				}
			}
			for _, name := range c.denied {
				if allowSet[name] {
					t.Errorf("tool %q should NOT be allowed in phase %s", name, c.phase)
				}
				// Confirm it's in the deny set.
				if !denySet[name] {
					t.Errorf("tool %q should appear in denied set for phase %s", name, c.phase)
				}
			}
		})
	}
}

func TestPhaseToolFiltering_UnknownPhase(t *testing.T) {
	allowed, denied := orchestrator.PhaseToolsForTest("unknown-phase")
	// Unknown phase: should allow nothing (returns nil allowed).
	if len(allowed) != 0 {
		t.Errorf("unknown phase: allowed = %v, want empty", allowed)
	}
	// Denied should include all known tools.
	if len(denied) == 0 {
		t.Error("unknown phase: denied should be non-empty")
	}
}
