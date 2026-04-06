package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactCheck_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	ac := ArtifactCheck{FilePath: "missing.md", RequiredSections: []string{"## Foo"}, Mode: "all"}
	err := ac.Validate(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestArtifactCheck_NoRequiredSections(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "artifact.md", "some content")
	ac := ArtifactCheck{FilePath: "artifact.md", RequiredSections: nil, Mode: "all"}
	if err := ac.Validate(dir); err != nil {
		t.Errorf("no required sections should always pass: %v", err)
	}
}

func TestArtifactCheck_ModeAll_AllPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Title\n\n## Requirements\nfoo\n\n## Constraints\nbar\n")
	ac := ArtifactCheck{
		FilePath:         "doc.md",
		RequiredSections: []string{"## Requirements", "## Constraints"},
		Mode:             "all",
	}
	if err := ac.Validate(dir); err != nil {
		t.Errorf("all sections present should pass: %v", err)
	}
}

func TestArtifactCheck_ModeAll_OneMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Title\n\n## Requirements\nfoo\n")
	ac := ArtifactCheck{
		FilePath:         "doc.md",
		RequiredSections: []string{"## Requirements", "## Constraints"},
		Mode:             "all",
	}
	err := ac.Validate(dir)
	if err == nil {
		t.Fatal("missing section should fail with mode=all")
	}
}

func TestArtifactCheck_ModeAny_AtLeastOne(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "## Files Created\nsome files\n")
	ac := ArtifactCheck{
		FilePath:         "doc.md",
		RequiredSections: []string{"## Files Created", "## Test Results"},
		Mode:             "any",
	}
	if err := ac.Validate(dir); err != nil {
		t.Errorf("at least one section present should pass with mode=any: %v", err)
	}
}

func TestArtifactCheck_ModeAny_NonePresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Some other content\n")
	ac := ArtifactCheck{
		FilePath:         "doc.md",
		RequiredSections: []string{"## Files Created", "## Test Results"},
		Mode:             "any",
	}
	err := ac.Validate(dir)
	if err == nil {
		t.Fatal("no sections present should fail with mode=any")
	}
}

func TestArtifactCheck_DefaultModeIsAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "## Requirements\nfoo\n")
	// Mode="" should behave like "all".
	ac := ArtifactCheck{
		FilePath:         "doc.md",
		RequiredSections: []string{"## Requirements", "## Missing"},
		Mode:             "",
	}
	err := ac.Validate(dir)
	if err == nil {
		t.Fatal("empty mode should default to all — missing section should fail")
	}
}

func TestValidatePhase_PlanSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "discovery.md", "## Requirements\nreqs\n")
	writeFile(t, dir, "exploration.md", "## Structural Patterns\npatterns\n")
	writeFile(t, dir, "architecture.md", "## Recommendation\nrec\n\n## Selected Approach\napproach\n")

	errs := ValidatePhase("plan", dir)
	if len(errs) != 0 {
		t.Errorf("plan validation should pass: %v", errs)
	}
}

func TestValidatePhase_PlanMissingArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "discovery.md", "## Requirements\nreqs\n")
	// exploration.md and architecture.md missing

	errs := ValidatePhase("plan", dir)
	if len(errs) == 0 {
		t.Fatal("should have errors for missing plan artifacts")
	}
}

func TestValidatePhase_TestPhase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "test-manifest.md", "## Test File Checksums\nsha256 abc123\n")

	errs := ValidatePhase("test", dir)
	if len(errs) != 0 {
		t.Errorf("test validation should pass: %v", errs)
	}
}

func TestValidatePhase_ImplementModeAny_FilesCreated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "impl-manifest.md", "## Files Created\nfoo.go\n")

	errs := ValidatePhase("implement", dir)
	if len(errs) != 0 {
		t.Errorf("implement validation (Files Created) should pass: %v", errs)
	}
}

func TestValidatePhase_ImplementModeAny_TestResults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "impl-manifest.md", "## Test Results\nall pass\n")

	errs := ValidatePhase("implement", dir)
	if len(errs) != 0 {
		t.Errorf("implement validation (Test Results) should pass: %v", errs)
	}
}

func TestValidatePhase_ImplementModeAny_NonePresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "impl-manifest.md", "# Other content\n")

	errs := ValidatePhase("implement", dir)
	if len(errs) == 0 {
		t.Fatal("implement validation should fail when no required section found")
	}
}

func TestValidatePhase_Verify(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "verify-report.md", "## Overall\npass\n\n## Action Required\nnone\n")

	errs := ValidatePhase("verify", dir)
	if len(errs) != 0 {
		t.Errorf("verify validation should pass: %v", errs)
	}
}

func TestValidatePhase_UnknownPhase(t *testing.T) {
	errs := ValidatePhase("nonexistent", t.TempDir())
	if len(errs) != 0 {
		t.Errorf("unknown phase should return no errors (no checks): %v", errs)
	}
}

func TestValidatePrepare_DirectMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "implementation-context.md", "## Implementation Plan\nplan\n")
	if err := ValidatePrepare(dir, "direct"); err != nil {
		t.Errorf("prepare direct mode should pass: %v", err)
	}
}

func TestValidatePrepare_DirectMode_Missing(t *testing.T) {
	dir := t.TempDir()
	if err := ValidatePrepare(dir, "direct"); err == nil {
		t.Fatal("prepare direct mode should fail when file missing")
	}
}

func TestValidatePrepare_GithubMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "issues.md", "## Issues\n- issue 1\n")
	if err := ValidatePrepare(dir, "github"); err != nil {
		t.Errorf("prepare github mode should pass: %v", err)
	}
}

// writeFile is a test helper that creates a file with given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}
