package observe

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// sanitizeSessionID validates that a session ID is safe for use in file paths.
// It rejects IDs containing path separators or traversal sequences.
func sanitizeSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID must not be empty")
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("session ID must not contain path separators")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("session ID must not contain '..'")
	}
	// Reject if filepath.Base changes the ID (catches hidden traversal)
	if filepath.Base(id) != id {
		return fmt.Errorf("session ID contains invalid path components")
	}
	return nil
}

var (
	globalWriter *Writer
	globalMu     sync.RWMutex
	globalSessID string
	globalRedact bool
	globalTurn   int
)

// EmitterOpts configures the global emitter.
type EmitterOpts struct {
	LogDir string // override default ~/.forge/logs
	Redact bool   // redact tool inputs/outputs
}

// Init starts the global emitter for the given session.
// Called once at startup from main.go.
func Init(sessionID string, opts EmitterOpts) error {
	if err := sanitizeSessionID(sessionID); err != nil {
		return fmt.Errorf("observe.Init: %w", err)
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	dir := opts.LogDir
	if dir == "" {
		var err error
		dir, err = logsDir()
		if err != nil {
			return err
		}
	}

	path := filepath.Join(dir, "session-"+sessionID+".jsonl")
	w, err := NewWriter(path)
	if err != nil {
		return err
	}
	globalWriter = w
	globalSessID = sessionID
	globalRedact = opts.Redact
	globalTurn = 0
	return nil
}

// Shutdown flushes and closes the global writer.
func Shutdown() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalWriter != nil {
		globalWriter.Close()
		globalWriter = nil
	}
}

// SetTurn updates the current loop turn number.
func SetTurn(turn int) {
	globalMu.Lock()
	globalTurn = turn
	globalMu.Unlock()
}

// Enabled returns true if the global emitter is active.
func Enabled() bool {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalWriter != nil
}

// Emit sends an event via the global writer. No-op if not initialized.
func Emit(eventType EventType, payload any) {
	EmitWithTrace(eventType, "", payload)
}

// EmitWithTrace sends an event with a specific trace ID.
func EmitWithTrace(eventType EventType, traceID string, payload any) {
	globalMu.RLock()
	w := globalWriter
	sessID := globalSessID
	redact := globalRedact
	turn := globalTurn
	globalMu.RUnlock()

	if w == nil {
		return
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}

	e := Event{
		Timestamp: time.Now().UTC(),
		SessionID: sessID,
		EventType: eventType,
		TraceID:   traceID,
		Turn:      turn,
		Payload:   raw,
	}

	if redact {
		Redact(&e)
	}

	w.Write(e)
}

// EmitToolStart is a convenience helper for tool_call_start events.
func EmitToolStart(name string, toolUseID string, input json.RawMessage, isConcSafe bool) string {
	traceID := NewTraceID()
	EmitWithTrace(EventToolCallStart, traceID, ToolCallStartPayload{
		ToolName:   name,
		ToolUseID:  toolUseID,
		Input:      input,
		IsConcSafe: isConcSafe,
	})
	return traceID
}

// EmitToolEnd is a convenience helper for tool_call_end events.
func EmitToolEnd(traceID string, name string, toolUseID string, duration time.Duration, output string, isError bool) {
	payload := ToolCallEndPayload{
		ToolName:   name,
		ToolUseID:  toolUseID,
		DurationMs: duration.Milliseconds(),
		IsError:    isError,
		Output:     output,
	}
	if isError {
		payload.ErrorMsg = output
	}
	EmitWithTrace(EventToolCallEnd, traceID, payload)
}

// EmitAgentSpawn is a convenience helper for agent_spawn events.
func EmitAgentSpawn(agentID, description, subagentType, model string, isBackground bool, prompt string) {
	Emit(EventAgentSpawn, AgentSpawnPayload{
		AgentID:      agentID,
		Description:  description,
		SubagentType: subagentType,
		Model:        model,
		IsBackground: isBackground,
		Prompt:       prompt,
	})
}

// EmitAgentComplete is a convenience helper for agent_complete events.
func EmitAgentComplete(agentID string, duration time.Duration, turns int, isError bool, stopReason string) {
	Emit(EventAgentComplete, AgentCompletePayload{
		AgentID:    agentID,
		DurationMs: duration.Milliseconds(),
		Turns:      turns,
		IsError:    isError,
		StopReason: stopReason,
	})
}

// EmitSkillInvoke is a convenience helper for skill_invoke events.
func EmitSkillInvoke(name, args, source string, allowedTools []string, promptLen int) {
	Emit(EventSkillInvoke, SkillInvokePayload{
		SkillName:    name,
		Args:         args,
		Source:       source,
		AllowedTools: allowedTools,
		PromptLen:    promptLen,
	})
}

// EmitAPICall is a convenience helper for api_call events.
func EmitAPICall(payload APICallPayload) {
	Emit(EventAPICall, payload)
}

// EmitError is a convenience helper for error events.
func EmitError(source, toolName, message string) {
	Emit(EventError, ErrorPayload{
		Source:   source,
		ToolName: toolName,
		Message:  message,
	})
}

// NewTraceID returns a short unique ID for correlating start/end pairs.
func NewTraceID() string {
	return uuid.NewString()[:8]
}
