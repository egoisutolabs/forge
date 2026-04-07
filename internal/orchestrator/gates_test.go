package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mockAsk is a test helper that returns the first option unconditionally,
// recording the last call for assertions.
func mockAsk(wantOption string) func(summary, question string, options []string) (string, error) {
	return func(summary, question string, options []string) (string, error) {
		for _, opt := range options {
			if opt == wantOption {
				return opt, nil
			}
		}
		return options[0], nil
	}
}

func mockAskError(e error) func(string, string, []string) (string, error) {
	return func(string, string, []string) (string, error) {
		return "", e
	}
}

func newGates(t *testing.T, ask func(string, string, []string) (string, error)) (*UserGates, string) {
	t.Helper()
	dir := t.TempDir()
	return &UserGates{FeatureDir: dir, Ask: ask}, dir
}

// TestAskStartup_Proceed confirms that answering "yes" returns true.
func TestAskStartup_Proceed(t *testing.T) {
	g, _ := newGates(t, mockAsk("yes"))
	ok, err := g.AskStartup("add user authentication")
	if err != nil {
		t.Fatalf("AskStartup: %v", err)
	}
	if !ok {
		t.Error("expected true when user answers yes")
	}
}

// TestAskStartup_Decline confirms that answering "no" returns false.
func TestAskStartup_Decline(t *testing.T) {
	g, _ := newGates(t, mockAsk("no"))
	ok, err := g.AskStartup("add user authentication")
	if err != nil {
		t.Fatalf("AskStartup: %v", err)
	}
	if ok {
		t.Error("expected false when user answers no")
	}
}

// TestAskStartup_SummaryContainsFeatureDesc checks that the feature description
// is included in the summary shown to the user.
func TestAskStartup_SummaryContainsFeatureDesc(t *testing.T) {
	var capturedSummary string
	g, _ := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "yes", nil
	})
	_, _ = g.AskStartup("add OAuth support")
	if !contains(capturedSummary, "add OAuth support") {
		t.Errorf("summary %q does not contain feature description", capturedSummary)
	}
}

// TestAskStartup_AskError propagates callback errors.
func TestAskStartup_AskError(t *testing.T) {
	g, _ := newGates(t, mockAskError(errors.New("cancelled")))
	_, err := g.AskStartup("feat")
	if err == nil {
		t.Fatal("expected error from Ask callback")
	}
}

// TestAskPlanQuestions_WithOpenQuestions verifies that questions are surfaced.
func TestAskPlanQuestions_WithOpenQuestions(t *testing.T) {
	var capturedSummary string
	g, dir := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "answered", nil
	})
	content := "# Discovery\n\n## Requirements\nfoo\n\n## Open Questions\n1. What is the scope?\n2. Which DB?\n"
	writeFile(t, dir, "discovery.md", content)

	result, err := g.AskPlanQuestions()
	if err != nil {
		t.Fatalf("AskPlanQuestions: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result when questions present")
	}
	if !contains(capturedSummary, "What is the scope?") {
		t.Errorf("summary %q should contain the open questions", capturedSummary)
	}
}

// TestAskPlanQuestions_NoQuestions returns nil when discovery.md has no Open Questions.
func TestAskPlanQuestions_NoQuestions(t *testing.T) {
	g, dir := newGates(t, mockAsk("answered"))
	writeFile(t, dir, "discovery.md", "# Discovery\n\n## Requirements\nrequirements here\n")

	result, err := g.AskPlanQuestions()
	if err != nil {
		t.Fatalf("AskPlanQuestions: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil when no open questions, got %v", result)
	}
}

// TestAskPlanQuestions_MissingFile returns an error.
func TestAskPlanQuestions_MissingFile(t *testing.T) {
	g, _ := newGates(t, mockAsk("answered"))
	_, err := g.AskPlanQuestions()
	if err == nil {
		t.Fatal("expected error when discovery.md missing")
	}
}

