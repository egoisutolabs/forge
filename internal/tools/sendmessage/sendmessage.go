package sendmessage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoisutolabs/forge/internal/models"
	"github.com/egoisutolabs/forge/internal/tools"
	"github.com/egoisutolabs/forge/internal/tools/agent"
)

const toolName = "SendMessage"

// InboxMessage is the JSON structure appended to an agent's inbox file.
type InboxMessage struct {
	From      string `json:"from"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp"`
	Summary   string `json:"summary,omitempty"`
}

type toolInput struct {
	To      string `json:"to"`
	Message string `json:"message"`
	Summary string `json:"summary,omitempty"`
}

// Tool implements SendMessage — delivers a follow-up message to a background agent.
// Used by the coordinator to continue workers without spawning new ones.
type Tool struct {
	registry *agent.AgentRegistry
	mu       sync.Mutex
}

// New creates a SendMessageTool backed by the given registry.
// If registry is nil, agent.DefaultRegistry is used.
func New(registry *agent.AgentRegistry) *Tool {
	if registry == nil {
		registry = agent.DefaultRegistry
	}
	return &Tool{registry: registry}
}

func (t *Tool) Name() string                             { return toolName }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return false }
func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *Tool) Description() string {
	return "Send a message to a running background agent. Used by the coordinator to continue workers with follow-up instructions."
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"to": {
				"type": "string",
				"description": "Agent ID or name of the target agent"
			},
			"message": {
				"type": "string",
				"description": "Message content to deliver to the agent"
			},
			"summary": {
				"type": "string",
				"description": "Optional 5-10 word summary shown as a preview"
			}
		},
		"required": ["to", "message"]
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if strings.TrimSpace(in.To) == "" {
		return fmt.Errorf("to is required and cannot be empty")
	}
	if strings.TrimSpace(in.Message) == "" {
		return fmt.Errorf("message is required and cannot be empty")
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) Execute(_ context.Context, input json.RawMessage, _ *tools.ToolContext) (*models.ToolResult, error) {
	var in toolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &models.ToolResult{Content: fmt.Sprintf("Invalid input: %s", err), IsError: true}, nil
	}

	agentID := t.resolveAgentID(in.To)
	if agentID == "" {
		return &models.ToolResult{
			Content: fmt.Sprintf("agent not found: %s", in.To),
			IsError: true,
		}, nil
	}

	msg := InboxMessage{
		From:      "coordinator",
		Text:      in.Message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Summary:   in.Summary,
	}
	if err := t.deliver(agentID, msg); err != nil {
		return &models.ToolResult{
			Content: fmt.Sprintf("Failed to deliver message: %s", err),
			IsError: true,
		}, nil
	}

	return &models.ToolResult{
		Content: fmt.Sprintf("Message delivered to agent %s.", agentID),
	}, nil
}

// resolveAgentID maps to (an agent ID or description/name) to a registered
// agent ID. Returns "" if not found.
func (t *Tool) resolveAgentID(to string) string {
	if ba := t.registry.Get(to); ba != nil {
		return ba.AgentID
	}
	// Fall back: match by description (set when agent is named via the name field).
	for _, ba := range t.registry.List() {
		if ba.Description == to {
			return ba.AgentID
		}
	}
	return ""
}

// deliver appends msg to ~/.forge/agents/{agentID}/inbox.json.
// The mutation is serialized by mu.
func (t *Tool) deliver(agentID string, msg InboxMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	dir, err := agentInboxDir(agentID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create agent inbox dir: %w", err)
	}

	inboxPath := filepath.Join(dir, "inbox.json")

	var messages []InboxMessage
	if data, err := os.ReadFile(inboxPath); err == nil {
		_ = json.Unmarshal(data, &messages)
	}
	messages = append(messages, msg)

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inbox: %w", err)
	}
	if err := os.WriteFile(inboxPath, data, 0o600); err != nil {
		return fmt.Errorf("write inbox: %w", err)
	}
	return nil
}

// agentInboxDir returns the directory path for an agent's inbox:
// ~/.forge/agents/{agentID}/
func agentInboxDir(agentID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".forge", "agents", agentID), nil
}
