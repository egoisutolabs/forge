package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/egoisutolabs/forge/api"
	"github.com/egoisutolabs/forge/engine"
	"github.com/egoisutolabs/forge/internal/log"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/services/compact"
	"github.com/egoisutolabs/forge/tools"
)

// EngineBridge connects the Forge QueryEngine to the Bubbletea TUI.
// It converts OnEvent callbacks into tea.Msg values via program.Send().
//
// This is the equivalent of Claude Code's streaming event forwarding in
// cli/print.ts that drives the Ink/React REPL updates.
type EngineBridge struct {
	eng     *engine.QueryEngine
	caller  api.Caller
	program *tea.Program // set by Run() after program is created
}

// NewBridge creates an EngineBridge wrapping the given engine and caller.
// It wires OnEvent and PermissionPrompt on the engine so that streaming events
// and permission requests are forwarded to the Bubbletea program once started.
func NewBridge(eng *engine.QueryEngine, caller api.Caller) *EngineBridge {
	b := &EngineBridge{
		eng:    eng,
		caller: caller,
	}
	eng.OnEvent = b.onStreamEvent
	eng.OnToolComplete = b.onToolComplete
	eng.SetPermissionPrompt(b.PermissionPromptFn())
	eng.SetUserPrompt(b.UserPromptFn())
	return b
}

// Model returns the model name from the engine config.
func (b *EngineBridge) Model() string {
	return b.eng.Config().Model
}

// TotalUsage returns the accumulated token usage across all submissions.
func (b *EngineBridge) TotalUsage() models.Usage {
	return b.eng.TotalUsage()
}

// SetModel changes the active model for subsequent API calls.
func (b *EngineBridge) SetModel(model string) {
	b.eng.SetModel(model)
}

// Compact summarizes the conversation history to save context window space.
func (b *EngineBridge) Compact(ctx context.Context) error {
	msgs := b.eng.Messages()
	sysPrompt := b.eng.SystemPrompt()
	compacted, err := compact.CompactConversation(ctx, b.caller, msgs, sysPrompt)
	if err != nil {
		return err
	}
	b.eng.SetMessages(compacted)
	return nil
}

// Submit sends a user prompt to the engine and returns a tea.Msg.
// This is intended to be called inside a tea.Cmd (i.e. from a goroutine).
//
// The streaming events (text deltas, tool calls) are forwarded via program.Send().
// This function blocks until the prompt is fully processed, then returns a
// PromptDoneMsg or ErrorMsg.
//
// ctx is the cancellation context owned by the caller (AppModel). When the user
// presses Ctrl+C or Esc, the AppModel cancels this context, which propagates
// through to eng.SubmitMessage and aborts in-flight API calls.
func (b *EngineBridge) Submit(ctx context.Context, prompt string) tea.Msg {
	result, err := b.eng.SubmitMessage(ctx, b.caller, prompt)
	if err != nil {
		return ErrorMsg{Err: err}
	}
	return PromptDoneMsg{Result: result}
}

// onStreamEvent converts a raw api.StreamEvent into tea messages and sends
// them to the running program. This is the glue between the engine's callback
// model and Bubbletea's message-passing model.
func (b *EngineBridge) onStreamEvent(event api.StreamEvent) {
	if b.program == nil {
		log.Debug("tui/bridge: onStreamEvent dropped (program=nil) type=%s", event.Type)
		return
	}
	switch event.Type {
	case "text_delta":
		log.Debug("tui/bridge: sending StreamTextMsg len=%d", len(event.Text))
		b.program.Send(StreamTextMsg{Text: event.Text})

	case "tool_use":
		// event.Message contains the in-progress tool blocks on the assistant message
		if event.Message != nil {
			for _, block := range event.Message.ToolUseBlocks() {
				detail := extractToolDetail(block.Name, block.Input)
				b.program.Send(ToolStartMsg{
					Name:   block.Name,
					ID:     block.ID,
					Detail: detail,
				})
			}
		}

	case "message_done":
		if event.Message != nil && event.Message.Usage != nil {
			usage := *event.Message.Usage
			b.program.Send(CostUpdateMsg{
				Usage:   usage,
				CostUSD: models.CostForModel(b.eng.Config().Model, b.eng.TotalUsage()),
			})
		}

	case "error":
		if event.Err != nil {
			b.program.Send(ErrorMsg{Err: event.Err})
		}
	}
}

// onToolComplete is called by the engine loop when a tool finishes execution.
// It forwards the result to the TUI as a ToolDoneMsg so the active-tools list
// is updated and the result appears in the conversation.
func (b *EngineBridge) onToolComplete(name, id, result string, isError bool) {
	if b.program == nil {
		log.Debug("tui/bridge: onToolComplete dropped (program=nil) tool=%s id=%s", name, id)
		return
	}
	log.Debug("tui/bridge: sending ToolDoneMsg tool=%s id=%s isError=%v", name, id, isError)
	b.program.Send(ToolDoneMsg{
		ID:      id,
		Name:    name,
		Result:  result,
		IsError: isError,
	})
}

// PermissionPromptFn returns a PermissionPrompt function suitable for
// engine.Config. It blocks the caller goroutine until the user responds
// via the TUI (y/n). The program.Send + channel pattern avoids data races.
func (b *EngineBridge) PermissionPromptFn() func(message string) bool {
	return func(message string) bool {
		if b.program == nil {
			return false
		}
		toolName := extractToolName(message)
		ch := make(chan bool, 1)
		b.program.Send(PermissionRequestMsg{
			ToolName:   toolName,
			Action:     extractAction(toolName, message),
			Detail:     extractDetail(message),
			Risk:       classifyRisk(toolName, message),
			Message:    message,
			ResponseCh: ch,
		})
		return <-ch
	}
}

