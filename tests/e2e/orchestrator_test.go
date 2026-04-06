package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/orchestrator"
	"github.com/egoisutolabs/forge/skills"
)

// TestOrchestrator_ForgeSkillRegistered verifies that /forge is in BundledRegistry after registration.
func TestOrchestrator_ForgeSkillRegistered(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	s := registry.Lookup("forge")
	if s == nil {
		t.Fatal("forge skill not registered")
	}
	if s.Name != "forge" {
		t.Errorf("skill name = %q, want forge", s.Name)
	}
	if !s.UserInvocable {
		t.Error("forge skill should be user-invocable")
	}
	if s.Source != "bundled" {
		t.Errorf("forge skill source = %q, want bundled", s.Source)
	}
	if s.Description == "" {
		t.Error("forge skill should have a description")
	}
	if s.Prompt == nil {
		t.Error("forge skill should have a Prompt func")
	}
	if s.Execute == nil {
		t.Error("forge skill should have an Execute func")
	}

	// Prompt should provide usage hint when args are empty.
	prompt := s.Prompt("")
	if prompt == "" {
		t.Error("forge skill Prompt('') should return usage hint")
	}

	// Prompt with args should include the feature description.
	prompt = s.Prompt("add user auth")
	if prompt == "" {
		t.Error("forge skill Prompt('add user auth') should return non-empty")
	}
}

// TestOrchestrator_BundledRegistryContainsCommitAndReview verifies built-in skills.
func TestOrchestrator_BundledRegistryContainsCommitAndReview(t *testing.T) {
	reg := skills.BundledRegistry()

	for _, name := range []string{"commit", "review"} {
		s := reg.Lookup(name)
		if s == nil {
			t.Errorf("BundledRegistry missing %q skill", name)
			continue
		}
		if !s.UserInvocable {
			t.Errorf("%q skill should be user-invocable", name)
		}
	}
}

// TestOrchestrator_ForgeState_CreateSaveLoad verifies state lifecycle in temp dir.
func TestOrchestrator_ForgeState_CreateSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Load from non-existent file → empty state.
	state, err := orchestrator.Load(statePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Features) != 0 {
		t.Errorf("new state has %d features, want 0", len(state.Features))
	}
	if state.ActiveFeature() != "" {
		t.Errorf("new state active = %q, want empty", state.ActiveFeature())
	}

	// Init a feature.
	conflict, err := state.Init("auth-feature", "direct")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict: %q", conflict)
	}
	if state.ActiveFeature() != "auth-feature" {
		t.Errorf("active = %q, want auth-feature", state.ActiveFeature())
	}

	// Verify all phases start as StatusNull.
	for _, phase := range orchestrator.PhaseOrder {
		status, err := state.GetPhase("auth-feature", phase)
		if err != nil {
			t.Fatalf("GetPhase(%s): %v", phase, err)
		}
		if status != orchestrator.StatusNull {
			t.Errorf("phase %s = %q, want empty (StatusNull)", phase, status)
		}
	}

	// Set plan phase to done.
	if err := state.SetPhase("auth-feature", "plan", orchestrator.StatusDone); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	// Save to disk.
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// Reload and verify persistence.
	loaded, err := orchestrator.Load(statePath)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if loaded.ActiveFeature() != "auth-feature" {
		t.Errorf("loaded active = %q, want auth-feature", loaded.ActiveFeature())
	}
	status, _ := loaded.GetPhase("auth-feature", "plan")
	if status != orchestrator.StatusDone {
		t.Errorf("loaded plan status = %q, want done", status)
	}

	// Resume should return "prepare" (first incomplete phase).
	next, err := loaded.Resume("auth-feature")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if next != "prepare" {
		t.Errorf("Resume = %q, want prepare", next)
	}
}

// TestOrchestrator_PhaseOrdering verifies the canonical 5-phase order.
func TestOrchestrator_PhaseOrdering(t *testing.T) {
	expected := []string{"plan", "prepare", "test", "implement", "verify"}
	if len(orchestrator.PhaseOrder) != len(expected) {
		t.Fatalf("PhaseOrder length = %d, want %d", len(orchestrator.PhaseOrder), len(expected))
	}
	for i, p := range expected {
		if orchestrator.PhaseOrder[i] != p {
			t.Errorf("PhaseOrder[%d] = %q, want %q", i, orchestrator.PhaseOrder[i], p)
		}
	}
}

// TestOrchestrator_PhaseRegistry verifies PhaseRegistry matches PhaseOrder.
func TestOrchestrator_PhaseRegistry(t *testing.T) {
	for _, phaseName := range orchestrator.PhaseOrder {
		phase, err := orchestrator.PhaseByName(phaseName)
		if err != nil {
			t.Errorf("PhaseByName(%q): %v", phaseName, err)
			continue
		}
		if phase.Name != phaseName {
			t.Errorf("phase.Name = %q, want %q", phase.Name, phaseName)
		}
		if phase.AgentDef == "" {
			t.Errorf("phase %q has empty AgentDef", phaseName)
		}
	}

	// Unknown phase.
	_, err := orchestrator.PhaseByName("nonexistent")
	if err == nil {
		t.Error("PhaseByName(nonexistent) should error")
	}
}

