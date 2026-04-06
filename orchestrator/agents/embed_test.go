package agents_test

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/orchestrator/agents"
)

func TestLoadAgents_Count(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	if len(defs) != 5 {
		t.Errorf("LoadAgents() = %d agents, want 5", len(defs))
	}
}

func TestLoadAgents_ExpectedNames(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	want := []string{"forge-plan", "forge-prepare", "forge-test", "forge-implement", "forge-verify"}
	got := make(map[string]bool, len(defs))
	for _, d := range defs {
		got[d.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("LoadAgents() missing agent %q", name)
		}
	}
}

func TestLoadAgents_ContractsResolved(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	for _, d := range defs {
		if strings.Contains(d.Prompt, "{{contracts.") {
			t.Errorf("agent %q: unresolved {{contracts.*}} placeholder in prompt", d.Name)
		}
		if d.Prompt == "" {
			t.Errorf("agent %q: empty prompt", d.Name)
		}
	}
}

func TestLoadAgents_PromptsContainContractContent(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	// Every agent should have the common-rules content embedded.
	for _, d := range defs {
		if !strings.Contains(d.Prompt, "Artifact Discipline") {
			t.Errorf("agent %q: prompt missing common-rules content", d.Name)
		}
	}
}

func TestLoadAgents_ModelDefault(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	for _, d := range defs {
		if d.Model == "" {
			t.Errorf("agent %q: empty model (should default to 'inherit')", d.Name)
		}
	}
}

func TestFindAgent_Found(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	found := agents.FindAgent(defs, "forge-plan")
	if found == nil {
		t.Fatal("FindAgent(forge-plan) = nil, want non-nil")
	}
	if found.Name != "forge-plan" {
		t.Errorf("FindAgent returned name %q, want forge-plan", found.Name)
	}
}

func TestFindAgent_CaseInsensitive(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	found := agents.FindAgent(defs, "FORGE-PLAN")
	if found == nil {
		t.Fatal("FindAgent(FORGE-PLAN) = nil, want case-insensitive match")
	}
}

func TestFindAgent_NotFound(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	found := agents.FindAgent(defs, "nonexistent-agent")
	if found != nil {
		t.Errorf("FindAgent(nonexistent) = %v, want nil", found)
	}
}

func TestFindAgent_DescriptionPopulated(t *testing.T) {
	defs, err := agents.LoadAgents()
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}
	for _, d := range defs {
		if d.Description == "" {
			t.Errorf("agent %q: empty description", d.Name)
		}
	}
}
