package contracts_test

import (
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/orchestrator/contracts"
)

func TestContractFor_KnownContracts(t *testing.T) {
	names := []string{
		"common-rules",
		"planning-artifacts",
		"execution-artifacts",
		"exploration-contract",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			content := contracts.ContractFor(name)
			if content == "" {
				t.Errorf("ContractFor(%q) returned empty string", name)
			}
		})
	}
}

func TestContractFor_Missing(t *testing.T) {
	content := contracts.ContractFor("nonexistent-contract")
	if content != "" {
		t.Errorf("ContractFor(nonexistent) should return empty, got %q", content)
	}
}

func TestContractFor_CommonRulesContent(t *testing.T) {
	content := contracts.ContractFor("common-rules")
	if !strings.Contains(content, "Artifact Discipline") {
		t.Error("common-rules should contain 'Artifact Discipline'")
	}
	if !strings.Contains(content, "Response Discipline") {
		t.Error("common-rules should contain 'Response Discipline'")
	}
}

func TestAll(t *testing.T) {
	all := contracts.All()
	if len(all) < 4 {
		t.Errorf("All() returned %d contracts, want at least 4", len(all))
	}
	expected := []string{"common-rules", "planning-artifacts", "execution-artifacts", "exploration-contract"}
	for _, name := range expected {
		if _, ok := all[name]; !ok {
			t.Errorf("All() missing contract %q", name)
		}
	}
}
