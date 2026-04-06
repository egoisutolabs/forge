package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/orchestrator"
	"github.com/egoisutolabs/forge/skills"
)

// =============================================================================
// State Save/Load Roundtrip
// =============================================================================

func TestStateSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	// Create and populate state.
	state := &orchestrator.ForgeState{
		Active:   "add-auth",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	conflict, err := state.Init("add-auth", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict: %q", conflict)
	}

	// Set some phases.
	if err := state.SetPhase("add-auth", "plan", orchestrator.StatusDone); err != nil {
		t.Fatal(err)
	}
	if err := state.SetPhase("add-auth", "prepare", orchestrator.StatusDone); err != nil {
		t.Fatal(err)
	}
	if err := state.SetPhase("add-auth", "test", orchestrator.StatusRunning); err != nil {
		t.Fatal(err)
	}

	// Save.
	if err := state.Save(stateFile); err != nil {
		t.Fatal(err)
	}

	// Load.
	loaded, err := orchestrator.Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Active != "add-auth" {
		t.Errorf("loaded Active = %q, want %q", loaded.Active, "add-auth")
	}

	entry, ok := loaded.Features["add-auth"]
	if !ok {
		t.Fatal("feature 'add-auth' not found in loaded state")
	}

	// Verify phase states.
	planState := entry.Phases["plan"]
	if planState == nil || planState.Status != orchestrator.StatusDone {
		t.Errorf("plan status = %v, want done", planState)
	}
	if planState.CompletedAt == nil {
		t.Error("plan CompletedAt should be set when done")
	}

	prepState := entry.Phases["prepare"]
	if prepState == nil || prepState.Status != orchestrator.StatusDone {
		t.Errorf("prepare status = %v, want done", prepState)
	}

	testState := entry.Phases["test"]
	if testState == nil || testState.Status != orchestrator.StatusRunning {
		t.Errorf("test status = %v, want running", testState)
	}
	if testState.CompletedAt != nil {
		t.Error("test CompletedAt should be nil when running")
	}

	// Verify unset phases are null.
	implState := entry.Phases["implement"]
	if implState == nil || implState.Status != orchestrator.StatusNull {
		t.Errorf("implement status = %v, want null", implState)
	}
}

// =============================================================================
// Resume from Mid-Pipeline
// =============================================================================

