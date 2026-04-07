package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInit_CreateFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	conflict, err := s.Init("my-feature", "direct")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if conflict != "" {
		t.Fatalf("unexpected conflict %q", conflict)
	}
	entry, ok := s.Features["my-feature"]
	if !ok {
		t.Fatal("feature not created")
	}
	if entry.Mode != "direct" {
		t.Errorf("mode = %q, want %q", entry.Mode, "direct")
	}
	if entry.StartedAt.IsZero() {
		t.Error("started_at should be set")
	}
	// All canonical phases should be initialized to null.
	for _, p := range PhaseOrder {
		ps, ok := entry.Phases[p]
		if !ok {
			t.Errorf("phase %q missing", p)
			continue
		}
		if ps.Status != StatusNull {
			t.Errorf("phase %q status = %q, want null", p, ps.Status)
		}
	}
	if s.Active != "my-feature" {
		t.Errorf("active = %q, want my-feature", s.Active)
	}
}

func TestInit_ConflictDetection(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	// Create first (incomplete) feature.
	_, _ = s.Init("feature-a", "direct")

	// Try to init a second feature while first is active and incomplete.
	conflict, err := s.Init("feature-b", "direct")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if conflict != "feature-a" {
		t.Errorf("conflict = %q, want feature-a", conflict)
	}
	// Active should still be feature-a.
	if s.Active != "feature-a" {
		t.Errorf("active = %q, want feature-a", s.Active)
	}
}

func TestInit_NoConflictWhenActiveComplete(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feature-a", "direct")
	// Mark all phases done.
	for _, p := range PhaseOrder {
		_ = s.SetPhase("feature-a", p, StatusDone)
	}

	// Now feature-a is complete; feature-b should succeed.
	conflict, err := s.Init("feature-b", "direct")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict %q", conflict)
	}
	if s.Active != "feature-b" {
		t.Errorf("active = %q, want feature-b", s.Active)
	}
}

func TestInit_SameSlugIdempotent(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("my-feature", "direct")
	before := s.Features["my-feature"].StartedAt

	// Re-init same slug should not recreate.
	conflict, err := s.Init("my-feature", "direct")
	if err != nil || conflict != "" {
		t.Fatalf("re-init failed: conflict=%q err=%v", conflict, err)
	}
	after := s.Features["my-feature"].StartedAt
	if !before.Equal(after) {
		t.Error("re-init should not overwrite existing entry")
	}
}

func TestSetPhaseGetPhase(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")

	if err := s.SetPhase("feat", "plan", StatusRunning); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}
	status, err := s.GetPhase("feat", "plan")
	if err != nil {
		t.Fatalf("GetPhase: %v", err)
	}
	if status != StatusRunning {
		t.Errorf("status = %q, want running", status)
	}
}

func TestSetPhase_DonePopulatesCompletedAt(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")

	before := time.Now()
	_ = s.SetPhase("feat", "plan", StatusDone)
	after := time.Now()

	ps := s.Features["feat"].Phases["plan"]
	if ps.CompletedAt == nil {
		t.Fatal("CompletedAt should be set on done")
	}
	if ps.CompletedAt.Before(before) || ps.CompletedAt.After(after) {
		t.Errorf("CompletedAt %v out of expected range [%v, %v]", ps.CompletedAt, before, after)
	}
}

func TestSetPhase_NonDoneClearsCompletedAt(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")
	_ = s.SetPhase("feat", "plan", StatusDone)
	_ = s.SetPhase("feat", "plan", StatusFail)

	ps := s.Features["feat"].Phases["plan"]
	if ps.CompletedAt != nil {
		t.Error("CompletedAt should be nil for non-done status")
	}
}

func TestSetPhase_UnknownFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	err := s.SetPhase("nonexistent", "plan", StatusRunning)
	if err == nil {
		t.Fatal("expected error for unknown feature")
	}
}

func TestGetPhase_UnknownFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, err := s.GetPhase("nonexistent", "plan")
	if err == nil {
		t.Fatal("expected error for unknown feature")
	}
}

func TestGetPhase_UnknownPhaseReturnsNull(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")

	status, err := s.GetPhase("feat", "nonexistent-phase")
	if err != nil {
		t.Fatalf("GetPhase unknown phase: %v", err)
	}
	if status != StatusNull {
		t.Errorf("status = %q, want empty (null)", status)
	}
}

func TestResume_FindsFirstNonDone(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")
	_ = s.SetPhase("feat", "plan", StatusDone)
	_ = s.SetPhase("feat", "prepare", StatusDone)
	// test is still null

	phase, err := s.Resume("feat")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if phase != "test" {
		t.Errorf("resume = %q, want test", phase)
	}
}