// TestAskArchitectureChoice_Approve tests the approve path.
func TestAskArchitectureChoice_Approve(t *testing.T) {
	g, dir := newGates(t, mockAsk("approve"))
	writeFile(t, dir, "architecture.md", "## Recommendation\nUse option A\n\n## Selected Approach\nA\n")

	choice, err := g.AskArchitectureChoice()
	if err != nil {
		t.Fatalf("AskArchitectureChoice: %v", err)
	}
	if choice != "approve" {
		t.Errorf("choice = %q, want approve", choice)
	}
}

// TestAskArchitectureChoice_SummaryContainsRecommendation verifies that the
// recommendation section drives the summary text shown to the user.
func TestAskArchitectureChoice_SummaryContainsRecommendation(t *testing.T) {
	var capturedSummary string
	g, dir := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "approve", nil
	})
	writeFile(t, dir, "architecture.md", "## Recommendation\nChoose microservices\n")

	_, _ = g.AskArchitectureChoice()
	if !contains(capturedSummary, "Choose microservices") {
		t.Errorf("summary %q should contain recommendation text", capturedSummary)
	}
}

// TestAskArchitectureChoice_MissingFile returns an error.
func TestAskArchitectureChoice_MissingFile(t *testing.T) {
	g, _ := newGates(t, mockAsk("approve"))
	_, err := g.AskArchitectureChoice()
	if err == nil {
		t.Fatal("expected error when architecture.md missing")
	}
}

// TestSummarizePrepare_DirectMode reads implementation-context.md.
func TestSummarizePrepare_DirectMode(t *testing.T) {
	var capturedSummary string
	g, dir := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "proceed", nil
	})
	writeFile(t, dir, "implementation-context.md", "## Implementation Plan\nStep 1: do X\nStep 2: do Y\n")

	choice, err := g.SummarizePrepare()
	if err != nil {
		t.Fatalf("SummarizePrepare: %v", err)
	}
	if choice != "proceed" {
		t.Errorf("choice = %q, want proceed", choice)
	}
	if !contains(capturedSummary, "Step 1") {
		t.Errorf("summary %q should contain plan content", capturedSummary)
	}
}

// TestSummarizePrepare_FallsBackToIssuesMd when direct file is absent.
func TestSummarizePrepare_FallsBackToIssuesMd(t *testing.T) {
	g, dir := newGates(t, mockAsk("proceed"))
	writeFile(t, dir, "issues.md", "## Issues\n- issue 1\n- issue 2\n")

	choice, err := g.SummarizePrepare()
	if err != nil {
		t.Fatalf("SummarizePrepare fallback: %v", err)
	}
	if choice != "proceed" {
		t.Errorf("choice = %q, want proceed", choice)
	}
}

// TestSummarizePrepare_BothMissing returns an error.
func TestSummarizePrepare_BothMissing(t *testing.T) {
	g, _ := newGates(t, mockAsk("proceed"))
	_, err := g.SummarizePrepare()
	if err == nil {
		t.Fatal("expected error when both prepare artifacts missing")
	}
}

// TestSummarizeVerifyPass_Accept returns true.
func TestSummarizeVerifyPass_Accept(t *testing.T) {
	g, dir := newGates(t, mockAsk("accept"))
	writeFile(t, dir, "verify-report.md", "## Overall\nAll checks pass\n\n## Action Required\nnone\n")

	ok, err := g.SummarizeVerifyPass()
	if err != nil {
		t.Fatalf("SummarizeVerifyPass: %v", err)
	}
	if !ok {
		t.Error("expected true when user accepts")
	}
}

// TestSummarizeVerifyPass_Revise returns false.
func TestSummarizeVerifyPass_Revise(t *testing.T) {
	g, dir := newGates(t, mockAsk("revise"))
	writeFile(t, dir, "verify-report.md", "## Overall\nPassed\n\n## Action Required\nnone\n")

	ok, err := g.SummarizeVerifyPass()
	if err != nil {
		t.Fatalf("SummarizeVerifyPass: %v", err)
	}
	if ok {
		t.Error("expected false when user selects revise")
	}
}