func TestResumeFromMidPipeline(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	state := &orchestrator.ForgeState{
		Active:   "fix-bug",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("fix-bug", "direct")

	// Simulate plan + prepare done, rest pending.
	state.SetPhase("fix-bug", "plan", orchestrator.StatusDone)
	state.SetPhase("fix-bug", "prepare", orchestrator.StatusDone)

	state.Save(stateFile)

	loaded, err := orchestrator.Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	// Resume should return "test" (first incomplete phase).
	resumeFrom, err := loaded.Resume("fix-bug")
	if err != nil {
		t.Fatal(err)
	}
	if resumeFrom != "test" {
		t.Errorf("resumeFrom = %q, want %q", resumeFrom, "test")
	}
}

func TestResumeAllDone(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "done-feature",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("done-feature", "direct")
	for _, p := range orchestrator.PhaseOrder {
		state.SetPhase("done-feature", p, orchestrator.StatusDone)
	}

	resumeFrom, err := state.Resume("done-feature")
	if err != nil {
		t.Fatal(err)
	}
	if resumeFrom != "" {
		t.Errorf("resumeFrom = %q, want empty (all done)", resumeFrom)
	}
}

func TestResumeFromPlan(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "new-feature",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("new-feature", "direct")

	resumeFrom, err := state.Resume("new-feature")
	if err != nil {
		t.Fatal(err)
	}
	if resumeFrom != "plan" {
		t.Errorf("resumeFrom = %q, want %q", resumeFrom, "plan")
	}
}

// =============================================================================
// Phase Registry
// =============================================================================

func TestPhaseRegistryCompleteness(t *testing.T) {
	expected := []string{"plan", "prepare", "test", "implement", "verify"}
	if len(orchestrator.PhaseRegistry) != len(expected) {
		t.Fatalf("registry has %d phases, want %d", len(orchestrator.PhaseRegistry), len(expected))
	}

	for i, phase := range orchestrator.PhaseRegistry {
		if phase.Name != expected[i] {
			t.Errorf("phase[%d].Name = %q, want %q", i, phase.Name, expected[i])
		}
		if phase.AgentDef == "" {
			t.Errorf("phase %q has empty AgentDef", phase.Name)
		}
		if len(phase.Artifacts) == 0 {
			t.Errorf("phase %q has no artifacts", phase.Name)
		}
	}

	// Verify gate pattern: plan=yes, prepare=yes, test=no, implement=no, verify=yes.
	gateExpected := map[string]bool{
		"plan": true, "prepare": true, "test": false, "implement": false, "verify": true,
	}
	for _, phase := range orchestrator.PhaseRegistry {
		if phase.HasGate != gateExpected[phase.Name] {
			t.Errorf("phase %q HasGate = %v, want %v", phase.Name, phase.HasGate, gateExpected[phase.Name])
		}
	}
}

func TestPhaseByName(t *testing.T) {
	phase, err := orchestrator.PhaseByName("implement")
	if err != nil {
		t.Fatal(err)
	}
	if phase.Name != "implement" {
		t.Errorf("name = %q, want %q", phase.Name, "implement")
	}
	if phase.AgentDef != "forge-implement.md" {
		t.Errorf("AgentDef = %q, want %q", phase.AgentDef, "forge-implement.md")
	}

	_, err = orchestrator.PhaseByName("nonexistent")
	if err == nil {
		t.Error("expected error for unknown phase")
	}
}

func TestNextPhase(t *testing.T) {
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
			t.Errorf("NextPhase(%q) error: %v", tt.current, err)
			continue
		}
		if next != tt.want {
			t.Errorf("NextPhase(%q) = %q, want %q", tt.current, next, tt.want)
		}
	}

	_, err := orchestrator.NextPhase("unknown")
	if err == nil {
		t.Error("expected error for unknown phase in NextPhase")
	}
}

// =============================================================================
// Artifact Validation
// =============================================================================

func TestArtifactValidationPass(t *testing.T) {
	dir := t.TempDir()

	// Create a valid discovery.md for plan phase.
	content := `# Discovery

## Requirements

- The system must support authentication.
- Users must be able to login.

## Open Questions

None.
`
	writeArtifact(t, dir, "discovery.md", content)

	check := orchestrator.ArtifactCheck{
		FilePath:         "discovery.md",
		RequiredSections: []string{"## Requirements"},
		Mode:             "all",
	}

	if err := check.Validate(dir); err != nil {
		t.Errorf("expected validation pass, got error: %v", err)
	}
}

