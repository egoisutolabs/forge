package sendmessage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoisutolabs/forge/tools/agent"
)

// newTestTool returns a SendMessageTool backed by a fresh AgentRegistry.
func newTestTool(t *testing.T) (*Tool, *agent.AgentRegistry) {
	t.Helper()
	reg := agent.NewAgentRegistry()
	return New(reg), reg
}

// registerAgent adds a BackgroundAgent to the registry and returns its ID.
func registerAgent(t *testing.T, reg *agent.AgentRegistry, id, description string) {
	t.Helper()
	dir := t.TempDir()
	ba := &agent.BackgroundAgent{
		AgentID:     id,
		Description: description,
		Status:      agent.AgentStatusRunning,
		OutputFile:  filepath.Join(dir, id+".output"),
		StartedAt:   time.Now(),
	}
	reg.Register(ba)
}

// --- interface compliance ---

func TestSendMessageTool_ImplementsToolInterface(t *testing.T) {
	// This will fail to compile if Tool doesn't implement tools.Tool.
	_ = New(nil)
}

func TestSendMessageTool_Name(t *testing.T) {
	if New(nil).Name() != "SendMessage" {
		t.Error("wrong tool name")
	}
}

func TestSendMessageTool_IsReadOnly(t *testing.T) {
	if New(nil).IsReadOnly(nil) {
		t.Error("SendMessage should not be read-only")
	}
}

func TestSendMessageTool_IsConcurrencySafe(t *testing.T) {
	if !New(nil).IsConcurrencySafe(nil) {
		t.Error("SendMessage should be concurrency-safe")
	}
}

// --- CheckPermissions ---

func TestSendMessageTool_CheckPermissions_AlwaysAllow(t *testing.T) {
	dec, err := New(nil).CheckPermissions(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Behavior != "allow" {
		t.Errorf("expected allow, got %q", dec.Behavior)
	}
}

// --- ValidateInput ---

func TestSendMessageTool_ValidateInput_Valid(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{"to":"agent-1","message":"hello"}`))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendMessageTool_ValidateInput_MissingTo(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{"message":"hello"}`))
	if err == nil {
		t.Error("expected error for missing 'to'")
	}
}

func TestSendMessageTool_ValidateInput_MissingMessage(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{"to":"agent-1"}`))
	if err == nil {
		t.Error("expected error for missing 'message'")
	}
}

func TestSendMessageTool_ValidateInput_EmptyTo(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{"to":"  ","message":"hello"}`))
	if err == nil {
		t.Error("expected error for blank 'to'")
	}
}

func TestSendMessageTool_ValidateInput_EmptyMessage(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{"to":"agent-1","message":"  "}`))
	if err == nil {
		t.Error("expected error for blank 'message'")
	}
}

func TestSendMessageTool_ValidateInput_InvalidJSON(t *testing.T) {
	err := New(nil).ValidateInput(json.RawMessage(`{bad`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- Execute: agent not found ---

func TestSendMessageTool_Execute_AgentNotFound(t *testing.T) {
	tool, _ := newTestTool(t)
	result, err := tool.Execute(context.Background(),
		json.RawMessage(`{"to":"nonexistent","message":"hello"}`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for nonexistent agent")
	}
	if !strings.Contains(result.Content, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result.Content)
	}
}

// --- Execute: delivery by ID ---

func TestSendMessageTool_Execute_DeliversByAgentID(t *testing.T) {
	tool, reg := newTestTool(t)
	registerAgent(t, reg, "abc-123", "test worker")

	result, err := tool.Execute(context.Background(),
		json.RawMessage(`{"to":"abc-123","message":"do this task"}`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "abc-123") {
		t.Errorf("expected agentID in result, got: %s", result.Content)
	}
}

// --- Execute: delivery by description/name ---

func TestSendMessageTool_Execute_DeliversByDescription(t *testing.T) {
	tool, reg := newTestTool(t)
	registerAgent(t, reg, "xyz-456", "my-worker")

	result, err := tool.Execute(context.Background(),
		json.RawMessage(`{"to":"my-worker","message":"follow-up"}`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}
}

// --- Execute: inbox file content ---

func TestSendMessageTool_Execute_WritesInboxFile(t *testing.T) {
	tool, reg := newTestTool(t)
	registerAgent(t, reg, "def-789", "inbox worker")

	_, err := tool.Execute(context.Background(),
		json.RawMessage(`{"to":"def-789","message":"hello world","summary":"greeting"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	inboxPath := filepath.Join(home, ".forge", "agents", "def-789", "inbox.json")

	data, err := os.ReadFile(inboxPath)
	if err != nil {
		t.Fatalf("inbox file not created: %v", err)
	}

	var messages []InboxMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("inbox file is not valid JSON: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.From != "coordinator" {
		t.Errorf("expected from=coordinator, got %q", msg.From)
	}
	if msg.Text != "hello world" {
		t.Errorf("expected text='hello world', got %q", msg.Text)
	}
	if msg.Summary != "greeting" {
		t.Errorf("expected summary='greeting', got %q", msg.Summary)
	}
	if msg.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}

	// Cleanup.
	_ = os.RemoveAll(filepath.Dir(inboxPath))
}

// --- Execute: appends messages ---

func TestSendMessageTool_Execute_AppendsMessages(t *testing.T) {
	tool, reg := newTestTool(t)
	registerAgent(t, reg, "app-001", "append worker")

	for i := 0; i < 3; i++ {
		_, err := tool.Execute(context.Background(),
			json.RawMessage(`{"to":"app-001","message":"msg"}`), nil)
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
	}

	home, _ := os.UserHomeDir()
	inboxPath := filepath.Join(home, ".forge", "agents", "app-001", "inbox.json")
	defer os.RemoveAll(filepath.Dir(inboxPath))

	data, _ := os.ReadFile(inboxPath)
	var messages []InboxMessage
	_ = json.Unmarshal(data, &messages)
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}
}

// --- Execute: invalid JSON input ---

func TestSendMessageTool_Execute_InvalidJSON(t *testing.T) {
	tool, _ := newTestTool(t)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{bad`), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid JSON input")
	}
}

// --- New with nil registry uses DefaultRegistry ---

func TestNew_NilRegistryUsesDefault(t *testing.T) {
	tool := New(nil)
	if tool.registry != agent.DefaultRegistry {
		t.Error("expected DefaultRegistry when nil passed to New")
	}
}
