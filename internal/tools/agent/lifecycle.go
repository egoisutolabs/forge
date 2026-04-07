package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentStatus is the lifecycle state of a background agent.
type AgentStatus string

const (
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
)

// BackgroundAgent tracks a background agent spawned asynchronously.
type BackgroundAgent struct {
	AgentID     string
	Description string
	Status      AgentStatus
	OutputFile  string
	Result      string
	Error       string
	StartedAt   time.Time
	FinishedAt  time.Time
}

// AgentNotification is emitted when a background agent completes or fails.
type AgentNotification struct {
	AgentID     string
	Description string
	Status      AgentStatus
	Result      string
	Error       string
}

// AgentRegistry is a thread-safe registry of background agents.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*BackgroundAgent

	// NotifyCh, when non-nil, receives formatted task-notification strings
	// each time a background agent completes or fails. Sends are non-blocking;
	// if the channel is full, notifications are dropped.
	NotifyCh chan<- string
}

// DefaultRegistry is the process-wide background agent registry.
var DefaultRegistry = NewAgentRegistry()

// NewAgentRegistry creates an empty registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*BackgroundAgent),
	}
}

// Register adds a new background agent to the registry.
func (r *AgentRegistry) Register(ba *BackgroundAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[ba.AgentID] = ba
}

// Get returns the BackgroundAgent with the given ID, or nil if not found.
func (r *AgentRegistry) Get(agentID string) *BackgroundAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[agentID]
}

// Complete marks the agent as completed, records the result, and writes
// the result to OutputFile. If NotifyCh is set, a formatted notification
// string is sent (non-blocking).
func (r *AgentRegistry) Complete(agentID, result string) {
	r.mu.Lock()
	ba, ok := r.agents[agentID]
	if !ok {
		r.mu.Unlock()
		return
	}
	ba.Status = AgentStatusCompleted
	ba.Result = result
	ba.FinishedAt = time.Now()
	_ = os.WriteFile(ba.OutputFile, []byte(result), 0o600)
	desc := ba.Description
	ch := r.NotifyCh
	r.mu.Unlock()

	if ch != nil {
		msg := fmt.Sprintf("<task-notification>\nAgent %q completed:\n%s\n</task-notification>", desc, result)
		select {
		case ch <- msg:
		default:
		}
	}
}

// Fail marks the agent as failed, records the error message, and writes
// an error marker to OutputFile. If NotifyCh is set, a formatted notification
// string is sent (non-blocking).
func (r *AgentRegistry) Fail(agentID, errMsg string) {
	r.mu.Lock()
	ba, ok := r.agents[agentID]
	if !ok {
		r.mu.Unlock()
		return
	}
	ba.Status = AgentStatusFailed
	ba.Error = errMsg
	ba.FinishedAt = time.Now()
	_ = os.WriteFile(ba.OutputFile, []byte(fmt.Sprintf("ERROR: %s", errMsg)), 0o600)
	desc := ba.Description
	ch := r.NotifyCh
	r.mu.Unlock()

	if ch != nil {
		msg := fmt.Sprintf("<task-notification>\nAgent %q failed:\n%s\n</task-notification>", desc, errMsg)
		select {
		case ch <- msg:
		default:
		}
	}
}

// List returns a snapshot of all registered background agents.
func (r *AgentRegistry) List() []*BackgroundAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*BackgroundAgent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

// outputFilePath returns the path where agent output will be written.
// Creates ~/.forge/agents/ if it does not exist.
func outputFilePath(agentID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".forge", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create agent output dir: %w", err)
	}
	return filepath.Join(dir, agentID+".output"), nil
}
