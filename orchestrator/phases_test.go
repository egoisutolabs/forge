package orchestrator

import "testing"

func TestPhaseRegistry_Order(t *testing.T) {
	want := []string{"plan", "prepare", "test", "implement", "verify"}
	if len(PhaseRegistry) != len(want) {
		t.Fatalf("registry length = %d, want %d", len(PhaseRegistry), len(want))
	}
	for i, name := range want {
		if PhaseRegistry[i].Name != name {
			t.Errorf("phase[%d] = %q, want %q", i, PhaseRegistry[i].Name, name)
		}
	}
}

func TestPhaseRegistry_AgentDefs(t *testing.T) {
	for _, p := range PhaseRegistry {
		if p.AgentDef == "" {
			t.Errorf("phase %q has empty AgentDef", p.Name)
		}
	}
}

func TestPhaseRegistry_Artifacts(t *testing.T) {
	for _, p := range PhaseRegistry {
		if len(p.Artifacts) == 0 {
			t.Errorf("phase %q has no artifacts", p.Name)
		}
	}
}

func TestPhaseRegistry_Gates(t *testing.T) {
	// plan, prepare, verify have gates; test and implement do not.
	wantGates := map[string]bool{
		"plan":      true,
		"prepare":   true,
		"test":      false,
		"implement": false,
		"verify":    true,
	}
	for _, p := range PhaseRegistry {
		want, ok := wantGates[p.Name]
		if !ok {
			t.Errorf("unexpected phase %q in registry", p.Name)
			continue
		}
		if p.HasGate != want {
			t.Errorf("phase %q HasGate = %v, want %v", p.Name, p.HasGate, want)
		}
	}
}

func TestPhaseByName_Found(t *testing.T) {
	p, err := PhaseByName("test")
	if err != nil {
		t.Fatalf("PhaseByName: %v", err)
	}
	if p.Name != "test" {
		t.Errorf("name = %q, want test", p.Name)
	}
	if p.AgentDef != "forge-test.md" {
		t.Errorf("agent_def = %q, want forge-test.md", p.AgentDef)
	}
}

func TestPhaseByName_NotFound(t *testing.T) {
	_, err := PhaseByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown phase")
	}
}

func TestNextPhase_Normal(t *testing.T) {
	cases := []struct {
		current string
		want    string
	}{
		{"plan", "prepare"},
		{"prepare", "test"},
		{"test", "implement"},
		{"implement", "verify"},
	}
	for _, tc := range cases {
		next, err := NextPhase(tc.current)
		if err != nil {
			t.Errorf("NextPhase(%q): %v", tc.current, err)
			continue
		}
		if next != tc.want {
			t.Errorf("NextPhase(%q) = %q, want %q", tc.current, next, tc.want)
		}
	}
}

func TestNextPhase_LastPhase(t *testing.T) {
	next, err := NextPhase("verify")
	if err != nil {
		t.Fatalf("NextPhase(verify): %v", err)
	}
	if next != "" {
		t.Errorf("NextPhase(verify) = %q, want empty", next)
	}
}

func TestNextPhase_UnknownPhase(t *testing.T) {
	_, err := NextPhase("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown phase")
	}
}

func TestPhaseRegistryConsistentWithPhaseOrder(t *testing.T) {
	// PhaseOrder and PhaseRegistry must list phases in the same order.
	if len(PhaseOrder) != len(PhaseRegistry) {
		t.Fatalf("PhaseOrder len=%d, PhaseRegistry len=%d — mismatch", len(PhaseOrder), len(PhaseRegistry))
	}
	for i, name := range PhaseOrder {
		if PhaseRegistry[i].Name != name {
			t.Errorf("PhaseOrder[%d]=%q but PhaseRegistry[%d].Name=%q", i, name, i, PhaseRegistry[i].Name)
		}
	}
}