func TestArtifactValidationMissingSection(t *testing.T) {
	dir := t.TempDir()

	content := `# Discovery

Some content without required sections.
`
	writeArtifact(t, dir, "discovery.md", content)

	check := orchestrator.ArtifactCheck{
		FilePath:         "discovery.md",
		RequiredSections: []string{"## Requirements"},
		Mode:             "all",
	}

	err := check.Validate(dir)
	if err == nil {
		t.Error("expected validation error for missing section")
	}
	if err != nil && !findSubstring(err.Error(), "missing required section") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArtifactValidationMissingFile(t *testing.T) {
	dir := t.TempDir()

	check := orchestrator.ArtifactCheck{
		FilePath:         "nonexistent.md",
		RequiredSections: []string{"## Anything"},
		Mode:             "all",
	}

	err := check.Validate(dir)
	if err == nil {
		t.Error("expected validation error for missing file")
	}
	if err != nil && !findSubstring(err.Error(), "file not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArtifactValidationModeAny(t *testing.T) {
	dir := t.TempDir()

	// architecture.md uses "any" mode for sections.
	content := `# Architecture

## Selected Approach

We'll use a microservices architecture.
`
	writeArtifact(t, dir, "architecture.md", content)

	check := orchestrator.ArtifactCheck{
		FilePath:         "architecture.md",
		RequiredSections: []string{"## Recommendation", "## Selected Approach"},
		Mode:             "any",
	}

	if err := check.Validate(dir); err != nil {
		t.Errorf("expected validation pass with mode=any, got error: %v", err)
	}
}

func TestArtifactValidationModeAnyNonePresent(t *testing.T) {
	dir := t.TempDir()

	content := `# Architecture

No matching sections here.
`
	writeArtifact(t, dir, "architecture.md", content)

	check := orchestrator.ArtifactCheck{
		FilePath:         "architecture.md",
		RequiredSections: []string{"## Recommendation", "## Selected Approach"},
		Mode:             "any",
	}

	err := check.Validate(dir)
	if err == nil {
		t.Error("expected validation error when no sections match with mode=any")
	}
	if err != nil && !findSubstring(err.Error(), "none of the required sections") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidatePhase(t *testing.T) {
	dir := t.TempDir()

	// Create valid plan artifacts.
	writeArtifact(t, dir, "discovery.md", "# Discovery\n\n## Requirements\n\nStuff.\n")
	writeArtifact(t, dir, "exploration.md", "# Exploration\n\n## Structural Patterns\n\nPatterns.\n")
	writeArtifact(t, dir, "architecture.md", "# Architecture\n\n## Recommendation\n\nUse X.\n\n## Selected Approach\n\nX.\n")

	errs := orchestrator.ValidatePhase("plan", dir)
	if len(errs) != 0 {
		t.Errorf("expected 0 validation errors, got %d: %v", len(errs), errs)
	}
}

func TestValidatePhasePlanMissingArtifacts(t *testing.T) {
	dir := t.TempDir()
	// Empty dir — all plan artifacts missing.
	errs := orchestrator.ValidatePhase("plan", dir)
	if len(errs) == 0 {
		t.Error("expected validation errors for missing plan artifacts")
	}
}

func TestValidatePrepare(t *testing.T) {
	dir := t.TempDir()

	// Direct mode: expects implementation-context.md with ## Implementation Plan.
	writeArtifact(t, dir, "implementation-context.md", "# Context\n\n## Implementation Plan\n\n1. Step one.\n")

	err := orchestrator.ValidatePrepare(dir, "direct")
	if err != nil {
		t.Errorf("expected no error for valid direct prepare, got: %v", err)
	}

	// GitHub mode: expects issues.md with ## Issues.
	dir2 := t.TempDir()
	writeArtifact(t, dir2, "issues.md", "# Issues\n\n## Issues\n\n- Issue 1.\n")

	err = orchestrator.ValidatePrepare(dir2, "github")
	if err != nil {
		t.Errorf("expected no error for valid github prepare, got: %v", err)
	}

	// Missing artifact.
	dir3 := t.TempDir()
	err = orchestrator.ValidatePrepare(dir3, "direct")
	if err == nil {
		t.Error("expected error for missing implementation-context.md")
	}
}

func writeArtifact(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// =============================================================================
// Phase Tool Filtering
// =============================================================================

func TestPhaseToolFiltering(t *testing.T) {
	// Verify plan phase tools.
	planAllowed, planDenied := orchestrator.PhaseToolsForTest("plan")
	expectContains(t, planAllowed, "Read")
	expectContains(t, planAllowed, "Glob")
	expectContains(t, planAllowed, "Grep")
	expectContains(t, planAllowed, "Bash")
	expectContains(t, planAllowed, "Browser")
	expectNotContains(t, planAllowed, "Write")
	expectNotContains(t, planAllowed, "Edit")
	expectContains(t, planDenied, "Write")
	expectContains(t, planDenied, "Edit")
	expectContains(t, planDenied, "Agent")
	expectContains(t, planDenied, "AskUserQuestion")

	// Verify implement phase tools.
	implAllowed, implDenied := orchestrator.PhaseToolsForTest("implement")
	expectContains(t, implAllowed, "Read")
	expectContains(t, implAllowed, "Write")
	expectContains(t, implAllowed, "Edit")
	expectContains(t, implAllowed, "Glob")
	expectContains(t, implAllowed, "Grep")
	expectContains(t, implAllowed, "Bash")
	expectNotContains(t, implAllowed, "Browser")
	expectContains(t, implDenied, "Browser")
	expectContains(t, implDenied, "Agent")

	// Verify verify phase tools (read-only).
	verifyAllowed, verifyDenied := orchestrator.PhaseToolsForTest("verify")
	expectContains(t, verifyAllowed, "Read")
	expectContains(t, verifyAllowed, "Glob")
	expectContains(t, verifyAllowed, "Grep")
	expectContains(t, verifyAllowed, "Bash")
	expectNotContains(t, verifyAllowed, "Write")
	expectNotContains(t, verifyAllowed, "Edit")
	expectContains(t, verifyDenied, "Write")
	expectContains(t, verifyDenied, "Edit")
}

func expectContains(t *testing.T, list []string, item string) {
	t.Helper()
	for _, s := range list {
		if s == item {
			return
		}
	}
	t.Errorf("expected list to contain %q, got: %v", item, list)
}

func expectNotContains(t *testing.T, list []string, item string) {
	t.Helper()
	for _, s := range list {
		if s == item {
			t.Errorf("expected list to NOT contain %q, got: %v", item, list)
			return
		}
	}
}

// =============================================================================
// ParsePhaseResult
// =============================================================================

func TestParsePhaseResult(t *testing.T) {
	tests := []struct {
		raw        string
		wantStatus string
		wantMsg    string
	}{
		{"done - plan ready", "done", "plan ready"},
		{"done", "done", ""},
		{"blocked - planning input required", "blocked", "planning input required"},
		{"pass", "pass", ""},
		{"fail - 3 test failures, 0 scope violations", "fail", "3 test failures, 0 scope violations"},
		{"DONE - all good", "done", "all good"},
		{"FAIL - broke something", "fail", "broke something"},
		{"Pass", "pass", ""},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			result := orchestrator.ParsePhaseResultForTest(tt.raw)
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tt.wantStatus)
			}
			if result.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", result.Message, tt.wantMsg)
			}
		})
	}
}

