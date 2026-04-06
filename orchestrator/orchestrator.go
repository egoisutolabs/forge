// Package orchestrator implements the forge 5-phase feature development pipeline.
// ForgeOrchestrator drives the full plan → prepare → test → implement → verify
// cycle, managing state transitions, user gates, and implement–verify retries.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/orchestrator/agents"
	"github.com/egoisutolabs/forge/tools"
)

const maxVerifyRetries = 3

// Config holds everything the ForgeOrchestrator needs at construction time.
type Config struct {
	// Cwd is the working directory for the session (project root).
	Cwd string

	// Caller is the API caller used to spawn sub-agent loops.
	Caller api.Caller

	// Model is the model ID passed to phase workers ("claude-sonnet-4-6", etc.).
	Model string

	// Tools is the full set of tools available; each phase runner filters this
	// down to only those permitted for that phase.
	Tools []tools.Tool

	// AskUser wires user interaction into the UserGates.
	// The gates call this function to present summaries and questions.
	// Signature: func(summary, question string, options []string) (string, error)
	AskUser func(summary, question string, options []string) (string, error)
}

// ForgeOrchestrator drives the full forge pipeline.
// Construct with New(); call Run() to start or resume a feature.
type ForgeOrchestrator struct {
	config    Config
	state     *ForgeState
	runner    *PhaseRunner
	stateFile string
}

// New creates a ForgeOrchestrator, loading existing state from .forge/state.json.
// The .forge/ directory is created if it does not exist.
func New(cfg Config) (*ForgeOrchestrator, error) {
	forgeDir := filepath.Join(cfg.Cwd, ".forge")
	if err := os.MkdirAll(forgeDir, 0o755); err != nil {
		return nil, fmt.Errorf("orchestrator: create .forge dir: %w", err)
	}

	stateFile := filepath.Join(forgeDir, "state.json")
	state, err := Load(stateFile)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: load state: %w", err)
	}

	agentDefs, err := agents.LoadAgents()
	if err != nil {
		return nil, fmt.Errorf("orchestrator: load agent defs: %w", err)
	}

	runner := &PhaseRunner{
		Caller:    cfg.Caller,
		Model:     cfg.Model,
		Tools:     cfg.Tools,
		Cwd:       cfg.Cwd,
		AgentDefs: agentDefs,
	}

	return &ForgeOrchestrator{
		config:    cfg,
		state:     state,
		runner:    runner,
		stateFile: stateFile,
	}, nil
}

// Run starts or resumes the forge pipeline for featureDesc.
//
// The full lifecycle:
//  1. Bootstrap — derive slug, detect env, init/resume state.
//  2. For each phase from the resume point:
//     plan  → gate (user reviews, can re-run)
//     prepare  → gate
//     test  → no gate (automated)
//     implement → (see retry loop below)
//     verify → gate (pass → done; fail → retry)
//  3. implement–verify retry loop (up to maxVerifyRetries auto retries;
//     after exhaustion the user is asked to accept, retry, or stop).
func (o *ForgeOrchestrator) Run(ctx context.Context, featureDesc string) error {
	boot := NewBootstrap(o.config.Cwd, o.state)
	rc, err := boot.Run(ctx, featureDesc)
	if err != nil {
		return fmt.Errorf("forge: bootstrap: %w", err)
	}

	// User confirmation before we begin / resume.
	if o.config.AskUser != nil {
		gates := &UserGates{FeatureDir: rc.FeatureDir, Ask: o.config.AskUser}
		confirmed, err := gates.AskStartup(featureDesc)
		if err != nil {
			return fmt.Errorf("forge: startup gate: %w", err)
		}
		if !confirmed {
			return fmt.Errorf("forge: user cancelled before start")
		}
	}

	// Determine which phases still need to run.
	startIdx := 0
	for i, p := range PhaseRegistry {
		if p.Name == rc.ResumeFrom {
			startIdx = i
			break
		}
	}

	for i := startIdx; i < len(PhaseRegistry); i++ {
		phase := PhaseRegistry[i]

		if phase.Name == "verify" {
			// verify is always handled inside runImplementVerifyLoop.
			continue
		}

		if phase.Name == "implement" {
			// implement and verify run together in the retry loop.
			if err := o.runImplementVerifyLoop(ctx, rc, featureDesc); err != nil {
				return err
			}
			// Skip the standalone verify entry in the loop (already handled).
			for i+1 < len(PhaseRegistry) && PhaseRegistry[i+1].Name == "verify" {
				i++
			}
			continue
		}

		if err := o.runPhaseWithGate(ctx, phase, rc, featureDesc); err != nil {
			return err
		}
	}

	return nil
}

