package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/internal/api"
	"github.com/egoisutolabs/forge/internal/engine"
	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/orchestrator/agents"
	"github.com/egoisutolabs/forge/internal/tools"
)

// PhaseResult holds the parsed output from a single phase run.
type PhaseResult struct {
	// Status is the normalised outcome: "done", "blocked", "pass", or "fail".
	Status string
	// Message is the detail clause after the " - " separator, if any.
	Message string
	// Raw is the full unprocessed status string returned by the agent.
	Raw string
}

// phaseAllowedTools maps each phase name to the tool names its worker may use.
// Workers never receive Agent or AskUserQuestion; the orchestrator handles
// both tool-spawning and user interaction.
var phaseAllowedTools = map[string][]string{
	"plan":      {"Read", "Glob", "AstGrep", "Grep", "Bash", "Browser", "WebFetch", "WebSearch"},
	"prepare":   {"Read", "Write", "Glob", "AstGrep", "Grep", "Bash", "Browser", "WebFetch", "WebSearch"},
	"test":      {"Read", "Write", "Glob", "AstGrep", "Grep", "Bash"},
	"implement": {"Read", "Write", "Edit", "Glob", "AstGrep", "Grep", "Bash"},
	"verify":    {"Read", "Glob", "AstGrep", "Grep", "Bash"},
}

// phaseInputFiles lists artifact filenames (relative to .forge/features/{slug}/)
// that should be loaded and injected into the user message for each phase.
var phaseInputFiles = map[string][]string{
	"plan":      {}, // plan reads the raw feature request; no prior artifacts required
	"prepare":   {"discovery.md", "exploration.md", "architecture.md"},
	"test":      {"implementation-context.md", "exploration.md"},
	"implement": {"implementation-context.md", "exploration.md", "test-manifest.md"},
	"verify":    {"implementation-context.md", "exploration.md", "test-manifest.md", "impl-manifest.md"},
}

// optionalInputFiles are loaded when present but not required.
var optionalInputFiles = map[string][]string{
	"prepare":   {"design-discuss.md"},
	"implement": {"verify-report.md"},
}

const phaseMaxTurns = 50

// PhaseRunner runs individual pipeline phases by spawning a sub-agent via
// engine.RunLoop. It is stateless — each RunPhase call is independent.
type PhaseRunner struct {
	Caller    api.Caller
	Model     string
	Tools     []tools.Tool
	Cwd       string
	AgentDefs []agents.AgentDef
}

// RunPhase executes one phase of the forge pipeline.
//
// It:
//  1. Looks up the embedded agent definition for the phase.
//  2. Builds a system prompt (agent markdown with contracts already resolved).
//  3. Constructs a user message containing the feature description and the
//     contents of required prior-phase artifacts.
//  4. Filters the tool set to only those permitted for this phase.
//  5. Calls engine.RunLoop to run the sub-agent.
//  6. Parses the final assistant message for the phase status string.
func (r *PhaseRunner) RunPhase(ctx context.Context, phase Phase, slug, featureDesc string, entry *FeatureEntry) (PhaseResult, error) {
	agentName := strings.TrimSuffix(phase.AgentDef, ".md")
	agentDef := agents.FindAgent(r.AgentDefs, agentName)
	if agentDef == nil {
		return PhaseResult{}, fmt.Errorf("runner: agent definition not found: %q", agentName)
	}

	systemPrompt := agentDef.Prompt
	userMsg := r.buildUserMessage(phase, slug, featureDesc, entry)
	phaseTools := r.filterToolsForPhase(phase.Name)

	result, msgs, err := engine.RunLoop(ctx, engine.LoopParams{
		Caller:       r.Caller,
		Messages:     []*models.Message{models.NewUserMessage(userMsg)},
		SystemPrompt: systemPrompt,
		Tools:        phaseTools,
		Model:        r.resolveModel(agentDef.Model),
		MaxTurns:     phaseMaxTurns,
	})
	if err != nil {
		return PhaseResult{}, fmt.Errorf("runner: phase %s loop error: %w", phase.Name, err)
	}
	if result.Reason == models.StopModelError {
		return PhaseResult{}, fmt.Errorf("runner: phase %s model error", phase.Name)
	}

	return r.parsePhaseResult(msgs), nil
}

// buildSystemPrompt returns the agent's prompt (already has contracts resolved).
func (r *PhaseRunner) BuildSystemPrompt(phase Phase) (string, error) {
	agentName := strings.TrimSuffix(phase.AgentDef, ".md")
	agentDef := agents.FindAgent(r.AgentDefs, agentName)
	if agentDef == nil {
		return "", fmt.Errorf("runner: agent definition not found: %q", agentName)
	}
	return agentDef.Prompt, nil
}

