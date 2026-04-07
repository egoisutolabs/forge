// Package askuser — verification tests comparing Go port against Claude Code's
// AskUserQuestionTool TypeScript source.
//
// GAP SUMMARY (as of 2026-04-04):
//
//  1. MISSING: `preview` field on questionOption.
//     Claude Code's TypeScript schema includes an optional `preview` string on
//     each option (ASCII or HTML mockup for side-by-side comparison UI).
//     The Go struct silently drops this field on unmarshal.
//
//  2. MISSING: `annotations` in output JSON.
//     TypeScript output: {questions, answers, annotations}.
//     Go output:         {questions, answers}.
//
//  3. FIELD NAMING PARITY: `multi_select` vs `multiSelect`.
//     TypeScript schema uses `multiSelect` (camelCase). Go schema uses
//     `multi_select` (snake_case). If the protocol is ever shared, the field
//     names must agree. Currently each side defines its own schema, so this
//     is only a concern if Claude sends camelCase to the Go server.
//
//  4. OPTION DESCRIPTION is required in Go (`required: ["label","description"]`)
//     but Claude Code allows omitting description. Overly strict validation.
//
//  5. NOT IMPLEMENTED: KAIROS channel detection (isEnabled check). Acceptable
//     for the Go port (no KAIROS infra), but documented here for parity.
package askuser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/egoisutolabs/forge/internal/tools"
)

// ─── GAP 1: preview field silently dropped ───────────────────────────────────

// TestVerification_PreviewFieldOnOption verifies that the `preview` field
// present in Claude Code's TypeScript schema is handled when provided in
// input.
//
// Claude Code source: AskUserQuestionTool.tsx — questionOptionSchema defines
//
//	preview: z.string().optional()
//
// Current Go behaviour: the field is silently ignored (not stored, not
// forwarded to the UI callback). This test documents the gap.
func TestVerification_PreviewFieldOnOption_SilentlyDropped(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Which layout?",
				"header":   "Layout",
				"options": []any{
					map[string]any{
						"label":       "Grid",
						"description": "CSS grid layout",
						"preview":     "[ col1 | col2 ]\n[ col1 | col2 ]", // ASCII mockup
					},
					map[string]any{
						"label":       "Flex",
						"description": "CSS flexbox layout",
						"preview":     "[ item1 ][ item2 ][ item3 ]",
					},
				},
			},
		},
	})

	// Validation should pass — extra fields must not break parsing.
	if err := (&Tool{}).ValidateInput(in); err != nil {
		t.Errorf("ValidateInput rejected input with preview field: %v", err)
	}

	// Execute should succeed even when preview is present.
	var receivedOpts []tools.AskQuestionOption
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		if len(qs) > 0 {
			receivedOpts = qs[0].Options
		}
		return map[string]string{"Which layout?": "Grid"}, nil
	})

	result, err := (&Tool{}).Execute(context.Background(), in, tctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Content)
	}

	// GAP: AskQuestionOption has no Preview field, so the mockup text is lost.
	// If a Preview field is added to AskQuestionOption, update this test to
	// assert that it is forwarded correctly.
	for _, opt := range receivedOpts {
		_ = opt // Preview field not present — gap confirmed.
	}
	t.Log("GAP CONFIRMED: preview field on options is silently dropped (AskQuestionOption has no Preview field)")
}

// ─── GAP 2: output missing annotations ───────────────────────────────────────

// TestVerification_OutputMissingAnnotations verifies the output JSON shape.
//
// Claude Code TypeScript output schema:
//
//	{ questions, answers, annotations }
//
// Go output schema:
//
//	{ questions, answers }
//
// The `annotations` field carries per-question user notes and preview
// selections. Its absence means callers cannot distinguish annotated from
// un-annotated answers.
func TestVerification_OutputMissingAnnotations(t *testing.T) {
	tctx := tctxWithPrompt(func(qs []tools.AskQuestion) (map[string]string, error) {
		return map[string]string{"Which approach?": "A"}, nil
	})

	result, _ := (&Tool{}).Execute(context.Background(), validInput(t), tctx)

	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// These two fields must always be present.
	if _, ok := out["questions"]; !ok {
		t.Error("output missing 'questions' field")
	}
	if _, ok := out["answers"]; !ok {
		t.Error("output missing 'answers' field")
	}

	// GAP: 'annotations' is absent. Claude Code always includes it.
	if _, ok := out["annotations"]; !ok {
		t.Log("GAP CONFIRMED: output JSON is missing 'annotations' field (present in Claude Code TypeScript)")
	}
}