// runPhaseWithGate runs a single phase and, if the phase has a gate, asks the
// user to review and confirm before the next phase begins.
// Plan phases can be blocked and re-run after user input.
func (o *ForgeOrchestrator) runPhaseWithGate(ctx context.Context, phase Phase, rc RunContext, featureDesc string) error {
	featureDir := filepath.Join(o.config.Cwd, ".forge", "features", rc.Slug)
	gates := &UserGates{FeatureDir: featureDir, Ask: o.config.AskUser}

	for { // retry loop for blocked plan phases
		if err := o.state.SetPhase(rc.Slug, phase.Name, StatusRunning); err != nil {
			return err
		}
		if err := o.state.Save(o.stateFile); err != nil {
			return fmt.Errorf("save state: %w", err)
		}

		entry := o.state.Features[rc.Slug]
		result, err := o.runner.RunPhase(ctx, phase, rc.Slug, featureDesc, entry)
		if err != nil {
			_ = o.state.SetPhase(rc.Slug, phase.Name, StatusFail)
			_ = o.state.Save(o.stateFile)
			return fmt.Errorf("phase %s: %w", phase.Name, err)
		}

		switch result.Status {
		case "done", "pass":
			if err := o.state.SetPhase(rc.Slug, phase.Name, StatusDone); err != nil {
				return err
			}
			if err := o.state.Save(o.stateFile); err != nil {
				return fmt.Errorf("save state: %w", err)
			}
			// Post-phase artifact validation.
			if errs := ValidatePhase(phase.Name, featureDir); len(errs) > 0 {
				// Log but don't fail — agent may have produced valid artifacts
				// with non-standard section names.
				for _, ve := range errs {
					fmt.Printf("forge: artifact warning (%s): %v\n", phase.Name, ve)
				}
			}
			if !phase.HasGate || o.config.AskUser == nil {
				return nil
			}
			return o.runPlanGate(ctx, gates, phase, rc, featureDesc)

		case "blocked":
			if phase.Name != "plan" {
				_ = o.state.SetPhase(rc.Slug, phase.Name, StatusBlocked)
				_ = o.state.Save(o.stateFile)
				return fmt.Errorf("phase %s blocked unexpectedly: %s", phase.Name, result.Message)
			}
			// Plan blocked: ask user for input, then retry.
			answers, err := gates.AskPlanQuestions()
			if err != nil {
				return fmt.Errorf("plan gate (blocked): %w", err)
			}
			if answers == nil {
				// No questions surfaced — re-run anyway (design changed).
				_ = o.state.SetPhase(rc.Slug, phase.Name, StatusNull)
				continue
			}
			_ = o.state.SetPhase(rc.Slug, phase.Name, StatusNull)
			if err := o.state.Save(o.stateFile); err != nil {
				return err
			}
			// Update ExtraContext so the next RunPhase call includes answers.
			featureDesc = featureDesc + "\n\nAdditional context: " + answers["response"]

		default:
			_ = o.state.SetPhase(rc.Slug, phase.Name, StatusFail)
			_ = o.state.Save(o.stateFile)
			return fmt.Errorf("phase %s unexpected status %q: %s", phase.Name, result.Status, result.Raw)
		}
	}
}

// runPlanGate presents the plan-phase gate question to the user.
func (o *ForgeOrchestrator) runPlanGate(ctx context.Context, gates *UserGates, phase Phase, rc RunContext, featureDesc string) error {
	switch phase.Name {
	case "plan":
		choice, err := gates.AskArchitectureChoice()
		if err != nil {
			return fmt.Errorf("plan gate: %w", err)
		}
		if choice == "revise" {
			// Re-run plan phase.
			_ = o.state.SetPhase(rc.Slug, phase.Name, StatusNull)
			_ = o.state.Save(o.stateFile)
			return o.runPhaseWithGate(ctx, phase, rc, featureDesc)
		}
	case "prepare":
		choice, err := gates.SummarizePrepare()
		if err != nil {
			return fmt.Errorf("prepare gate: %w", err)
		}
		if choice == "revise" {
			_ = o.state.SetPhase(rc.Slug, phase.Name, StatusNull)
			_ = o.state.Save(o.stateFile)
			return o.runPhaseWithGate(ctx, phase, rc, featureDesc)
		}
	}
	return nil
}

