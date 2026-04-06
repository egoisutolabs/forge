package coordinator

import (
	"os"

	"github.com/egoisutolabs/forge/tools"
)

// IsCoordinatorMode reports whether the current process is running as a
// coordinator. Enabled by setting FORGE_COORDINATOR_MODE=1.
func IsCoordinatorMode() bool {
	return os.Getenv("FORGE_COORDINATOR_MODE") == "1"
}

// CoordinatorSystemPrompt returns the system prompt for coordinator mode.
// It instructs the model to orchestrate workers rather than act directly.
func CoordinatorSystemPrompt() string {
	return `You are a coordinator. Your job is to direct workers to research, implement, and verify changes.

## Your Role

- Help the user achieve their goal by directing workers via the Agent tool
- Synthesize worker results and communicate clearly with the user
- Answer simple questions directly when no tools are needed
- Never fabricate or predict worker results — results arrive as separate messages

## Your Tools

- **Agent** — Spawn a new worker
- **SendMessage** — Continue an existing worker with a follow-up message
- **TaskStop** — Stop a running worker

## After Launching Agents

After launching agents, briefly tell the user what you launched and end your response.
Never fabricate or predict agent results in any format — results arrive as separate messages.`
}

// coordinatorAllowedTools is the set of tool names available to a coordinator.
var coordinatorAllowedTools = map[string]bool{
	"Agent":       true,
	"SendMessage": true,
	"TaskStop":    true,
}

// CoordinatorTools filters allTools to only those the coordinator may use:
// Agent, SendMessage, and TaskStop. All other tools are removed.
func CoordinatorTools(allTools []tools.Tool) []tools.Tool {
	var result []tools.Tool
	for _, t := range allTools {
		if coordinatorAllowedTools[t.Name()] {
			result = append(result, t)
		}
	}
	return result
}