// ─── GAP 3: camelCase multiSelect field ──────────────────────────────────────

// TestVerification_CamelCaseMultiSelectIgnored verifies that if a caller
// sends `multiSelect` (camelCase, as used by Claude Code's TypeScript schema)
// instead of `multi_select` (snake_case, used by Go schema), it is silently
// ignored.
//
// This is a protocol-compatibility gap: if Claude is configured with Claude
// Code's schema and sends camelCase keys, the Go server will not honour
// multi-select semantics.
func TestVerification_CamelCaseMultiSelectIgnored(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question":    "Pick features?",
				"header":      "Features",
				"multiSelect": true, // camelCase — TypeScript convention
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
		return map[string]string{"Pick features?": "Auth"}, nil
	})

	(&Tool{}).Execute(context.Background(), in, tctx) //nolint:errcheck

	if gotMultiSelect {
		t.Log("camelCase 'multiSelect' is honoured — no gap")
	} else {
		t.Log("GAP CONFIRMED: camelCase 'multiSelect' is silently ignored; Go schema requires 'multi_select'")
	}
}

// ─── GAP 4: option description required (over-strict) ────────────────────────

// TestVerification_OptionDescriptionRequired verifies that Go requires
// description on every option, while Claude Code marks it as optional.
//
// Claude Code TypeScript schema: description is present but NOT in required[].
// Go JSON schema: "required": ["label", "description"]
//
// This means Claude Code can send options without description; Go will reject
// them with a JSON unmarshal error (fields are set to zero) but actually
// validation only checks label, so let's confirm the exact behaviour.
func TestVerification_OptionDescriptionOptional(t *testing.T) {
	in := mustJSON(t, map[string]any{
		"questions": []any{
			map[string]any{
				"question": "Choose?",
				"header":   "Choice",
				"options": []any{
					map[string]any{"label": "Yes"}, // no description
					map[string]any{"label": "No"},  // no description
				},
			},
		},
	})

	// Claude Code allows this; Go validation should also allow it (label is
	// the only enforced field at the Go code level, even though the JSON
	// schema says description is required).
	err := (&Tool{}).ValidateInput(in)
	if err != nil {
		t.Logf("GAP CONFIRMED: Go validates description as required, but Claude Code marks it optional. Error: %v", err)
	} else {
		t.Log("Go correctly allows options without description (matches Claude Code behaviour)")
	}
}

// ─── Correct behaviour: parity with Claude Code ──────────────────────────────

// TestVerification_MaxFourQuestionsMatchesClaudeCode confirms Claude Code's
// documented 1-4 question limit is enforced by Go.
func TestVerification_MaxFourQuestionsMatchesClaudeCode(t *testing.T) {
	makeQ := func(id int) map[string]any {
		return map[string]any{
			"question": strings.Repeat("Q", id) + "?",
			"header":   strings.Repeat("H", id),
			"options": []any{
				map[string]any{"label": "A", "description": "desc"},
				map[string]any{"label": "B", "description": "desc"},
			},
		}
	}

	// 4 questions → valid (matches Claude Code).
	qs4 := []any{makeQ(1), makeQ(2), makeQ(3), makeQ(4)}
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"questions": qs4})); err != nil {
		t.Errorf("4 questions should be valid: %v", err)
	}

	// 5 questions → invalid (matches Claude Code maxItems: 4).
	qs5 := append(qs4, makeQ(5))
	if err := (&Tool{}).ValidateInput(mustJSON(t, map[string]any{"questions": qs5})); err == nil {
		t.Error("5 questions should be rejected")
	}
}

// TestVerification_PermAskBehaviourMatchesClaudeCode confirms the permission
// decision is PermAsk, matching Claude Code's checkPermissions which always
// returns { behavior: 'ask' }.
func TestVerification_PermAskBehaviourMatchesClaudeCode(t *testing.T) {
	decision, err := (&Tool{}).CheckPermissions(validInput(t), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Claude Code: checkPermissions → { behavior: 'ask', ... }
	const wantBehavior = "ask"
	if string(decision.Behavior) != wantBehavior {
		t.Errorf("CheckPermissions Behavior = %q, want %q (must match Claude Code)", decision.Behavior, wantBehavior)
	}
}
