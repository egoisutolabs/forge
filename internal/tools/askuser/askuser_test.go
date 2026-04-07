package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return json.RawMessage(b)
}

func tctxWithPrompt(fn func([]tools.AskQuestion) (map[string]string, error)) *tools.ToolContext {
	return &tools.ToolContext{UserPrompt: fn}
}

func validInput(t *testing.T) json.RawMessage {
	t.Helper()
	return mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which approach?",
				"header":   "Approach",
				"options": []any{
					map[string]any{"label": "A", "description": "Option A"},
					map[string]any{"label": "B", "description": "Option B"},
				},
			},
		},
	})
}

// ─── interface compliance ─────────────────────────────────────────────────────

func TestAskUserQuestionTool_ImplementsInterface(t *testing.T) {
	var _ tools.Tool = &Tool{}
}

func TestAskUserQuestionTool_Name(t *testing.T) {
	if got := (&Tool{}).Name(); got != "AskUserQuestion" {
		t.Errorf("Name() = %q, want %q", got, "AskUserQuestion")
	}
}

func TestAskUserQuestionTool_IsConcurrencySafe(t *testing.T) {
	if !(&Tool{}).IsConcurrencySafe(nil) {
		t.Error("AskUserQuestionTool should be concurrency-safe")
	}
}

func TestAskUserQuestionTool_IsReadOnly(t *testing.T) {
	if !(&Tool{}).IsReadOnly(nil) {
		t.Error("AskUserQuestionTool should be read-only")
	}
}

// ─── ValidateInput ────────────────────────────────────────────────────────────

func TestValidateInput_MissingQuestions(t *testing.T) {
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{})); err == nil {
		t.Error("expected error for missing questions")
	}
}

func TestValidateInput_EmptyQuestions(t *testing.T) {
	in := mustJSON(t, map[string]any{"questions": []any{}})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for empty questions array")
	}
}

func TestValidateInput_TooManyQuestions(t *testing.T) {
	qs := make([]any, 5)
	for i := range qs {
		qs[i] = map[string]any{
			"question": fmt.Sprintf("Q%d?", i),
			"header":   fmt.Sprintf("H%d", i),
			"options": []any{
				map[string]any{"label": "A", "description": "opt a"},
				map[string]any{"label": "B", "description": "opt b"},
			},
		}
	}
	in := mustJSON(t, map[string]any{"questions": qs})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for >4 questions")
	}
}

func TestValidateInput_EmptyQuestionText(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "",
				"header":   "H",
				"options": []any{
					map[string]any{"label": "A", "description": "opt a"},
					map[string]any{"label": "B", "description": "opt b"},
				},
			},
		},
	})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for empty question text")
	}
}

func TestValidateInput_TooFewOptions(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Q?",
				"header":   "H",
				"options": []any{
					map[string]any{"label": "A", "description": "only one"},
				},
			},
		},
	})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for <2 options")
	}
}

func TestValidateInput_TooManyOptions(t *testing.T) {
	opts := make([]any, 5)
	for i := range opts {
		opts[i] = map[string]any{"label": fmt.Sprintf("O%d", i), "description": "desc"}
	}
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{"question": "Q?", "header": "H", "options": opts},
		},
	})
	if err := (&Tool{}).ValidateInput(in); err == nil {
		t.Error("expected error for >4 options")
	}
}

func TestValidateInput_Valid(t *testing.T) {
	if err := (&Tool{}).ValidateInput(validInput(t)); err != nil {
		t.Errorf("unexpected error for valid input: %v", err)
	}
}

// ─── CheckPermissions ─────────────────────────────────────────────────────────

