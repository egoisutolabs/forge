package orchestrator

import "fmt"

// Phase describes a single stage in the forge 5-phase pipeline.
type Phase struct {
	// Name is the canonical phase identifier (e.g. "plan").
	Name string

	// AgentDef is the filename of the embedded agent markdown definition
	// (e.g. "forge-plan.md").
	AgentDef string

	// Artifacts lists the primary output files produced by this phase,
	// relative to .forge/features/{slug}/.
	Artifacts []string

	// HasGate indicates whether this phase ends with a user checkpoint
	// before the next phase begins.
	HasGate bool
}

// PhaseRegistry is the ordered list of all forge pipeline phases.
// The order here is the canonical execution order.
var PhaseRegistry = []Phase{
	{
		Name:      "plan",
		AgentDef:  "forge-plan.md",
		Artifacts: []string{"discovery.md", "exploration.md", "architecture.md"},
		HasGate:   true,
	},
	{
		Name:      "prepare",
		AgentDef:  "forge-prepare.md",
		Artifacts: []string{"implementation-context.md"},
		HasGate:   true,
	},
	{
		Name:      "test",
		AgentDef:  "forge-test.md",
		Artifacts: []string{"test-manifest.md"},
		HasGate:   false,
	},
	{
		Name:      "implement",
		AgentDef:  "forge-implement.md",
		Artifacts: []string{"impl-manifest.md"},
		HasGate:   false,
	},
	{
		Name:      "verify",
		AgentDef:  "forge-verify.md",
		Artifacts: []string{"verify-report.md"},
		HasGate:   true,
	},
}

// PhaseByName returns the Phase with the given name.
func PhaseByName(name string) (Phase, error) {
	for _, p := range PhaseRegistry {
		if p.Name == name {
			return p, nil
		}
	}
	return Phase{}, fmt.Errorf("phases: unknown phase %q", name)
}

// NextPhase returns the name of the phase that follows current in the registry.
// Returns ("", nil) when current is the last phase.
// Returns an error when current is not a known phase name.
func NextPhase(current string) (string, error) {
	for i, p := range PhaseRegistry {
		if p.Name == current {
			if i+1 < len(PhaseRegistry) {
				return PhaseRegistry[i+1].Name, nil
			}
			return "", nil
		}
	}
	return "", fmt.Errorf("phases: unknown phase %q", current)
}
