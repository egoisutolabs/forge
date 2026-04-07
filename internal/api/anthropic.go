package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/egoisutolabs/forge/internal/config"
	log "github.com/egoisutolabs/forge/internal/logger"
	"github.com/egoisutolabs/forge/internal/models"
)

const (
	anthropicDefaultURL = "https://api.anthropic.com"
	anthropicVersion    = "2023-06-01"
	defaultMaxTokens    = 8096
)

// AnthropicCaller implements api.Caller against the real Anthropic Messages API.
type AnthropicCaller struct {
	baseURL    string
	apiKey     string
	model      string
	headers    map[string]string
	httpClient *http.Client
}

// NewAnthropicCaller creates an AnthropicCaller with the given API key and default model.
// This is the backward-compatible constructor (no custom base URL or headers).
func NewAnthropicCaller(apiKey, model string) *AnthropicCaller {
	return &AnthropicCaller{
		baseURL:    anthropicDefaultURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// NewAnthropicCallerFromProvider creates an AnthropicCaller from a config.Provider.
func NewAnthropicCallerFromProvider(p *config.Provider) *AnthropicCaller {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = anthropicDefaultURL
	}
	// Pick the first model from the provider's model list as the default.
	model := ""
	if len(p.Models) > 0 {
		model = p.Models[0]
	}
	return &AnthropicCaller{
		baseURL:    baseURL,
		apiKey:     p.APIKey,
		model:      model,
		headers:    p.Headers,
		httpClient: &http.Client{},
	}
}

// Stream implements api.Caller. It sends a streaming request to the Anthropic API
// and converts SSE events into StreamEvents on the returned channel.
func (c *AnthropicCaller) Stream(ctx context.Context, params StreamParams) <-chan StreamEvent {
	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		if err := c.stream(ctx, params, ch); err != nil {
			select {
			case ch <- StreamEvent{Type: "error", Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return ch
}

// --- wire types for API request ---

// apiBlock is the wire format for content blocks sent to/received from the API.
type apiBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// apiMessage is the wire format for messages sent to the API.
type apiMessage struct {
	Role    string     `json:"role"`
	Content []apiBlock `json:"content"`
}

// requestBody is the JSON body sent to POST /v1/messages.
type requestBody struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Messages  []apiMessage `json:"messages"`
	Tools     []ToolSchema `json:"tools,omitempty"`
	Stream    bool         `json:"stream"`
}

func (c *AnthropicCaller) stream(ctx context.Context, params StreamParams, ch chan<- StreamEvent) error {
	model := params.Model
	if model == "" {
		model = c.model
	}
	maxTokens := params.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	// Build API messages. Callers (e.g. RunLoop) are responsible for calling
	// NormalizeForAPI before passing messages; we just convert to wire format.
	apiMsgs := make([]apiMessage, 0, len(params.Messages))
	for _, msg := range params.Messages {
		am := apiMessage{Role: string(msg.Role)}
		for _, b := range msg.Content {
			am.Content = append(am.Content, blockToAPI(b))
		}
		apiMsgs = append(apiMsgs, am)
	}

	body := requestBody{
		Model:     model,
		MaxTokens: maxTokens,
		System:    params.SystemPrompt,
		Messages:  apiMsgs,
		Stream:    true,
	}
	if len(params.Tools) > 0 {
		body.Tools = params.Tools
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	apiURL := strings.TrimRight(c.baseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	log.Debug("API request: model=%s messages=%d tools=%d max_tokens=%d", model, len(apiMsgs), len(params.Tools), maxTokens)
	log.Debug("API request body size: %d bytes", len(bodyBytes))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debug("API request failed: %v", err)
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	log.Debug("API response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error.Message != "" {
			log.Debug("API error: %d %s", resp.StatusCode, errBody.Error.Message)
			return fmt.Errorf("API error %d: %s", resp.StatusCode, errBody.Error.Message)
		}
		log.Debug("API error: status %d (no body)", resp.StatusCode)
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	return parseSSEStream(resp.Body, ch)
}

// blockToAPI converts an internal Block to the API wire format.
func blockToAPI(b models.Block) apiBlock {
	ab := apiBlock{Type: string(b.Type)}
	switch b.Type {
	case models.BlockText:
		ab.Text = b.Text
	case models.BlockToolUse:
		ab.ID = b.ID
		ab.Name = b.Name
		if len(b.Input) > 0 {
			ab.Input = b.Input
		} else {
			ab.Input = json.RawMessage("{}")
		}
	case models.BlockToolResult:
		ab.ToolUseID = b.ToolUseID
		ab.Content = b.Content
		ab.IsError = b.IsError
	}
	return ab
}

// --- SSE parsing ---

// sseBlock tracks an in-flight content block during streaming.
type sseBlock struct {
	blockType string // "text" or "tool_use"
	id        string
	name      string
	textBuf   strings.Builder
	inputBuf  strings.Builder
}

// sseState holds accumulated state across SSE events for a single API response.
type sseState struct {
	msgID       string
	msgModel    string
	stopReason  models.StopReason
	inputUsage  *models.Usage // from message_start
	outputUsage *models.Usage // from message_delta
	blocks      map[int]*sseBlock
	blockOrder  []int
}

func parseSSEStream(body io.Reader, ch chan<- StreamEvent) error {
	state := &sseState{
		blocks: make(map[int]*sseBlock),
	}

	scanner := bufio.NewScanner(body)
	// 1 MB buffer for large events (e.g. tool inputs)
	buf := make([]byte, 1<<20)
	scanner.Buffer(buf, 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		if data == "[DONE]" {
			break
		}
		if err := processSSEEvent(data, state, ch); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func processSSEEvent(data string, state *sseState, ch chan<- StreamEvent) error {
	// Decode only the "type" field first for dispatch.
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return nil // ignore malformed events
	}

	log.Debug("SSE event: %s", envelope.Type)

	switch envelope.Type {
	case "message_start":
		var e struct {
			Message struct {
				ID    string        `json:"id"`
				Model string        `json:"model"`
				Usage *models.Usage `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &e); err == nil {
			state.msgID = e.Message.ID
			state.msgModel = e.Message.Model
			state.inputUsage = e.Message.Usage
		}

	case "content_block_start":
		var e struct {
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &e); err == nil {
			blk := &sseBlock{
				blockType: e.ContentBlock.Type,
				id:        e.ContentBlock.ID,
				name:      e.ContentBlock.Name,
			}
			state.blocks[e.Index] = blk
			state.blockOrder = append(state.blockOrder, e.Index)
		}

	case "content_block_delta":
		var e struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &e); err == nil {
			blk, ok := state.blocks[e.Index]
			if !ok {
				return nil
			}
			switch e.Delta.Type {
			case "text_delta":
				blk.textBuf.WriteString(e.Delta.Text)
				select {
				case ch <- StreamEvent{Type: "text_delta", Text: e.Delta.Text}:
				default:
					ch <- StreamEvent{Type: "text_delta", Text: e.Delta.Text}
				}
			case "input_json_delta":
				blk.inputBuf.WriteString(e.Delta.PartialJSON)
			}
		}

	case "content_block_stop":
		// Block is finalized — no action needed; we build content at message_stop.

	case "message_delta":
		var e struct {
			Delta struct {
				StopReason models.StopReason `json:"stop_reason"`
			} `json:"delta"`
			Usage *models.Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &e); err == nil {
			state.stopReason = e.Delta.StopReason
			state.outputUsage = e.Usage
		}

	case "message_stop":
		msg := buildFinalMessage(state)
		ch <- StreamEvent{Type: "message_done", Message: msg}

	case "error":
		var e struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &e); err == nil && e.Error.Message != "" {
			ch <- StreamEvent{Type: "error", Err: fmt.Errorf("stream error: %s", e.Error.Message)}
		}
	}

	return nil
}

func buildFinalMessage(state *sseState) *models.Message {
	var content []models.Block

	for _, idx := range state.blockOrder {
		blk, ok := state.blocks[idx]
		if !ok {
			continue
		}
		switch blk.blockType {
		case "text":
			if text := blk.textBuf.String(); text != "" {
				content = append(content, models.Block{
					Type: models.BlockText,
					Text: text,
				})
			}
		case "tool_use":
			input := json.RawMessage(blk.inputBuf.String())
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			content = append(content, models.Block{
				Type:  models.BlockToolUse,
				ID:    blk.id,
				Name:  blk.name,
				Input: input,
			})
		}
	}

	// Merge input and output usage into a single Usage struct.
	var usage *models.Usage
	if state.inputUsage != nil || state.outputUsage != nil {
		u := models.Usage{}
		if state.inputUsage != nil {
			u.InputTokens = state.inputUsage.InputTokens
			u.CacheRead = state.inputUsage.CacheRead
			u.CacheCreate = state.inputUsage.CacheCreate
		}
		if state.outputUsage != nil {
			u.OutputTokens = state.outputUsage.OutputTokens
		}
		usage = &u
	}

	msgID := state.msgID
	if msgID == "" {
		msgID = uuid.NewString()
	}

	return &models.Message{
		ID:         msgID,
		Role:       models.RoleAssistant,
		Content:    content,
		Model:      state.msgModel,
		StopReason: state.stopReason,
		Usage:      usage,
		Timestamp:  time.Now(),
	}
}