// =============================================================================
// State Init Conflict Detection
// =============================================================================

func TestStateInitConflict(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	// Init first feature.
	conflict, err := state.Init("feature-a", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict: %q", conflict)
	}
	if state.Active != "feature-a" {
		t.Errorf("Active = %q, want %q", state.Active, "feature-a")
	}

	// Try to init a second feature while first is incomplete.
	conflict, err = state.Init("feature-b", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != "feature-a" {
		t.Errorf("conflict = %q, want %q", conflict, "feature-a")
	}
}

func TestStateInitNoConflictWhenComplete(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("feature-a", "direct")
	// Mark all phases done.
	for _, p := range orchestrator.PhaseOrder {
		state.SetPhase("feature-a", p, orchestrator.StatusDone)
	}

	// Now starting a new feature should not conflict.
	conflict, err := state.Init("feature-b", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict: %q", conflict)
	}
	if state.Active != "feature-b" {
		t.Errorf("Active = %q, want %q", state.Active, "feature-b")
	}
}

func TestStateInitSameSlug(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("feature-a", "direct")
	// Re-init same slug should not conflict.
	conflict, err := state.Init("feature-a", "direct")
	if err != nil {
		t.Fatal(err)
	}
	if conflict != "" {
		t.Errorf("unexpected conflict: %q", conflict)
	}
}

// =============================================================================
// State Remove
// =============================================================================

func TestStateRemove(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}

	state.Init("feature-a", "direct")
	state.Init("feature-a", "direct") // no conflict with self

	// Remove it.
	state.Remove("feature-a")

	if _, ok := state.Features["feature-a"]; ok {
		t.Error("feature-a should be removed")
	}
	if state.Active != "" {
		t.Errorf("Active = %q, want empty after removing only feature", state.Active)
	}
}

