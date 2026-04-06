// Package speculation implements speculative pre-execution of predicted user prompts.
//
// After an assistant response, the Speculator can predict likely follow-up
// prompts (e.g., "run tests", "commit changes") and pre-execute them in the
// background. If the user accepts a suggestion, the pre-computed results are
// injected instantly rather than waiting for a fresh execution.
package speculation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/models"
)

// SpecStatus represents the lifecycle state of a speculation.
type SpecStatus string

const (
	StatusPending   SpecStatus = "pending"
	StatusRunning   SpecStatus = "running"
	StatusCompleted SpecStatus = "completed"
	StatusFailed    SpecStatus = "failed"
	StatusCancelled SpecStatus = "cancelled"
)

// Config holds configuration for the Speculator.
type Config struct {
	Model        string
	SystemPrompt string
	MaxTurns     int
	Cwd          string // working directory for speculative agents
}

// Speculator manages speculative pre-execution of predicted prompts.
type Speculator struct {
	config Config
	caller api.Caller
	active map[string]*Speculation
	mu     sync.Mutex
}

// Speculation represents a single speculative execution.
type Speculation struct {
	ID      string
	Prompt  string
	Status  SpecStatus
	Result  *SpeculationResult
	Cancel  context.CancelFunc
	WorkDir string // temp directory for file changes
}

// SpeculationResult holds the output of a completed speculation.
type SpeculationResult struct {
	Messages   []*models.Message
	FileDiffs  []FileDiff
	TotalUsage models.Usage
}

// FileDiff captures the before/after state of a file.
type FileDiff struct {
	Path   string // absolute path in the real workspace
	Before string // empty if new file
	After  string // empty if deleted
}

// NewSpeculator creates a new Speculator with the given config and API caller.
func NewSpeculator(cfg Config, caller api.Caller) *Speculator {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 20
	}
	return &Speculator{
		config: cfg,
		caller: caller,
		active: make(map[string]*Speculation),
	}
}

// Speculate launches a background execution of the given prompt.
// Returns the speculation ID for later Accept/Reject.
func (s *Speculator) Speculate(ctx context.Context, prompt string) (string, error) {
	id := uuid.NewString()

	workDir, err := os.MkdirTemp("", "forge-spec-*")
	if err != nil {
		return "", fmt.Errorf("speculation: create temp dir: %w", err)
	}

	specCtx, cancel := context.WithCancel(ctx)

	spec := &Speculation{
		ID:      id,
		Prompt:  prompt,
		Status:  StatusPending,
		Cancel:  cancel,
		WorkDir: workDir,
	}

	s.mu.Lock()
	s.active[id] = spec
	s.mu.Unlock()

	go s.run(specCtx, spec)

	return id, nil
}

// run executes the speculation in a goroutine.
func (s *Speculator) run(ctx context.Context, spec *Speculation) {
	s.mu.Lock()
	spec.Status = StatusRunning
	s.mu.Unlock()

	eng := engine.New(engine.Config{
		Model:        s.config.Model,
		SystemPrompt: s.config.SystemPrompt,
		MaxTurns:     s.config.MaxTurns,
		Cwd:          spec.WorkDir,
	})

	result, err := eng.SubmitMessage(ctx, s.caller, spec.Prompt)

	s.mu.Lock()
	defer s.mu.Unlock()

	if ctx.Err() != nil {
		spec.Status = StatusCancelled
		return
	}
	if err != nil {
		spec.Status = StatusFailed
		return
	}

	messages := eng.Messages()
	spec.Status = StatusCompleted
	spec.Result = &SpeculationResult{
		Messages:   messages,
		TotalUsage: result.TotalUsage,
	}
}