func TestResume_ReturnsFirstPhaseWhenAllNull(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")

	phase, err := s.Resume("feat")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if phase != "plan" {
		t.Errorf("resume = %q, want plan", phase)
	}
}

func TestResume_ReturnsEmptyWhenAllDone(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")
	for _, p := range PhaseOrder {
		_ = s.SetPhase("feat", p, StatusDone)
	}

	phase, err := s.Resume("feat")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if phase != "" {
		t.Errorf("resume = %q, want empty (complete)", phase)
	}
}

func TestResume_FailedPhaseIsResumed(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")
	_ = s.SetPhase("feat", "plan", StatusDone)
	_ = s.SetPhase("feat", "prepare", StatusFail)

	phase, err := s.Resume("feat")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if phase != "prepare" {
		t.Errorf("resume = %q, want prepare", phase)
	}
}

func TestResume_UnknownFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, err := s.Resume("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown feature")
	}
}

func TestActiveFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	if s.ActiveFeature() != "" {
		t.Error("active should be empty initially")
	}
	_, _ = s.Init("feat", "direct")
	if s.ActiveFeature() != "feat" {
		t.Errorf("active = %q, want feat", s.ActiveFeature())
	}
}

func TestRemove_ClearsActive(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat", "direct")
	s.Remove("feat")

	if _, ok := s.Features["feat"]; ok {
		t.Error("feature should be removed")
	}
	if s.Active != "" {
		t.Errorf("active = %q after remove, want empty", s.Active)
	}
}

func TestRemove_ReassignsActiveToIncomplete(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat-a", "direct")
	// Force-add feat-b without conflict.
	s.Features["feat-b"] = &FeatureEntry{
		Mode:   "direct",
		Phases: map[string]*PhaseState{"plan": {Status: StatusNull}},
	}

	s.Remove("feat-a")

	if _, ok := s.Features["feat-a"]; ok {
		t.Error("feat-a should be removed")
	}
	if s.Active != "feat-b" {
		t.Errorf("active = %q, want feat-b", s.Active)
	}
}

func TestRemove_NonActiveFeature(t *testing.T) {
	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("feat-a", "direct")
	s.Features["feat-b"] = &FeatureEntry{
		Mode:   "direct",
		Phases: map[string]*PhaseState{},
	}

	s.Remove("feat-b")
	if s.Active != "feat-a" {
		t.Errorf("active = %q, want feat-a", s.Active)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &ForgeState{Features: make(map[string]*FeatureEntry)}
	_, _ = s.Init("my-feat", "direct")
	_ = s.SetPhase("my-feat", "plan", StatusDone)

	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Active != "my-feat" {
		t.Errorf("loaded.Active = %q, want my-feat", loaded.Active)
	}
	ps := loaded.Features["my-feat"].Phases["plan"]
	if ps.Status != StatusDone {
		t.Errorf("plan status = %q, want done", ps.Status)
	}
	if ps.CompletedAt == nil {
		t.Error("plan CompletedAt should be set")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Create empty file.
	f, _ := os.Create(path)
	f.Close()

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty file: %v", err)
	}
	if s.Features == nil {
		t.Error("Features should be initialized, not nil")
	}
}

func TestLoad_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// File does not exist yet; Load should create it.
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load new file: %v", err)
	}
	if s == nil || s.Features == nil {
		t.Error("expected initialized empty state")
	}
}

func TestMigrate_LegacyPhases(t *testing.T) {
	s := &ForgeState{
		Active: "feat",
		Features: map[string]*FeatureEntry{
			"feat": {
				Mode: "direct",
				Phases: map[string]*PhaseState{
					"discover":  {Status: StatusDone},
					"explore":   {Status: StatusDone},
					"architect": {Status: StatusDone},
					"handoff":   {Status: StatusDone},
					"test":      {Status: StatusRunning},
					"implement": {Status: StatusNull},
					"verify":    {Status: StatusNull},
				},
			},
		},
	}
	migrateState(s)

	entry := s.Features["feat"]

	// Legacy phases should be removed.
	for _, legacy := range []string{"discover", "explore", "architect", "handoff"} {
		if _, ok := entry.Phases[legacy]; ok {
			t.Errorf("legacy phase %q should have been removed", legacy)
		}
	}

	// plan and prepare should be promoted to done.
	if entry.Phases["plan"].Status != StatusDone {
		t.Errorf("plan = %q, want done", entry.Phases["plan"].Status)
	}
	if entry.Phases["prepare"].Status != StatusDone {
		t.Errorf("prepare = %q, want done", entry.Phases["prepare"].Status)
	}
	// All canonical phases must exist.
	for _, p := range PhaseOrder {
		if _, ok := entry.Phases[p]; !ok {
			t.Errorf("canonical phase %q missing after migration", p)
		}
	}
}
