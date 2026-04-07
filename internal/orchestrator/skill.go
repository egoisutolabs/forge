package orchestrator

import (
	"context"
	"fmt"

	"github.com/egoisutolabs/forge/internal/skills"
	"github.com/egoisutolabs/forge/internal/tools"
)

// RegisterForgeSkill registers the /forge skill in registry.
//
// When the user types /forge <feature description>, the SkillTool detects the
// Execute callback and calls it directly, which creates a ForgeOrchestrator
// and runs the full 5-phase pipeline instead of just injecting a text prompt.
//
// Call this from your application setup (e.g. cmd/forge/main.go):
//
//	orchestrator.RegisterForgeSkill(skills.BundledRegistry())
func RegisterForgeSkill(registry *skills.SkillRegistry) {
	registry.Register(&skills.Skill{
		Name:          "forge",
		Description:   "Build or change a feature end-to-end: plan, prepare, test, implement, verify",
		WhenToUse:     "When the user wants to build a complete feature from scratch or make a significant change that needs planning, tests, implementation, and verification",
		Context:       skills.ContextInline,
		UserInvocable: true,
		Source:        "bundled",

		// Prompt is a fallback used when Execute is not called (e.g. the skill
		// is invoked from an older skill runner that doesn't support Execute).
		Prompt: func(args string) string {
			if args == "" {
				return "Please provide a feature description. Usage: /forge <feature description>"
			}
			return fmt.Sprintf(
				"Start the forge pipeline to build this feature: %s\n\n"+
					"Run the full 5-phase pipeline: plan → prepare → test → implement → verify.",
				args,
			)
		},

		// Execute is called by SkillTool when Skill.Execute is non-nil.
		// It creates a ForgeOrchestrator from the ToolContext and runs the pipeline.
		Execute: func(ctx context.Context, args string, execCtx interface{}) error {
			tctx, ok := execCtx.(*tools.ToolContext)
			if !ok || tctx == nil {
				return fmt.Errorf("forge skill: execution context unavailable")
			}
			if tctx.Caller == nil {
				return fmt.Errorf("forge skill: API caller not set in ToolContext")
			}
			if args == "" {
				return fmt.Errorf("forge skill: feature description is required")
			}

			askUser := forgeAskUserAdapter(tctx)

			orch, err := New(Config{
				Cwd:     tctx.Cwd,
				Caller:  tctx.Caller,
				Model:   tctx.Model,
				Tools:   tctx.Tools,
				AskUser: askUser,
			})
			if err != nil {
				return fmt.Errorf("forge skill: create orchestrator: %w", err)
			}
			return orch.Run(ctx, args)
		},
	})
}

// forgeAskUserAdapter wraps tctx.UserPrompt into the simpler
// func(summary, question string, options []string) (string, error) signature
// that UserGates expects.
//
// If tctx.UserPrompt is nil (non-interactive mode), returns nil so the
// orchestrator knows to skip gates.
func forgeAskUserAdapter(tctx *tools.ToolContext) func(summary, question string, options []string) (string, error) {
	if tctx == nil || tctx.UserPrompt == nil {
		return nil
	}
	return func(summary, question string, options []string) (string, error) {
		// Build an AskQuestion using the question as the key.
		aqs := []tools.AskQuestion{{
			Question: question,
			Header:   summary,
		}}
		if len(options) > 0 {
			aqs[0].Options = make([]tools.AskQuestionOption, len(options))
			for i, opt := range options {
				aqs[0].Options[i] = tools.AskQuestionOption{Label: opt}
			}
		}

		answers, err := tctx.UserPrompt(aqs)
		if err != nil {
			return "", err
		}
		return answers[question], nil
	}
}