// Accept applies a completed speculation's results to the real workspace.
// Returns the result for the caller to inject into the main conversation.
// The speculation is removed from the active map afterward.
func (s *Speculator) Accept(id string) (*SpeculationResult, error) {
	s.mu.Lock()
	spec, ok := s.active[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("speculation: not found: %s", id)
	}
	if spec.Status != StatusCompleted {
		s.mu.Unlock()
		return nil, fmt.Errorf("speculation: cannot accept non-completed speculation (status: %s)", spec.Status)
	}
	result := spec.Result
	workDir := spec.WorkDir
	delete(s.active, id)
	s.mu.Unlock()

	// Apply file diffs to real workspace
	for _, diff := range result.FileDiffs {
		if err := applyFileDiff(diff); err != nil {
			return nil, fmt.Errorf("speculation: apply diff for %s: %w", diff.Path, err)
		}
	}

	// Clean up temp directory
	if workDir != "" {
		os.RemoveAll(workDir)
	}

	return result, nil
}

// Reject discards a speculation and cleans up its temp directory.
func (s *Speculator) Reject(id string) error {
	s.mu.Lock()
	spec, ok := s.active[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("speculation: not found: %s", id)
	}
	workDir := spec.WorkDir
	if spec.Cancel != nil {
		spec.Cancel()
	}
	delete(s.active, id)
	s.mu.Unlock()

	if workDir != "" {
		os.RemoveAll(workDir)
	}
	return nil
}

// Cancel cancels all in-flight speculations and cleans up.
func (s *Speculator) Cancel() {
	s.mu.Lock()
	for id, spec := range s.active {
		if spec.Cancel != nil {
			spec.Cancel()
		}
		if spec.WorkDir != "" {
			os.RemoveAll(spec.WorkDir)
		}
		spec.Status = StatusCancelled
		delete(s.active, id)
	}
	s.mu.Unlock()
}

// Suggest returns predicted follow-up prompts based on recent conversation
// context. Uses simple heuristics (not AI-generated) to keep it fast.
func (s *Speculator) Suggest(messages []*models.Message) []string {
	if len(messages) == 0 {
		return nil
	}

	var suggestions []string

	last := messages[len(messages)-1]

	// Check what tools were used in the last message
	toolNames := toolNamesFromMessage(last)

	// Check for error patterns in tool results
	hasError := hasErrorResult(messages)

	switch {
	case hasError:
		suggestions = append(suggestions, "Fix the error")
	case containsAny(toolNames, "Edit", "Write"):
		suggestions = append(suggestions, "Run tests")
		suggestions = append(suggestions, "Commit changes")
	case containsAny(toolNames, "Bash"):
		suggestions = append(suggestions, "Run tests")
	case containsAny(toolNames, "Read", "Glob", "Grep"):
		suggestions = append(suggestions, "Explain this code")
	}

	// Cap at 2 suggestions
	if len(suggestions) > 2 {
		suggestions = suggestions[:2]
	}
	return suggestions
}

// applyFileDiff writes a file diff to disk.
func applyFileDiff(diff FileDiff) error {
	if diff.After == "" {
		// Deletion
		return os.Remove(diff.Path)
	}
	// Create parent directories if needed
	dir := filepath.Dir(diff.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(diff.Path, []byte(diff.After), 0644)
}

// toolNamesFromMessage extracts tool names from tool_use blocks.
func toolNamesFromMessage(msg *models.Message) []string {
	var names []string
	for _, b := range msg.Content {
		if b.Type == models.BlockToolUse {
			names = append(names, b.Name)
		}
	}
	return names
}

// hasErrorResult checks if any recent message has an error tool result.
func hasErrorResult(messages []*models.Message) bool {
	// Check last 3 messages for error results
	start := len(messages) - 3
	if start < 0 {
		start = 0
	}
	for _, msg := range messages[start:] {
		for _, b := range msg.Content {
			if b.Type == models.BlockToolResult && b.IsError {
				return true
			}
			// Also check content text for common failure patterns
			if b.Type == models.BlockToolResult && containsFailurePattern(b.Content) {
				return true
			}
		}
	}
	return false
}

// containsFailurePattern checks text for common test/build failure indicators.
func containsFailurePattern(text string) bool {
	lower := strings.ToLower(text)
	patterns := []string{"fail:", "error:", "panic:", "fatal:"}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// containsAny returns true if names contains any of the target values.
func containsAny(names []string, targets ...string) bool {
	for _, n := range names {
		for _, t := range targets {
			if n == t {
				return true
			}
		}
	}
	return false
}