// =============================================================================
// State SetPhase CompletedAt
// =============================================================================

func TestSetPhaseCompletedAt(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}
	state.Init("test-feature", "direct")

	// Setting to done should populate CompletedAt.
	before := time.Now().UTC()
	state.SetPhase("test-feature", "plan", orchestrator.StatusDone)
	after := time.Now().UTC()

	ps := state.Features["test-feature"].Phases["plan"]
	if ps.CompletedAt == nil {
		t.Fatal("CompletedAt should be set for StatusDone")
	}
	if ps.CompletedAt.Before(before) || ps.CompletedAt.After(after) {
		t.Error("CompletedAt should be between before and after timestamps")
	}

	// Setting to running should clear CompletedAt.
	state.SetPhase("test-feature", "plan", orchestrator.StatusRunning)
	ps = state.Features["test-feature"].Phases["plan"]
	if ps.CompletedAt != nil {
		t.Error("CompletedAt should be nil for StatusRunning")
	}
}

// =============================================================================
// State GetPhase
// =============================================================================

func TestGetPhase(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}
	state.Init("test-feature", "direct")

	status, err := state.GetPhase("test-feature", "plan")
	if err != nil {
		t.Fatal(err)
	}
	if status != orchestrator.StatusNull {
		t.Errorf("status = %q, want null", status)
	}

	state.SetPhase("test-feature", "plan", orchestrator.StatusDone)
	status, err = state.GetPhase("test-feature", "plan")
	if err != nil {
		t.Fatal(err)
	}
	if status != orchestrator.StatusDone {
		t.Errorf("status = %q, want done", status)
	}

	// Unknown feature.
	_, err = state.GetPhase("unknown", "plan")
	if err == nil {
		t.Error("expected error for unknown feature")
	}
}

// =============================================================================
// Skill Registration
// =============================================================================

func TestRegisterForgeSkill(t *testing.T) {
	registry := skills.NewRegistry()
	orchestrator.RegisterForgeSkill(registry)

	skill := registry.Lookup("forge")
	if skill == nil {
		t.Fatal("expected /forge skill to be registered")
	}

	if skill.Name != "forge" {
		t.Errorf("name = %q, want %q", skill.Name, "forge")
	}
	if skill.Description == "" {
		t.Error("expected non-empty description")
	}
	if !skill.UserInvocable {
		t.Error("expected skill to be user-invocable")
	}
	if skill.Context != skills.ContextInline {
		t.Errorf("context = %q, want inline", skill.Context)
	}
	if skill.Execute == nil {
		t.Error("expected Execute callback to be set")
	}
	if skill.Prompt == nil {
		t.Error("expected Prompt callback to be set")
	}

	// Test prompt fallback.
	prompt := skill.Prompt("add user authentication")
	if !findSubstring(prompt, "add user authentication") {
		t.Errorf("prompt should contain the feature description, got: %s", prompt)
	}

	// Test empty args prompt.
	emptyPrompt := skill.Prompt("")
	if !findSubstring(emptyPrompt, "provide a feature description") {
		t.Errorf("empty prompt should ask for description, got: %s", emptyPrompt)
	}
}

// =============================================================================
// PhaseOrder Constant
// =============================================================================

func TestPhaseOrderMatchesRegistry(t *testing.T) {
	if len(orchestrator.PhaseOrder) != len(orchestrator.PhaseRegistry) {
		t.Fatalf("PhaseOrder has %d entries, PhaseRegistry has %d", len(orchestrator.PhaseOrder), len(orchestrator.PhaseRegistry))
	}
	for i, name := range orchestrator.PhaseOrder {
		if orchestrator.PhaseRegistry[i].Name != name {
			t.Errorf("PhaseOrder[%d] = %q, PhaseRegistry[%d].Name = %q", i, name, i, orchestrator.PhaseRegistry[i].Name)
		}
	}
}

// =============================================================================
// State JSON Serialization
// =============================================================================

