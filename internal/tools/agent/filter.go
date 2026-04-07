package agent

import (
	"github.com/egoisutolabs/forge/internal/tools"
)

// allAgentDisallowedTools lists tools never available to sub-agents.
// Mirrors Claude Code's ALL_AGENT_DISALLOWED_TOOLS.
var allAgentDisallowedTools = map[string]bool{
	"Agent":           true, // no recursive nesting
	"AskUserQuestion": true, // sub-agents cannot prompt the user
	"TaskStop":        true, // only the top-level agent can stop tasks
}

// asyncAgentAllowedTools is the ONLY set allowed for background agents.
// Mirrors Claude Code's ASYNC_AGENT_ALLOWED_TOOLS.
var asyncAgentAllowedTools = map[string]bool{
	"Read":       true,
	"Write":      true,
	"Edit":       true,
	"Glob":       true,
	"Grep":       true,
	"Bash":       true,
	"Skill":      true,
	"ToolSearch": true,
	"AstGrep":    true,
}

// FilterToolsForAgent returns the subset of tools available to a sub-agent.
//
// Rules (mirrors Claude Code's filterToolsForAgent in agentToolUtils.ts):
//  1. Agent, AskUserQuestion, TaskStop are always removed.
//  2. If isAsync is true, only asyncAgentAllowedTools are kept.
func FilterToolsForAgent(tt []tools.Tool, isAsync bool) []tools.Tool {
	var result []tools.Tool
	for _, t := range tt {
		name := t.Name()
		if allAgentDisallowedTools[name] {
			continue
		}
		if isAsync && !asyncAgentAllowedTools[name] {
			continue
		}
		result = append(result, t)
	}
	return result
}

// FilterToolsByNames returns only the tools whose names are in allowed.
// If allowed is nil or empty, tt is returned unchanged.
func FilterToolsByNames(tt []tools.Tool, allowed []string) []tools.Tool {
	if len(allowed) == 0 {
		return tt
	}
	allowSet := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allowSet[n] = true
	}
	var result []tools.Tool
	for _, t := range tt {
		if allowSet[t.Name()] {
			result = append(result, t)
		}
	}
	return result
}
