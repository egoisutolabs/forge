// Package askuser implements AskUserQuestionTool — the Go port of Claude Code's
// AskUserQuestionTool. It presents the user with 1-4 multiple-choice questions
// and returns their answers.
package askuser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
)

// questionOption mirrors the JSON input shape for a single option.
type questionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// question mirrors the JSON input shape for a single question.
type question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []questionOption `json:"options"`
	MultiSelect bool             `json:"multi_select,omitempty"`
}

// toolInput is the JSON schema for AskUserQuestionTool input.
type toolInput struct {
	Questions   []question        `json:"questions"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Tool implements AskUserQuestionTool — ask the user multiple-choice questions.
//
// This is the Go port of Claude Code's AskUserQuestionTool. Key behaviors:
//   - 1–4 questions, each with 2–4 options
//   - Requires ToolContext.UserPrompt callback for user interaction
//   - Multi-select answers are comma-separated
//   - Permission: PermAsk (preview message shown; user must approve)
//   - Concurrency: safe (read-only)
type Tool struct{}

func (t *Tool) Name() string { return "AskUserQuestion" }
func (t *Tool) Description() string {
	return "Ask the user one or more multiple-choice questions and collect their answers."
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"questions": {
				"type": "array",
				"description": "Questions to ask the user (1-4 questions)",
				"minItems": 1,
				"maxItems": 4,
				"items": {
					"type": "object",
					"properties": {
						"question": {
							"type": "string",
							"description": "The question to ask the user"
						},
						"header": {
							"type": "string",
							"description": "Short label displayed as a chip/tag (max ~20 chars)"
						},
						"options": {
							"type": "array",
							"description": "The available choices (2-4 options)",
							"minItems": 2,
							"maxItems": 4,
							"items": {
								"type": "object",
								"properties": {
									"label": {
										"type": "string",
										"description": "Display text for this option"
									},
									"description": {
										"type": "string",
										"description": "Explanation of what this option means"
									},
									"preview": {
										"type": "string",
										"description": "Optional preview text shown alongside the option"
									}
								},
								"required": ["label", "description"]
							}
						},
						"multi_select": {
							"type": "boolean",
							"default": false,
							"description": "Allow the user to select multiple options"
						}
					},
					"required": ["question", "header", "options"]
				}
			}
		},
		"required": ["questions"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if len(in.Questions) < 1 || len(in.Questions) > 4 {
		return fmt.Errorf("questions must have 1-4 items, got %d", len(in.Questions))
	}
	for i, q := range in.Questions {
		if strings.TrimSpace(q.Question) == "" {
			return fmt.Errorf("question[%d].question is required and cannot be empty", i)
		}
		if strings.TrimSpace(q.Header) == "" {
			return fmt.Errorf("question[%d].header is required and cannot be empty", i)
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return fmt.Errorf("question[%d].options must have 2-4 items, got %d", i, len(q.Options))
		}
		for j, o := range q.Options {
			if strings.TrimSpace(o.Label) == "" {
				return fmt.Errorf("question[%d].options[%d].label is required", i, j)
			}
		}
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}

	parts := make([]string, len(in.Questions))
	for i, q := range in.Questions {
		parts[i] = q.Question
	}
	msg := "ask user: " + strings.Join(parts, "; ")
	return &models.PermissionDecision{Behavior: models.PermAsk, Message: msg}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(_ context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	if tctx == nil || tctx.UserPrompt == nil {
		return &models.ToolResult{Content: "user interaction not available", IsError: true}, nil
	}

	// Convert to tools.AskQuestion for the callback.
	questions := make([]tools.AskQuestion, len(in.Questions))
	for i, q := range in.Questions {
		opts := make([]tools.AskQuestionOption, len(q.Options))
		for j, o := range q.Options {
			opts[j] = tools.AskQuestionOption{Label: o.Label, Description: o.Description}
		}
		questions[i] = tools.AskQuestion{
			Question:    q.Question,
			Header:      q.Header,
			Options:     opts,
			MultiSelect: q.MultiSelect,
		}
	}

	answers, err := tctx.UserPrompt(questions)
	if err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("user prompt error: %s", err), IsError: true}, nil
	}

	type output struct {
		Questions   []question        `json:"questions"`
		Answers     map[string]string `json:"answers"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}
	out := output{Questions: in.Questions, Answers: answers, Annotations: in.Annotations}
	data, _ := json.Marshal(out)
	return &models.ToolResult{Content: string(data)}, nil
}
