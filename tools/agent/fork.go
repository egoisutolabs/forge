package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
	"github.com/google/uuid"
)

// ForkBoilerplateTag is the XML tag injected into fork-child messages.
// IsForkChild detects this tag to prevent recursive forking.
// Mirrors Claude Code's FORK_BOILERPLATE_TAG constant.
const ForkBoilerplateTag = "fork-boilerplate"

// ForkDirectivePrefix is prepended to the user directive text inside the fork
// message. Mirrors Claude Code's FORK_DIRECTIVE_PREFIX.
const ForkDirectivePrefix = "Your directive: "

// ForkStatus represents the lifecycle state of a running fork agent.
type ForkStatus string

const (
	ForkStatusRunning   ForkStatus = "running"
	ForkStatusCompleted ForkStatus = "completed"
	ForkStatusError     ForkStatus = "error"
)

// ForkAgent holds metadata about a forked sub-agent.
type ForkAgent struct {
	AgentID        string
	ParentMessages []*models.Message
	SystemPrompt   string
	Status         ForkStatus
}

// AgentLoopRunner is called by StartFork to execute the agent loop inside the
// background goroutine. Set this to a real implementation before calling
// StartFork (e.g. a closure wrapping engine.RunLoop). In tests, replace it
// with a stub to capture arguments without making real API calls.
//
// The default is nil; StartFork is a no-op loop when it is nil (the goroutine
// exits immediately), which is safe for unit tests that only verify message
// construction and recursion prevention.
var AgentLoopRunner func(ctx context.Context, caller api.Caller, messages []*models.Message, systemPrompt string, tt []tools.Tool)

// StartFork spawns a fork child agent in a background goroutine and returns
// its agentID immediately.
//
// The child:
//   - Inherits all parentMessages verbatim (maximises prompt-cache reuse)
//   - Receives the same systemPrompt as the parent
//   - Has the fork boilerplate appended as the final message so it will not
//     spawn further sub-agents
//
// Returns an error immediately (without starting a goroutine) when called from
// inside an existing fork child, preventing infinite recursion.
func StartFork(ctx context.Context, parentMessages []*models.Message, systemPrompt string, tt []tools.Tool, caller api.Caller) (string, error) {
	if IsForkChild(parentMessages) {
		return "", fmt.Errorf("recursive fork prevented: already running inside a fork child")
	}

	agentID := uuid.NewString()

	// Build child messages: full parent history + fork directive appended last.
	childMessages := make([]*models.Message, len(parentMessages), len(parentMessages)+1)
	copy(childMessages, parentMessages)
	childMessages = append(childMessages, BuildForkDirective("Execute your assigned task directly."))

	// Sub-agents must not spawn further agents.
	filteredTools := FilterToolsForAgent(tt, false)

	runner := AgentLoopRunner // capture before goroutine to avoid data race
	go func() {
		if runner != nil {
			runner(ctx, caller, childMessages, systemPrompt, filteredTools)
		}
	}()

	return agentID, nil
}

// IsForkChild returns true when the conversation history contains a message
// with the fork boilerplate tag, indicating this agent is already a fork child
// and must not spawn further forks.
func IsForkChild(messages []*models.Message) bool {
	needle := "<" + ForkBoilerplateTag + ">"
	for _, m := range messages {
		if m.Role != models.RoleUser {
			continue
		}
		for _, b := range m.Content {
			if b.Type == models.BlockText && strings.Contains(b.Text, needle) {
				return true
			}
		}
	}
	return false
}

// BuildForkDirective creates the user message injected as the final entry in a
// fork child's conversation history. It wraps prompt inside the fork boilerplate
// tag so that IsForkChild will recognise the child and block further nesting.
func BuildForkDirective(prompt string) *models.Message {
	return models.NewUserMessage(buildForkText(prompt))
}

// buildForkText constructs the full text content for a fork child directive.
// Mirrors Claude Code's buildChildMessage() in forkSubagent.ts.
func buildForkText(directive string) string {
	open := "<" + ForkBoilerplateTag + ">"
	close := "</" + ForkBoilerplateTag + ">"
	return open + `
STOP. READ THIS FIRST.

You are a forked worker process. You are NOT the main agent.

RULES (non-negotiable):
1. Do NOT spawn sub-agents; execute directly.
2. Do NOT converse, ask questions, or suggest next steps.
3. Do NOT editorialize or add meta-commentary.
4. USE your tools directly: Bash, Read, Write, etc.
5. Stay strictly within your directive's scope.
6. Your response MUST begin with "Scope:". No preamble.
7. REPORT structured facts, then stop.
` + close + `

` + ForkDirectivePrefix + directive
}