// TestSummarizeVerifyFail_Retry returns true.
func TestSummarizeVerifyFail_Retry(t *testing.T) {
	g, dir := newGates(t, mockAsk("retry"))
	writeFile(t, dir, "verify-report.md", "## Overall\nFailed\n\n## Action Required\nFix test X\n")

	retry, err := g.SummarizeVerifyFail()
	if err != nil {
		t.Fatalf("SummarizeVerifyFail: %v", err)
	}
	if !retry {
		t.Error("expected true when user selects retry")
	}
}

// TestSummarizeVerifyFail_ActionRequiredInSummary checks the section is shown.
func TestSummarizeVerifyFail_ActionRequiredInSummary(t *testing.T) {
	var capturedSummary string
	g, dir := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "stop", nil
	})
	writeFile(t, dir, "verify-report.md", "## Overall\nFailed\n\n## Action Required\nFix the broken import\n")

	_, _ = g.SummarizeVerifyFail()
	if !contains(capturedSummary, "Fix the broken import") {
		t.Errorf("summary %q should contain action required text", capturedSummary)
	}
}

// TestSummarizeVerifyFail_MissingFile returns an error.
func TestSummarizeVerifyFail_MissingFile(t *testing.T) {
	g, _ := newGates(t, mockAsk("retry"))
	_, err := g.SummarizeVerifyFail()
	if err == nil {
		t.Fatal("expected error when verify-report.md missing")
	}
}

// TestAskRetryOrStop_Retry returns true.
func TestAskRetryOrStop_Retry(t *testing.T) {
	g, _ := newGates(t, mockAsk("retry"))
	ok, err := g.AskRetryOrStop("agent returned fail status")
	if err != nil {
		t.Fatalf("AskRetryOrStop: %v", err)
	}
	if !ok {
		t.Error("expected true for retry")
	}
}

// TestAskRetryOrStop_Stop returns false.
func TestAskRetryOrStop_Stop(t *testing.T) {
	g, _ := newGates(t, mockAsk("stop"))
	ok, err := g.AskRetryOrStop("agent returned fail status")
	if err != nil {
		t.Fatalf("AskRetryOrStop: %v", err)
	}
	if ok {
		t.Error("expected false for stop")
	}
}

// TestExtractSection_Found extracts text between header and next ##.
func TestExtractSection_Found(t *testing.T) {
	content := "# Title\n\n## Requirements\nReq A\nReq B\n\n## Constraints\nCon C\n"
	got := extractSection(content, "## Requirements")
	if !contains(got, "Req A") || !contains(got, "Req B") {
		t.Errorf("extractSection = %q, want Req A and Req B", got)
	}
	if contains(got, "Con C") {
		t.Errorf("extractSection should not include next section content")
	}
}

// TestExtractSection_LastSection extracts until end of file.
func TestExtractSection_LastSection(t *testing.T) {
	content := "## Only Section\nContent here\n"
	got := extractSection(content, "## Only Section")
	if !contains(got, "Content here") {
		t.Errorf("extractSection last = %q, want 'Content here'", got)
	}
}

// TestExtractSection_NotFound returns empty string.
func TestExtractSection_NotFound(t *testing.T) {
	got := extractSection("# No sections here\n", "## Missing")
	if got != "" {
		t.Errorf("extractSection not found = %q, want empty", got)
	}
}

// TestSummarizeVerifyPass_SummaryContainsOverall checks the Overall section drives the summary.
func TestSummarizeVerifyPass_SummaryContainsOverall(t *testing.T) {
	var capturedSummary string
	g, dir := newGates(t, func(summary, _ string, _ []string) (string, error) {
		capturedSummary = summary
		return "accept", nil
	})
	writeFile(t, dir, "verify-report.md", "## Overall\nAll 42 tests pass\n\n## Action Required\nnone\n")

	_, _ = g.SummarizeVerifyPass()
	if !contains(capturedSummary, "All 42 tests pass") {
		t.Errorf("summary %q should contain Overall section text", capturedSummary)
	}
}

// writeFile writes content to dir/name — defined in artifacts_test.go.
// contains is a convenience helper.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// ensure writeFile is available from artifacts_test.go (same package).
var _ = os.WriteFile
var _ = filepath.Join
