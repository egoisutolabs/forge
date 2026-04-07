package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/egoisutolabs/forge/internal/config"
	log "github.com/egoisutolabs/forge/internal/logger"
)

// OpenAICaller implements api.Caller using the OpenAI ChatCompletion API.
// Compatible with any provider that speaks the OpenAI protocol
// (OpenRouter, Ollama, vLLM, Together, Groq, local llama.cpp, etc.).
type OpenAICaller struct {
	baseURL    string
	apiKey     string
	model      string
	headers    map[string]string
	httpClient *http.Client
}

// NewOpenAICaller creates an OpenAICaller from a config.Provider.
func NewOpenAICaller(p *config.Provider) *OpenAICaller {
	baseURL := strings.TrimRight(p.BaseURL, "/")
	model := ""
	if len(p.Models) > 0 {
		model = p.Models[0]
	}
	return &OpenAICaller{
		baseURL:    baseURL,
		apiKey:     p.APIKey,
		model:      model,
		headers:    p.Headers,
		httpClient: &http.Client{},
	}
}

// Stream implements api.Caller. It sends a streaming ChatCompletion request
// and converts SSE events into StreamEvents on the returned channel.
func (c *OpenAICaller) Stream(ctx context.Context, params StreamParams) <-chan StreamEvent {
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

// --- request body types ---

type openaiRequest struct {
	Model         string          `json:"model"`
	Messages      []openaiMessage `json:"messages"`
	Tools         []openaiTool    `json:"tools,omitempty"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
	Stream        bool            `json:"stream"`
	StreamOptions *streamOpts     `json:"stream_options,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

func (c *OpenAICaller) stream(ctx context.Context, params StreamParams, ch chan<- StreamEvent) error {
	model := params.Model
	if model == "" {
		model = c.model
	}
	maxTokens := params.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	// Translate messages and tools to OpenAI format.
	oaiMsgs := toOpenAIMessages(params.SystemPrompt, params.Messages)
	var oaiTools []openaiTool
	if len(params.Tools) > 0 {
		oaiTools = toOpenAITools(params.Tools)
	}

	body := openaiRequest{
		Model:     model,
		Messages:  oaiMsgs,
		Tools:     oaiTools,
		MaxTokens: maxTokens,
		Stream:    true,
		StreamOptions: &streamOpts{
			IncludeUsage: true,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	apiURL := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Auth: Bearer token (OpenAI convention).
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	// Custom headers (e.g. X-Title for OpenRouter).
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	log.Debug("OpenAI request: model=%s messages=%d tools=%d max_tokens=%d",
		model, len(oaiMsgs), len(oaiTools), maxTokens)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debug("OpenAI request failed: %v", err)
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	log.Debug("OpenAI response: status=%d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		// Try to extract an error message from the response.
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			log.Debug("OpenAI error: %d %s", resp.StatusCode, errResp.Error.Message)
			return fmt.Errorf("API error %d: %s", resp.StatusCode, errResp.Error.Message)
		}
		bodyPreview := string(respBody)
		if len(bodyPreview) > 512 {
			bodyPreview = bodyPreview[:512] + "...(truncated)"
		}
		log.Debug("OpenAI error: status %d body=%s", resp.StatusCode, bodyPreview)
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	return parseOpenAISSEStream(resp.Body, ch)
}
