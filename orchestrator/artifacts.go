package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactCheck describes a validation rule for a phase output file.
// It checks that the file exists and contains the required section headers.
type ArtifactCheck struct {
	// FilePath is the artifact filename relative to .forge/features/{slug}/.
	FilePath string

	// RequiredSections is a list of markdown section headers that must appear
	// in the file (e.g. "## Requirements").
	RequiredSections []string

	// Mode controls how RequiredSections are evaluated:
	//   "all" — every section must be present (default)
	//   "any" — at least one section must be present
	Mode string
}

// Validate checks that the artifact file exists at featureDir/FilePath and
// contains all required section headers (or any, when Mode="any").
func (ac *ArtifactCheck) Validate(featureDir string) error {
	path := filepath.Join(featureDir, ac.FilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("artifact %s: file not found", ac.FilePath)
		}
		return fmt.Errorf("artifact %s: %w", ac.FilePath, err)
	}

	if len(ac.RequiredSections) == 0 {
		return nil
	}

	content := string(data)
	switch ac.Mode {
	case "any":
		for _, section := range ac.RequiredSections {
			if strings.Contains(content, section) {
				return nil
			}
		}
		return fmt.Errorf("artifact %s: none of the required sections found: %v", ac.FilePath, ac.RequiredSections)
	default: // "all"
		for _, section := range ac.RequiredSections {
			if !strings.Contains(content, section) {
				return fmt.Errorf("artifact %s: missing required section %q", ac.FilePath, section)
			}
		}
		return nil
	}
}

// PhaseArtifacts maps each phase name to its required artifact checks.
// Replaces the phase-spec bash metadata.
var PhaseArtifacts = map[string][]ArtifactCheck{
	"plan": {
		{FilePath: "discovery.md", RequiredSections: []string{"## Requirements"}, Mode: "all"},
		{FilePath: "exploration.md", RequiredSections: []string{"## Structural Patterns"}, Mode: "all"},
		{FilePath: "architecture.md", RequiredSections: []string{"## Recommendation", "## Selected Approach"}, Mode: "all"},
	},
	// prepare: checked dynamically (implementation-context.md for direct mode,
	// issues.md for github mode). No static checks here.
	"prepare": {},
	"test": {
		{FilePath: "test-manifest.md", RequiredSections: []string{"## Test File Checksums"}, Mode: "all"},
	},
	"implement": {
		{FilePath: "impl-manifest.md", RequiredSections: []string{"## Files Created", "## Test Results"}, Mode: "any"},
	},
	"verify": {
		{FilePath: "verify-report.md", RequiredSections: []string{"## Overall", "## Action Required"}, Mode: "all"},
	},
}

// ValidatePhase runs all artifact checks for phase against featureDir.
// Returns a slice of errors; empty on success.
func ValidatePhase(phase, featureDir string) []error {
	checks, ok := PhaseArtifacts[phase]
	if !ok {
		return nil
	}
	var errs []error
	for i := range checks {
		if err := checks[i].Validate(featureDir); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// ValidatePrepare checks the prepare-phase artifact for the given mode.
// Direct mode expects implementation-context.md; github mode expects issues.md.
func ValidatePrepare(featureDir, mode string) error {
	var check ArtifactCheck
	switch mode {
	case "github":
		check = ArtifactCheck{
			FilePath:         "issues.md",
			RequiredSections: []string{"## Issues"},
			Mode:             "all",
		}
	default: // "direct"
		check = ArtifactCheck{
			FilePath:         "implementation-context.md",
			RequiredSections: []string{"## Implementation Plan"},
			Mode:             "all",
		}
	}
	return check.Validate(featureDir)
}