func TestStateJSONFormat(t *testing.T) {
	state := &orchestrator.ForgeState{
		Active:   "my-feature",
		Features: make(map[string]*orchestrator.FeatureEntry),
	}
	state.Init("my-feature", "direct")
	state.SetPhase("my-feature", "plan", orchestrator.StatusDone)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON and has expected structure.
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("state JSON is invalid: %v", err)
	}

	if parsed["active"] != "my-feature" {
		t.Errorf("active = %v, want %q", parsed["active"], "my-feature")
	}
	features, ok := parsed["features"].(map[string]interface{})
	if !ok {
		t.Fatal("features should be a map")
	}
	if _, ok := features["my-feature"]; !ok {
		t.Error("expected my-feature in features map")
	}
}

// =============================================================================
// PhaseArtifacts Map
// =============================================================================

func TestPhaseArtifactsMap(t *testing.T) {
	// Verify all phases have entries (even if empty).
	for _, p := range orchestrator.PhaseOrder {
		checks, ok := orchestrator.PhaseArtifacts[p]
		if !ok {
			// prepare has an empty entry, that's fine.
			if p == "prepare" {
				continue
			}
			t.Errorf("phase %q missing from PhaseArtifacts", p)
			continue
		}

		switch p {
		case "plan":
			if len(checks) != 3 {
				t.Errorf("plan should have 3 artifact checks, got %d", len(checks))
			}
		case "prepare":
			if len(checks) != 0 {
				t.Errorf("prepare should have 0 artifact checks (dynamic), got %d", len(checks))
			}
		case "test":
			if len(checks) != 1 {
				t.Errorf("test should have 1 artifact check, got %d", len(checks))
			}
		case "implement":
			if len(checks) != 1 {
				t.Errorf("implement should have 1 artifact check, got %d", len(checks))
			}
		case "verify":
			if len(checks) != 1 {
				t.Errorf("verify should have 1 artifact check, got %d", len(checks))
			}
		}
	}
}

// =============================================================================
// Legacy State Migration
// =============================================================================

func TestLegacyStateMigration(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	// Write a legacy state with old 9-phase names.
	legacyState := `{
  "active": "old-feature",
  "features": {
    "old-feature": {
      "started_at": "2025-01-01T00:00:00Z",
      "mode": "direct",
      "phases": {
        "discover": {"status": "done", "completed_at": "2025-01-01T01:00:00Z"},
        "explore": {"status": "done", "completed_at": "2025-01-01T02:00:00Z"},
        "architect": {"status": "done", "completed_at": "2025-01-01T03:00:00Z"},
        "handoff": {"status": "running"},
        "test": {"status": ""},
        "implement": {"status": ""},
        "verify": {"status": ""}
      },
      "retries": 0
    }
  }
}`
	if err := os.WriteFile(stateFile, []byte(legacyState), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := orchestrator.Load(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	entry := loaded.Features["old-feature"]
	if entry == nil {
		t.Fatal("old-feature not found")
	}

	// Legacy discover/explore/architect → plan (done wins).
	planStatus := entry.Phases["plan"]
	if planStatus == nil || planStatus.Status != orchestrator.StatusDone {
		t.Errorf("plan status after migration = %v, want done", planStatus)
	}

	// Legacy handoff → prepare (running).
	prepareStatus := entry.Phases["prepare"]
	if prepareStatus == nil || prepareStatus.Status != orchestrator.StatusRunning {
		t.Errorf("prepare status after migration = %v, want running", prepareStatus)
	}

	// Legacy names should be removed.
	for _, old := range []string{"discover", "explore", "architect", "handoff"} {
		if _, ok := entry.Phases[old]; ok {
			t.Errorf("legacy phase %q should be removed after migration", old)
		}
	}

	// All canonical phases should exist.
	for _, p := range orchestrator.PhaseOrder {
		if _, ok := entry.Phases[p]; !ok {
			t.Errorf("canonical phase %q should exist after migration", p)
		}
	}
}
