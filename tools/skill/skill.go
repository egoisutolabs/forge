// Package skill implements SkillTool — the Go port of Claude Code's SkillTool.
// It looks up a named skill from the SkillRegistry and returns its prompt for
// inline execution.
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/observe"
	"github.com/egoisutolabs/forge/tools"
)

// safeOnlyTools is the set of tool names that are unconditionally read-only.
// A skill whose AllowedTools is a strict subset of these is auto-approved.
var safeOnlyTools = map[string]bool{
	"Read": true, "Glob": true, "Grep": true, "AstGrep": true,
}

// toolInput is the JSON schema for SkillTool input.
type toolInput struct {
	Skill string `json:"skill"`
	Args  string `json:"args,omitempty"`
}

// Tool implements SkillTool — invoke a skill (slash command).
//
// This is the Go port of Claude Code's SkillTool. Key behaviors:
//   - Input: {skill, args?}
//   - Leading slash stripped from skill name for compatibility
//   - Looks up skill in tctx.Skills (SkillRegistry)
//   - Inline mode: returns the skill prompt so the model processes it
//   - Fork mode: not yet implemented; falls back to inline
//   - Concurrency: NOT safe (expands into a full prompt)
type Tool struct{}

func (t *Tool) Name() string        { return "Skill" }
func (t *Tool) Description() string { return "Invoke a skill (slash command) by name." }

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"skill": {
				"type": "string",
				"description": "The skill name. E.g., \"commit\", \"review-pr\", or \"pdf\""
			},
			"args": {
				"type": "string",
				"description": "Optional arguments for the skill"
			}
		},
		"required": ["skill"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	name := strings.TrimSpace(in.Skill)
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return fmt.Errorf("skill name is required and cannot be empty")
	}
	return nil
}

func (t *Tool) CheckPermissions(input json.RawMessage, tctx *tools.ToolContext) (*models.PermissionDecision, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.PermissionDecision{Behavior: models.PermDeny, Message: "invalid input"}, nil
	}

	name := normalizeSkillName(in.Skill)

	if tctx == nil || tctx.Skills == nil {
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}

	s := tctx.Skills.Lookup(name)
	if s == nil {
		return &models.PermissionDecision{
			Behavior: models.PermDeny,
			Message:  fmt.Sprintf("unknown skill: %s", name),
		}, nil
	}

	// Skills restricted to only read-only tools can be auto-approved.
	if isSkillSafe(s.AllowedTools) {
		return &models.PermissionDecision{Behavior: models.PermAllow}, nil
	}
	return &models.PermissionDecision{
		Behavior: models.PermAsk,
		Message:  fmt.Sprintf("run skill: %s", name),
	}, nil
}

// IsConcurrencySafe returns false — skill execution expands into a full prompt
// that the model must process, so only one skill should run at a time.
func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return false }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return false }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	name := normalizeSkillName(in.Skill)

	if tctx == nil || tctx.Skills == nil {
		return &models.ToolResult{Content: "skill registry not available", IsError: true}, nil
	}

	s := tctx.Skills.Lookup(name)
	if s == nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Unknown skill: %s", name),
			IsError: true,
		}, nil
	}

	// Emit skill_invoke event.
	if observe.Enabled() {
		promptLen := 0
		if s.Prompt != nil {
			promptLen = len(s.Prompt(in.Args))
		}
		observe.EmitSkillInvoke(name, in.Args, s.Source, s.AllowedTools, promptLen)
	}

	// Skills with an Execute function run programmatically (e.g. ForgeOrchestrator).
	if s.Execute != nil {
		if err := s.Execute(ctx, in.Args, tctx); err != nil {
			return &models.ToolResult{
				Content: fmt.Sprintf("skill %q error: %s", name, err),
				IsError: true,
			}, nil
		}
		return &models.ToolResult{Content: fmt.Sprintf("skill %q complete", name)}, nil
	}

	if s.Prompt == nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Skill %q has no prompt", name),
			IsError: true,
		}, nil
	}

	prompt := s.Prompt(in.Args)

	type output struct {
		Success      bool     `json:"success"`
		CommandName  string   `json:"commandName"`
		Status       string   `json:"status"`
		AllowedTools []string `json:"allowedTools,omitempty"`
		Prompt       string   `json:"prompt"`
	}
	out := output{
		Success:      true,
		CommandName:  name,
		Status:       "inline",
		AllowedTools: s.AllowedTools,
		Prompt:       prompt,
	}
	data, _ := json.Marshal(out)
	return &models.ToolResult{Content: string(data)}, nil
}

// normalizeSkillName strips leading slash and trims whitespace.
func normalizeSkillName(name string) string {
	name = strings.TrimSpace(name)
	return strings.TrimPrefix(name, "/")
}

// isSkillSafe returns true when all of the skill's AllowedTools are read-only.
// A skill with no AllowedTools restriction is NOT considered safe since it
// can run any tool including write operations.
func isSkillSafe(allowedTools []string) bool {
	if len(allowedTools) == 0 {
		return false
	}
	for _, name := range allowedTools {
		if !safeOnlyTools[name] {
			return false
		}
	}
	return true
}