// buildUserMessage constructs the initial user message for a phase.
// It injects the feature description, slug, mode, and the contents of
// required prior-phase artifacts.
func (r *PhaseRunner) buildUserMessage(phase Phase, slug, featureDesc string, entry *FeatureEntry) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Run the **%s** phase for feature: %s\n", phase.Name, featureDesc)
	fmt.Fprintf(&sb, "Slug: %s\n", slug)
	fmt.Fprintf(&sb, "Feature directory: .forge/features/%s/\n", slug)
	if entry != nil {
		fmt.Fprintf(&sb, "Mode: %s\n", entry.Mode)
	}
	if r.Cwd != "" {
		fmt.Fprintf(&sb, "Working directory: %s\n", r.Cwd)
	}

	// Load required input artifacts.
	r.appendArtifacts(&sb, phase.Name, slug, phaseInputFiles[phase.Name], true)
	// Load optional input artifacts.
	r.appendArtifacts(&sb, phase.Name, slug, optionalInputFiles[phase.Name], false)

	return sb.String()
}

// appendArtifacts reads artifact files and appends their contents to sb.
// If required is true, missing files are noted. If false, they are silently skipped.
func (r *PhaseRunner) appendArtifacts(sb *strings.Builder, _, slug string, files []string, required bool) {
	featureDir := filepath.Join(r.Cwd, ".forge", "features", slug)
	for _, filename := range files {
		path := filepath.Join(featureDir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if required {
				fmt.Fprintf(sb, "\n> Note: required artifact %q not found.\n", filename)
			}
			continue
		}
		fmt.Fprintf(sb, "\n## %s\n\n```\n%s\n```\n", filename, strings.TrimSpace(string(data)))
	}
}

// filterToolsForPhase returns the subset of r.Tools allowed for the given phase.
// Unknown phases fall back to the full tool set.
func (r *PhaseRunner) filterToolsForPhase(phaseName string) []tools.Tool {
	allowed, ok := phaseAllowedTools[phaseName]
	if !ok {
		return r.Tools
	}
	allowSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowSet[name] = true
	}
	var filtered []tools.Tool
	for _, t := range r.Tools {
		if allowSet[t.Name()] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// parsePhaseResult extracts the status from the final assistant message.
//
// Agents respond with strings like:
//   - "done - plan ready"
//   - "blocked - planning input required"
//   - "pass"
//   - "fail - 3 test failures, 0 scope violations"
//
// The returned Status is normalised to: "done", "blocked", "pass", or "fail".
func (r *PhaseRunner) parsePhaseResult(msgs []*models.Message) PhaseResult {
	raw := lastAssistantText(msgs)

	// Split on first " - " to get status and optional detail.
	parts := strings.SplitN(raw, " - ", 2)
	status := strings.TrimSpace(strings.ToLower(parts[0]))
	detail := ""
	if len(parts) > 1 {
		detail = strings.TrimSpace(parts[1])
	}

	// Normalise: some agents say "done - ..." with extra words; we only want the keyword.
	switch {
	case status == "pass":
		// keep as-is
	case strings.HasPrefix(status, "done"):
		status = "done"
	case strings.HasPrefix(status, "blocked"):
		status = "blocked"
	case strings.HasPrefix(status, "fail"):
		status = "fail"
	}

	return PhaseResult{Status: status, Message: detail, Raw: raw}
}

// resolveModel maps the agent's model hint to a full model ID.
// "inherit" (or empty) falls back to the runner's configured model.
func (r *PhaseRunner) resolveModel(hint string) string {
	switch strings.ToLower(hint) {
	case "inherit", "":
		return r.Model
	case "sonnet":
		return "claude-sonnet-4-6"
	case "opus":
		return "claude-opus-4-6"
	case "haiku":
		return "claude-haiku-4-5-20251001"
	default:
		return hint // treat as a full model ID
	}
}

// ParsePhaseResultForTest is an exported wrapper for parsePhaseResult used in
// package-external tests. It builds a minimal message list and delegates.
func ParsePhaseResultForTest(raw string) PhaseResult {
	msg := models.NewUserMessage("") // role user but we swap it
	msg.Role = models.RoleAssistant
	msg.Content = []models.Block{{Type: models.BlockText, Text: raw}}
	return (&PhaseRunner{}).parsePhaseResult([]*models.Message{msg})
}

// PhaseToolsForTest returns the allowed tool names for phaseName and a fixed
// set of canonical "denied" names (Agent, AskUserQuestion, Write, Edit, AstGrep, Browser, WebFetch).
// Used in tests that need to verify per-phase tool filtering without a live runner.
func PhaseToolsForTest(phaseName string) (allowed, denied []string) {
	allowed = phaseAllowedTools[phaseName]
	// Denied is the complement of allowed in the full canonical tool name set.
	allNames := []string{"Read", "Write", "Edit", "Glob", "AstGrep", "Grep", "Bash", "Browser", "WebFetch", "WebSearch", "Agent", "AskUserQuestion"}
	allowSet := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allowSet[n] = true
	}
	for _, n := range allNames {
		if !allowSet[n] {
			denied = append(denied, n)
		}
	}
	return allowed, denied
}

// lastAssistantText returns the trimmed text of the last assistant message, or "".
func lastAssistantText(msgs []*models.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == models.RoleAssistant {
			return strings.TrimSpace(msgs[i].TextContent())
		}
	}
	return ""
}
