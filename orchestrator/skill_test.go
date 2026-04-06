package orchestrator

import (
	"errors"
	"testing"

	"github.com/egoisutolabs/forge/tools"
)

func TestForgeAskUserAdapter_NilToolContext(t *testing.T) {
	adapter := forgeAskUserAdapter(nil)
	if adapter != nil {
		t.Error("adapter should be nil for nil ToolContext")
	}
}

func TestForgeAskUserAdapter_NilUserPrompt(t *testing.T) {
	tctx := &tools.ToolContext{}
	adapter := forgeAskUserAdapter(tctx)
	if adapter != nil {
		t.Error("adapter should be nil when UserPrompt is nil")
	}
}

func TestForgeAskUserAdapter_ForwardsCallCorrectly(t *testing.T) {
	var capturedQuestions []tools.AskQuestion

	tctx := &tools.ToolContext{
		UserPrompt: func(questions []tools.AskQuestion) (map[string]string, error) {
			capturedQuestions = questions
			return map[string]string{questions[0].Question: "approved"}, nil
		},
	}

	adapter := forgeAskUserAdapter(tctx)
	if adapter == nil {
		t.Fatal("adapter should not be nil when UserPrompt is set")
	}

	answer, err := adapter("Summary text", "Do you approve?", []string{"yes", "no"})
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if answer != "approved" {
		t.Errorf("answer = %q, want approved", answer)
	}

	// Verify the question was constructed correctly.
	if len(capturedQuestions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(capturedQuestions))
	}
	q := capturedQuestions[0]
	if q.Question != "Do you approve?" {
		t.Errorf("question = %q", q.Question)
	}
	if q.Header != "Summary text" {
		t.Errorf("header = %q", q.Header)
	}
	if len(q.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(q.Options))
	}
	if q.Options[0].Label != "yes" || q.Options[1].Label != "no" {
		t.Errorf("options = %v", q.Options)
	}
}

func TestForgeAskUserAdapter_NoOptions(t *testing.T) {
	tctx := &tools.ToolContext{
		UserPrompt: func(questions []tools.AskQuestion) (map[string]string, error) {
			if len(questions[0].Options) != 0 {
				t.Errorf("expected no options, got %d", len(questions[0].Options))
			}
			return map[string]string{questions[0].Question: "freeform"}, nil
		},
	}

	adapter := forgeAskUserAdapter(tctx)
	answer, err := adapter("Summary", "Any thoughts?", nil)
	if err != nil {
		t.Fatalf("adapter error: %v", err)
	}
	if answer != "freeform" {
		t.Errorf("answer = %q, want freeform", answer)
	}
}

func TestForgeAskUserAdapter_PropagatesError(t *testing.T) {
	tctx := &tools.ToolContext{
		UserPrompt: func(_ []tools.AskQuestion) (map[string]string, error) {
			return nil, errors.New("user cancelled")
		},
	}

	adapter := forgeAskUserAdapter(tctx)
	_, err := adapter("Summary", "Question?", []string{"ok"})
	if err == nil {
		t.Fatal("expected error from adapter")
	}
	if err.Error() != "user cancelled" {
		t.Errorf("error = %q", err.Error())
	}
}