// UserPromptFn returns a UserPrompt callback for the engine. When called
// by AskUserQuestionTool, it sends an AskUserRequestMsg to the TUI and
// blocks until the user answers via the huh form.
func (b *EngineBridge) UserPromptFn() func(questions []tools.AskQuestion) (map[string]string, error) {
	return func(questions []tools.AskQuestion) (map[string]string, error) {
		if b.program == nil {
			return nil, fmt.Errorf("TUI not available")
		}
		ch := make(chan AskUserResponse, 1)
		b.program.Send(AskUserRequestMsg{
			Questions:  questions,
			ResponseCh: ch,
		})
		resp := <-ch
		return resp.Answers, resp.Err
	}
}

// extractToolName pulls a tool name out of a permission message.
// Falls back to "tool" if the message doesn't contain one.
func extractToolName(msg string) string {
	for i, c := range msg {
		if c == ':' {
			return msg[:i]
		}
		if i > 32 {
			break
		}
	}
	return "tool"
}

// extractAction returns a human-readable action description.
func extractAction(toolName, _ string) string {
	switch toolName {
	case "Bash":
		return "Execute command"
	case "Edit":
		return "Modify file"
	case "Write":
		return "Create/overwrite file"
	case "Read":
		return "Read file"
	case "Grep":
		return "Search file contents"
	case "Glob":
		return "Search for files"
	case "Agent":
		return "Spawn sub-agent"
	case "WebFetch":
		return "Fetch URL"
	case "WebSearch":
		return "Search the web"
	default:
		return "Execute tool"
	}
}

// extractDetail pulls the specific command/path from a permission message.
func extractDetail(msg string) string {
	// After "ToolName: ", the rest is the detail
	for i, c := range msg {
		if c == ':' && i+2 < len(msg) {
			return strings.TrimSpace(msg[i+1:])
		}
		if i > 32 {
			break
		}
	}
	return msg
}

// classifyRisk determines the risk level of a tool operation.
func classifyRisk(toolName, message string) RiskLevel {
	switch toolName {
	case "Read", "Grep", "Glob":
		return RiskLow
	case "Edit", "Write":
		return RiskModerate
	case "Bash":
		lower := strings.ToLower(message)
		dangerousPatterns := []string{
			"rm ", "rm\t", "sudo ", "kill ", "chmod ", "chown ",
			"mv ", "dd ", "mkfs", "> /", "curl | sh", "curl|sh",
			"--force", "--hard", "push -f", "push --force",
		}
		for _, pat := range dangerousPatterns {
			if strings.Contains(lower, pat) {
				return RiskHigh
			}
		}
		return RiskModerate
	case "Agent":
		return RiskModerate
	default:
		return RiskModerate
	}
}

// extractToolDetail parses the Input JSON of a tool_use block to extract
// a human-readable detail string shown in the activity panel.
func extractToolDetail(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}

	switch toolName {
	case "Bash":
		if cmd, ok := m["command"].(string); ok {
			if len(cmd) > 60 {
				cmd = cmd[:57] + "..."
			}
			return "`" + cmd + "`"
		}
	case "Read":
		if fp, ok := m["file_path"].(string); ok {
			return shortenPath(fp)
		}
	case "Edit":
		if fp, ok := m["file_path"].(string); ok {
			return shortenPath(fp)
		}
	case "Write":
		if fp, ok := m["file_path"].(string); ok {
			return shortenPath(fp)
		}
	case "Grep":
		if pat, ok := m["pattern"].(string); ok {
			detail := `"` + pat + `"`
			if path, ok := m["path"].(string); ok {
				detail += " in " + shortenPath(path)
			}
			return detail
		}
	case "Glob":
		if pat, ok := m["pattern"].(string); ok {
			return pat
		}
	case "Agent":
		if desc, ok := m["description"].(string); ok {
			return desc
		}
		if prompt, ok := m["prompt"].(string); ok {
			if len(prompt) > 50 {
				prompt = prompt[:47] + "..."
			}
			return prompt
		}
	case "WebFetch":
		if url, ok := m["url"].(string); ok {
			if len(url) > 60 {
				url = url[:57] + "..."
			}
			return url
		}
	case "WebSearch":
		if q, ok := m["query"].(string); ok {
			return `"` + q + `"`
		}
	}
	return ""
}

// shortenPath trims a file path to show just the last 2 components.
func shortenPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) <= 3 {
		return p
	}
	return ".../" + strings.Join(parts[len(parts)-2:], "/")
}

// MockCaller is a Caller implementation that returns canned responses.
// Used during development and in tests without a live API key.
type MockCaller struct {
	Response string
}

// Stream returns a single text delta followed by a message_done.
func (m *MockCaller) Stream(_ context.Context, params api.StreamParams) <-chan api.StreamEvent {
	ch := make(chan api.StreamEvent, 4)
	go func() {
		defer close(ch)

		resp := m.Response
		if resp == "" {
			// Echo the last user message for testing
			for i := len(params.Messages) - 1; i >= 0; i-- {
				if params.Messages[i].Role == models.RoleUser {
					resp = fmt.Sprintf("Echo: %s", params.Messages[i].TextContent())
					break
				}
			}
			if resp == "" {
				resp = "Hello from Forge!"
			}
		}

		ch <- api.StreamEvent{Type: "text_delta", Text: resp}
		ch <- api.StreamEvent{
			Type: "message_done",
			Message: &models.Message{
				Role: models.RoleAssistant,
				Content: []models.Block{
					{Type: models.BlockText, Text: resp},
				},
				Usage: &models.Usage{
					InputTokens:  len(params.Messages) * 10,
					OutputTokens: len(resp) / 4,
				},
			},
		}
	}()
	return ch
}