// TestOrchestrator_NextPhase verifies phase sequencing.
func TestOrchestrator_NextPhase(t *testing.T) {
	tests := []struct {
		current string
		want    string
	}{
		{"plan", "prepare"},
		{"prepare", "test"},
		{"test", "implement"},
		{"implement", "verify"},
		{"verify", ""},
	}

	for _, tt := range tests {
		next, err := orchestrator.NextPhase(tt.current)
		if err != nil {
			t.Errorf("NextPhase(%q): %v", tt.current, err)
			continue
		}
		if next != tt.want {
			t.Errorf("NextPhase(%q) = %q, want %q", tt.current, next, tt.want)
		}
	}

	// Unknown phase.
	_, err := orchestrator.NextPhase("nonexistent")
	if err == nil {
		t.Error("NextPhase(nonexistent) should error")
	}
}

// TestOrchestrator_ArtifactCheck_AllSections verifies "all" mode requires every section.
func TestOrchestrator_ArtifactCheck_AllSections(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a discovery.md with the required section.
	content := "# Discovery\n\n## Requirements\n\nThe app needs auth.\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "discovery.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	check := orchestrator.ArtifactCheck{
		FilePath:         "discovery.md",
		RequiredSections: []string{"## Requirements"},
		Mode:             "all",
	}

	if err := check.Validate(tmpDir); err != nil {
		t.Errorf("valid artifact should pass: %v", err)
	}

	// Missing section should fail.
	check2 := orchestrator.ArtifactCheck{
		FilePath:         "discovery.md",
		RequiredSections: []string{"## Requirements", "## Nonexistent"},
		Mode:             "all",
	}
	if err := check2.Validate(tmpDir); err == nil {
		t.Error("missing section should fail validation")
	}
}

// TestOrchestrator_ArtifactCheck_AnySections verifies "any" mode requires at least one section.
func TestOrchestrator_ArtifactCheck_AnySections(t *testing.T) {
	tmpDir := t.TempDir()

	content := "# Impl\n\n## Files Created\n\n- main.go\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "impl-manifest.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	check := orchestrator.ArtifactCheck{
		FilePath:         "impl-manifest.md",
		RequiredSections: []string{"## Files Created", "## Test Results"},
		Mode:             "any",
	}

	if err := check.Validate(tmpDir); err != nil {
		t.Errorf("'any' with one matching section should pass: %v", err)
	}

	// None matching should fail.
	check2 := orchestrator.ArtifactCheck{
		FilePath:         "impl-manifest.md",
		RequiredSections: []string{"## Nonexistent A", "## Nonexistent B"},
		Mode:             "any",
	}
	if err := check2.Validate(tmpDir); err == nil {
		t.Error("'any' with no matching section should fail")
	}
}

// TestOrchestrator_ArtifactCheck_MissingFile verifies missing artifact files are caught.
func TestOrchestrator_ArtifactCheck_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	check := orchestrator.ArtifactCheck{
		FilePath:         "nonexistent.md",
		RequiredSections: []string{"## Anything"},
		Mode:             "all",
	}

	err := check.Validate(tmpDir)
	if err == nil {
		t.Error("missing file should fail validation")
	}
}

// TestOrchestrator_ValidatePhase verifies the full phase validation pipeline.
func TestOrchestrator_ValidatePhase(t *testing.T) {
	tmpDir := t.TempDir()

	// Empty dir should fail plan validation (missing discovery.md etc.).
	errs := orchestrator.ValidatePhase("plan", tmpDir)
	if len(errs) == 0 {
		t.Error("plan validation should fail on empty dir")
	}

	// Unknown phase should return no errors (not registered).
	errs = orchestrator.ValidatePhase("nonexistent", tmpDir)
	if len(errs) != 0 {
		t.Errorf("unknown phase should return empty errors, got %d", len(errs))
	}
}

// TestOrchestrator_StateConflict verifies that Init returns conflict when another feature is active.
func TestOrchestrator_StateConflict(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state, _ := orchestrator.Load(statePath)
	state.Init("feature-a", "direct")

	// feature-a is active and incomplete, so starting feature-b should return conflict.
	conflict, err := state.Init("feature-b", "direct")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if conflict != "feature-a" {
		t.Errorf("conflict = %q, want feature-a", conflict)
	}
}

// TestOrchestrator_StateRemove verifies feature removal.
func TestOrchestrator_StateRemove(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state, _ := orchestrator.Load(statePath)
	state.Init("to-remove", "direct")

	state.Remove("to-remove")
	if state.ActiveFeature() != "" {
		t.Errorf("active should be empty after removal, got %q", state.ActiveFeature())
	}
	if _, ok := state.Features["to-remove"]; ok {
		t.Error("feature should be removed from map")
	}
}
