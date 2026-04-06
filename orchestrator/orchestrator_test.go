package orchestrator_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/orchestrator"
)

// ---- ForgeOrchestrator.New --------------------------------------------------

func TestNew_CreatesForgeDir(t *testing.T) {
	dir := t.TempDir()
	_, err := orchestrator.New(orchestrator.Config{
		Cwd:   dir,
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	info, statErr := os.Stat(filepath.Join(dir, ".forge"))
	if statErr != nil {
		t.Fatalf(".forge/ not created: %v", statErr)
	}
	if !info.IsDir() {
		t.Error(".forge is not a directory")
	}
}

func TestNew_LoadsExistingState(t *testing.T) {
	dir := t.TempDir()
	forgeDir := filepath.Join(dir, ".forge")
	_ = os.MkdirAll(forgeDir, 0o755)

	// Pre-populate a state file.
	state := newTestState()
	_, _ = state.Init("existing-slug", "direct")
	_ = state.SetPhase("existing-slug", "plan", orchestrator.StatusDone)
	stateFile := filepath.Join(forgeDir, "state.json")
	if err := state.Save(stateFile); err != nil {
		t.Fatalf("state.Save: %v", err)
	}

	o, err := orchestrator.New(orchestrator.Config{Cwd: dir, Model: "test"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if o == nil {
		t.Fatal("New() returned nil orchestrator")
	}
}

// ---- PhaseRegistry export ---------------------------------------------------

func TestPhaseRegistry_Order(t *testing.T) {
	expected := []string{"plan", "prepare", "test", "implement", "verify"}
	if len(orchestrator.PhaseRegistry) != len(expected) {
		t.Fatalf("PhaseRegistry len = %d, want %d", len(orchestrator.PhaseRegistry), len(expected))
	}
	for i, phase := range orchestrator.PhaseRegistry {
		if phase.Name != expected[i] {
			t.Errorf("PhaseRegistry[%d].Name = %q, want %q", i, phase.Name, expected[i])
		}
	}
}

func TestPhaseRegistry_AgentDefFields(t *testing.T) {
	for _, phase := range orchestrator.PhaseRegistry {
		if phase.AgentDef == "" {
			t.Errorf("phase %q: AgentDef is empty", phase.Name)
		}
	}
}

func TestPhaseByName_AllKnown(t *testing.T) {
	for _, name := range []string{"plan", "prepare", "test", "implement", "verify"} {
		p, err := orchestrator.PhaseByName(name)
		if err != nil {
			t.Errorf("PhaseByName(%q) unexpected error: %v", name, err)
			continue
		}
		if p.Name != name {
			t.Errorf("PhaseByName(%q).Name = %q", name, p.Name)
		}
	}
}

func TestPhaseByName_NotFound(t *testing.T) {
	_, err := orchestrator.PhaseByName("nonexistent")
	if err == nil {
		t.Error("PhaseByName(nonexistent) should return error")
	}
}

// ---- ForgeOrchestrator construction -----------------------------------------

func TestNew_NilCallerStillConstructs(t *testing.T) {
	dir := t.TempDir()
	o, err := orchestrator.New(orchestrator.Config{
		Cwd:   dir,
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if o == nil {
		t.Fatal("New() returned nil")
	}
}

// ---- Run with bad feature desc ----------------------------------------------

func TestRun_EmptySlugFromDesc_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	o, err := orchestrator.New(orchestrator.Config{
		Cwd:   dir,
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	// All special chars → empty slug → bootstrap error.
	err = o.Run(context.Background(), "!@#$%^&*()")
	if err == nil {
		t.Error("Run with empty-slug description should return error")
	}
}

// ---- Run with conflict -------------------------------------------------------

func TestRun_ConflictActiveFeature_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	o, err := orchestrator.New(orchestrator.Config{
		Cwd:   dir,
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	// Manually seed state with an active, incomplete feature.
	forgeDir := filepath.Join(dir, ".forge")
	_ = os.MkdirAll(forgeDir, 0o755)
	state := newTestState()
	_, _ = state.Init("first-feature", "direct")
	_ = state.Save(filepath.Join(forgeDir, "state.json"))

	// Create a second orchestrator that loads this state.
	o2, err := orchestrator.New(orchestrator.Config{Cwd: dir, Model: "test"})
	if err != nil {
		t.Fatalf("New() for conflict test: %v", err)
	}
	// Running a different feature should fail.
	err = o2.Run(context.Background(), "second feature that conflicts")
	if err == nil {
		t.Error("Run with conflicting active feature should return error")
	}
	_ = o
}