func TestCheckPermissions_ReturnsAsk(t *testing.T) {
	decision, err := (&Tool{}).CheckPermissions(validInput(t), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermAsk {
		t.Errorf("Behavior = %q, want %q", decision.Behavior, models.PermAsk)
	}
	if decision.Message == "" {
		t.Error("expected non-empty permission message")
	}
}

func TestCheckPermissions_MessageContainsQuestion(t *testing.T) {
	decision, _ := (&Tool{}).CheckPermissions(validInput(t), nil)
	if !contains(decision.Message, "Which approach?") {
		t.Errorf("permission message %q should contain question text", decision.Message)
	}
}

func TestCheckPermissions_InvalidInput(t *testing.T) {
	decision, err := (&Tool{}).CheckPermissions(json.RawMessage(`{bad}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Behavior != models.PermDeny {
		t.Errorf("expected PermDeny for bad input, got %q", decision.Behavior)
	}
}

// ─── Execute ─────────────────────────────────────────────────────────────────

func TestExecute_NilToolContext(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), validInput(t), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when ToolContext is nil")
	}
	if !contains(result.Content, "user interaction not available") {
		t.Errorf("expected 'user interaction not available', got %q", result.Content)
	}
}

func TestExecute_NilUserPrompt(t *testing.T) {
	result, err := (&Tool{}).Execute(context.Background(), validInput(t), &tools.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when UserPrompt is nil")
	}
	if !contains(result.Content, "user interaction not available") {
		t.Errorf("expected 'user interaction not available', got %q", result.Content)
	}
}

func TestExecute_Success(t *testing.T) {
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		answers := make(map[string]string)
		for _, q := range qs {
			answers[q.Question] = q.Options[0].Label
		}
		return answers, nil
	})

	result, err := (&Tool{}).Execute(context.Background(), validInput(t), tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := out["answers"]; !ok {
		t.Error("output missing 'answers' field")
	}
	if _, ok := out["questions"]; !ok {
		t.Error("output missing 'questions' field")
	}
}

func TestExecute_MultipleQuestions(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Q1?",
				"header":   "H1",
				"options": []any{
					map[string]any{"label": "A", "description": "opt a"},
					map[string]any{"label": "B", "description": "opt b"},
				},
			},
			map[string]any{
				"question": "Q2?",
				"header":   "H2",
				"options": []any{
					map[string]any{"label": "X", "description": "opt x"},
					map[string]any{"label": "Y", "description": "opt y"},
				},
			},
		},
	})

	var receivedQs []tools.AskQuestion
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		receivedQs = qs
		return map[string]string{"Q1?": "A", "Q2?": "X"}, nil
	})

	result, _ := (&Tool{}).Execute(context.Background(), in, tctx)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if len(receivedQs) != 2 {
		t.Errorf("callback received %d questions, want 2", len(receivedQs))
	}
}

func TestExecute_UserPromptError(t *testing.T) {
	tctx := tctxWithPrompt(func(_ []tools.AskQuestion) (map[string]string, error) {
		return nil, fmt.Errorf("interaction cancelled")
	})

	result, _ := (&Tool{}).Execute(context.Background(), validInput(t), tctx)
	if !result.IsError {
		t.Error("expected IsError=true when UserPrompt returns error")
	}
}

func TestExecute_MultiSelectPassedToCallback(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question":     "Pick features?",
				"header":       "Features",
				"multi_select": true,
				"options": []any{
					map[string]any{"label": "Auth", "description": "Authentication"},
					map[string]any{"label": "Logs", "description": "Logging"},
				},
			},
		},
	})

	var gotMultiSelect bool
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		if len(qs) > 0 {
			gotMultiSelect = qs[0].MultiSelect
		}
		return map[string]string{"Pick features?": "Auth,Logs"}, nil
	})

	(&Tool{}).Execute(context.Background(), in, tctx) //nolint:errcheck
	if !gotMultiSelect {
		t.Error("expected MultiSelect=true to be forwarded to callback")
	}
}

// ─── Preview field ────────────────────────────────────────────────────────────

func TestExecute_PreviewFieldRoundTrip(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which approach?",
				"header":   "Approach",
				"options": []any{
					map[string]any{"label": "A", "description": "Option A", "preview": "Preview text A"},
					map[string]any{"label": "B", "description": "Option B"},
				},
			},
		},
	})

	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		return map[string]string{"Which approach?": "A"}, nil
	})

	result, err := (&Tool{}).Execute(context.Background(), in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	// Output should contain the preview field.
	if !contains(result.Content, "Preview text A") {
		t.Errorf("output should contain preview text, got: %s", result.Content)
	}
}

func TestValidateInput_WithPreview(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Q?",
				"header":   "H",
				"options": []any{
					map[string]any{"label": "A", "description": "opt a", "preview": "see this"},
					map[string]any{"label": "B", "description": "opt b"},
				},
			},
		},
	})
	if err := (&Tool{}).ValidateInput(in); err != nil {
		t.Errorf("preview field should be accepted: %v", err)
	}
}

// ─── Annotations output ───────────────────────────────────────────────────────

func TestExecute_AnnotationsInOutput(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which approach?",
				"header":   "Approach",
				"options": []any{
					map[string]any{"label": "A", "description": "Option A"},
					map[string]any{"label": "B", "description": "Option B"},
				},
			},
		},
		"annotations": map[string]string{
			"context": "refactor session",
			"source":  "engine",
		},
	})

	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		return map[string]string{"Which approach?": "A"}, nil
	})

	result, err := (&Tool{}).Execute(context.Background(), in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	annotations, ok := out["annotations"]
	if !ok {
		t.Fatal("output missing 'annotations' field")
	}
	annMap, ok := annotations.(map[string]any)
	if !ok {
		t.Fatalf("annotations should be a map, got %T", annotations)
	}
	if annMap["context"] != "refactor session" {
		t.Errorf("annotations.context = %q, want %q", annMap["context"], "refactor session")
	}
}

func TestExecute_NoAnnotationsOmitted(t *testing.T) {
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		return map[string]string{"Which approach?": "A"}, nil
	})

	result, _ := (&Tool{}).Execute(context.Background(), validInput(t), tctx)
	var out map[string]any
	json.Unmarshal([]byte(result.Content), &out) //nolint:errcheck
	if _, ok := out["annotations"]; ok {
		t.Error("annotations field should be omitted when not provided")
	}
}

func TestInputSchema_ContainsPreview(t *testing.T) {
	schema := (&Tool{}).InputSchema()
	if !contains(string(schema), "preview") {
		t.Error("InputSchema should include 'preview' field")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

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
