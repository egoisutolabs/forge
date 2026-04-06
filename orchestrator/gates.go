package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UserGates provides human checkpoint methods for the forge pipeline.
// Each method reads relevant artifacts from FeatureDir, formats a summary,
// and calls Ask to get the user's decision.
//
// The Ask callback signature mirrors tools.ToolContext.UserPrompt at a higher
// abstraction: the orchestrator is responsible for wiring this to the TUI or
// AskUserQuestionTool.
type UserGates struct {
	// FeatureDir is the .forge/features/{slug}/ directory.
	FeatureDir string

	// Ask is called for every user interaction. It receives a summary (shown
	// as context), a question, and the available option labels.
	// Returns the selected option label, or an error.
	Ask func(summary, question string, options []string) (string, error)
}

// AskStartup presents the feature description and asks whether to proceed.
// Returns true when the user confirms.
func (g *UserGates) AskStartup(featureDesc string) (bool, error) {
	summary := fmt.Sprintf("Forge will plan, prepare, test, implement, and verify:\n\n  %s", featureDesc)
	answer, err := g.Ask(summary, "Ready to begin this feature?", []string{"yes", "no"})
	if err != nil {
		return false, err
	}
	return answer == "yes", nil
}

// AskPlanQuestions reads discovery.md and presents any open questions to the
// user. Returns the raw answer text, or nil map when no questions are present.
func (g *UserGates) AskPlanQuestions() (map[string]string, error) {
	content, err := g.readArtifact("discovery.md")
	if err != nil {
		return nil, fmt.Errorf("gates: plan questions: %w", err)
	}

	questions := extractSection(content, "## Open Questions")
	if strings.TrimSpace(questions) == "" {
		return nil, nil
	}

	answer, err := g.Ask(
		questions,
		"The plan phase raised design questions. Please provide your answers:",
		[]string{"answered", "skip"},
	)
	if err != nil {
		return nil, err
	}
	return map[string]string{"response": answer}, nil
}

// AskArchitectureChoice reads architecture.md and asks the user to approve
// or request a revision. Returns the user's choice ("approve" or "revise").
func (g *UserGates) AskArchitectureChoice() (string, error) {
	content, err := g.readArtifact("architecture.md")
	if err != nil {
		return "", fmt.Errorf("gates: architecture choice: %w", err)
	}

	recommendation := extractSection(content, "## Recommendation")
	if recommendation == "" {
		recommendation = content
	}

	return g.Ask(
		recommendation,
		"Approve this architecture and proceed to implementation planning?",
		[]string{"approve", "revise"},
	)
}

// SummarizePrepare reads the prepare-phase artifact and asks the user to
// confirm before moving to the test phase.
// Returns the user's choice ("proceed" or "revise").
func (g *UserGates) SummarizePrepare() (string, error) {
	// Try direct mode artifact first, fall back to github mode.
	content, err := g.readArtifact("implementation-context.md")
	if err != nil {
		content, err = g.readArtifact("issues.md")
		if err != nil {
			return "", fmt.Errorf("gates: summarize prepare: no prepare artifact found")
		}
	}

	plan := extractSection(content, "## Implementation Plan")
	if plan == "" {
		// Fall back to first N lines of the file.
		lines := strings.SplitN(content, "\n", 30)
		plan = strings.Join(lines, "\n")
	}

	return g.Ask(
		plan,
		"Implementation plan is ready. Proceed to writing tests?",
		[]string{"proceed", "revise"},
	)
}

// SummarizeVerifyPass reads verify-report.md and asks whether the user
// accepts the result. Returns true when the user accepts.
func (g *UserGates) SummarizeVerifyPass() (bool, error) {
	content, err := g.readArtifact("verify-report.md")
	if err != nil {
		return false, fmt.Errorf("gates: verify pass: %w", err)
	}

	overall := extractSection(content, "## Overall")
	if overall == "" {
		overall = content
	}

	answer, err := g.Ask(
		overall,
		"Verification passed. Accept and mark this feature complete?",
		[]string{"accept", "revise"},
	)
	if err != nil {
		return false, err
	}
	return answer == "accept", nil
}

// SummarizeVerifyFail reads verify-report.md and asks whether to retry.
// Returns true when the user wants to retry implement+verify.
func (g *UserGates) SummarizeVerifyFail() (bool, error) {
	content, err := g.readArtifact("verify-report.md")
	if err != nil {
		return false, fmt.Errorf("gates: verify fail: %w", err)
	}

	actionRequired := extractSection(content, "## Action Required")
	if actionRequired == "" {
		actionRequired = content
	}

	answer, err := g.Ask(
		actionRequired,
		"Verification failed. Retry the implement+verify cycle?",
		[]string{"retry", "stop"},
	)
	if err != nil {
		return false, err
	}
	return answer == "retry", nil
}

// AskRetryOrStop presents a failure reason and asks the user whether to
// retry the current phase or stop. Returns true to retry.
func (g *UserGates) AskRetryOrStop(reason string) (bool, error) {
	answer, err := g.Ask(
		reason,
		"Retry this phase?",
		[]string{"retry", "stop"},
	)
	if err != nil {
		return false, err
	}
	return answer == "retry", nil
}

// readArtifact reads a file relative to FeatureDir.
func (g *UserGates) readArtifact(name string) (string, error) {
	path := filepath.Join(g.FeatureDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return string(data), nil
}

// extractSection extracts the text body following a markdown section header
// up to (but not including) the next ## header.
func extractSection(content, header string) string {
	idx := strings.Index(content, header)
	if idx < 0 {
		return ""
	}
	body := content[idx+len(header):]
	// Find the next top-level (##) section.
	if next := strings.Index(body, "\n## "); next >= 0 {
		body = body[:next]
	}
	return strings.TrimSpace(body)
}