// runImplementVerifyLoop runs implement → verify with automatic retries.
// After maxVerifyRetries failures the user decides: accept, retry, or stop.
func (o *ForgeOrchestrator) runImplementVerifyLoop(ctx context.Context, rc RunContext, featureDesc string) error {
	featureDir := filepath.Join(o.config.Cwd, ".forge", "features", rc.Slug)
	gates := &UserGates{FeatureDir: featureDir, Ask: o.config.AskUser}
	implementPhase, err := PhaseByName("implement")
	if err != nil {
		return err
	}
	verifyPhase, err := PhaseByName("verify")
	if err != nil {
		return err
	}

	for retries := 0; ; retries++ {
		// ---- implement ----
		if err := o.state.SetPhase(rc.Slug, "implement", StatusRunning); err != nil {
			return err
		}
		_ = o.state.Save(o.stateFile)

		entry := o.state.Features[rc.Slug]
		implResult, err := o.runner.RunPhase(ctx, implementPhase, rc.Slug, featureDesc, entry)
		if err != nil {
			_ = o.state.SetPhase(rc.Slug, "implement", StatusFail)
			_ = o.state.Save(o.stateFile)
			return fmt.Errorf("implement: %w", err)
		}
		if implResult.Status != "done" {
			_ = o.state.SetPhase(rc.Slug, "implement", StatusFail)
			_ = o.state.Save(o.stateFile)
			return fmt.Errorf("implement phase failed: %s", implResult.Raw)
		}
		if err := o.state.SetPhase(rc.Slug, "implement", StatusDone); err != nil {
			return err
		}
		_ = o.state.Save(o.stateFile)

		// ---- verify ----
		if err := o.state.SetPhase(rc.Slug, "verify", StatusRunning); err != nil {
			return err
		}
		_ = o.state.Save(o.stateFile)

		entry = o.state.Features[rc.Slug]
		verifyResult, err := o.runner.RunPhase(ctx, verifyPhase, rc.Slug, featureDesc, entry)
		if err != nil {
			_ = o.state.SetPhase(rc.Slug, "verify", StatusFail)
			_ = o.state.Save(o.stateFile)
			return fmt.Errorf("verify: %w", err)
		}

		passed := verifyResult.Status == "pass" || verifyResult.Status == "done"
		if passed {
			if err := o.state.SetPhase(rc.Slug, "verify", StatusDone); err != nil {
				return err
			}
		} else {
			if err := o.state.SetPhase(rc.Slug, "verify", StatusFail); err != nil {
				return err
			}
			o.state.Features[rc.Slug].Retries++
		}
		_ = o.state.Save(o.stateFile)

		// Auto-retry when under the limit and still failing.
		if !passed && retries < maxVerifyRetries {
			// Reset implement and verify for the next attempt.
			_ = o.state.SetPhase(rc.Slug, "implement", StatusNull)
			_ = o.state.SetPhase(rc.Slug, "verify", StatusNull)
			_ = o.state.Save(o.stateFile)
			continue
		}

		// At this point: either passed, or retries exhausted.
		if o.config.AskUser == nil {
			if !passed {
				return fmt.Errorf("verify failed after %d retries: %s", retries, verifyResult.Message)
			}
			return nil
		}

		// User gate: accept / retry / stop.
		if passed {
			accept, err := gates.SummarizeVerifyPass()
			if err != nil {
				return fmt.Errorf("verify pass gate: %w", err)
			}
			if accept {
				return nil
			}
			// User wants to revise despite passing.
			_ = o.state.SetPhase(rc.Slug, "implement", StatusNull)
			_ = o.state.SetPhase(rc.Slug, "verify", StatusNull)
			o.state.Features[rc.Slug].Retries = 0
			_ = o.state.Save(o.stateFile)
			retries = -1 // will be incremented to 0 on next iteration
			continue
		}

		// Verify failed + retries exhausted: ask user.
		doRetry, err := gates.SummarizeVerifyFail()
		if err != nil {
			return fmt.Errorf("verify fail gate: %w", err)
		}
		if doRetry {
			_ = o.state.SetPhase(rc.Slug, "implement", StatusNull)
			_ = o.state.SetPhase(rc.Slug, "verify", StatusNull)
			o.state.Features[rc.Slug].Retries = 0
			_ = o.state.Save(o.stateFile)
			retries = -1 // reset
			continue
		}
		return fmt.Errorf("forge: stopped at verify (failed after %d retries)", retries)
	}
}
