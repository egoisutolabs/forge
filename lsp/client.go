package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	// ErrServerDead is returned when the language server process has exited.
	ErrServerDead = errors.New("language server process has exited")

	// ErrRequestCanceled is returned when the request context is canceled.
	ErrRequestCanceled = errors.New("request canceled")
)

// NotificationHandler handles a server-initiated notification.
type NotificationHandler func(method string, params json.RawMessage)

// jsonrpcRequest is a JSON-RPC 2.0 request.
type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError is a JSON-RPC 2.0 error.
type jsonrpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// RPCError is a JSON-RPC error with an accessible error code.
type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Client is a JSON-RPC 2.0 client over stdio.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex // serializes writes to stdin
	nextID  atomic.Int64

	pendingMu sync.Mutex
	pending   map[int64]chan *jsonrpcResponse

	notifyMu sync.RWMutex
	notify   map[string]NotificationHandler

	done chan struct{} // closed when process exits
	err  error         // first fatal error
}

// NewClient creates a Client that will spawn the given command.
func NewClient() *Client {
	return &Client{
		pending: make(map[int64]chan *jsonrpcResponse),
		notify:  make(map[string]NotificationHandler),
		done:    make(chan struct{}),
	}
}

// Start spawns the subprocess and begins reading responses.
func (c *Client) Start(command string, args []string, env []string, dir string) error {
	c.cmd = exec.Command(command, args...)
	c.cmd.Dir = dir
	if len(env) > 0 {
		c.cmd.Env = append(c.cmd.Environ(), env...)
	}

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	go c.readLoop()
	go c.waitLoop()

	return nil
}

// Request sends a JSON-RPC request and blocks until the response arrives.
func (c *Client) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-c.done:
		return nil, ErrServerDead
	default:
	}

	id := c.nextID.Add(1)
	ch := make(chan *jsonrpcResponse, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}
	if err := c.writeMessage(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, &RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ErrRequestCanceled
	case <-c.done:
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ErrServerDead
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	select {
	case <-c.done:
		return ErrServerDead
	default:
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

// OnNotification registers a handler for server-initiated notifications.
func (c *Client) OnNotification(method string, handler NotificationHandler) {
	c.notifyMu.Lock()
	c.notify[method] = handler
	c.notifyMu.Unlock()
}

// Close sends shutdown+exit and kills the process after a timeout.
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}

	// Best-effort shutdown: send shutdown request, then exit notification.
	ctx := context.Background()
	_, _ = c.Request(ctx, "shutdown", nil)
	_ = c.Notify(ctx, "exit", nil)

	// Close stdin to signal EOF.
	_ = c.stdin.Close()

	// Wait for process exit; kill if it doesn't cooperate.
	select {
	case <-c.done:
		return nil
	default:
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-c.done
		return nil
	}
}

// writeMessage serializes a JSON-RPC message with Content-Length framing.
func (c *Client) writeMessage(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := io.WriteString(c.stdin, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.stdin.Write(body); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// readLoop reads JSON-RPC messages from stdout and dispatches them.
func (c *Client) readLoop() {
	reader := bufio.NewReader(c.stdout)
	for {
		body, err := readMessage(reader)
		if err != nil {
			return // process exited or pipe closed
		}

		// Try to parse as a response (has "id" field).
		var msg struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		if msg.ID != nil && msg.Method == "" {
			// It's a response to our request.
			var resp jsonrpcResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				continue
			}
			c.pendingMu.Lock()
			ch, ok := c.pending[*msg.ID]
			if ok {
				delete(c.pending, *msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- &resp
			}
		} else if msg.Method != "" {
			// It's a notification from the server.
			c.notifyMu.RLock()
			handler := c.notify[msg.Method]
			c.notifyMu.RUnlock()
			if handler != nil {
				handler(msg.Method, msg.Params)
			}
		}
	}
}

// maxMessageSize is the maximum allowed Content-Length for an LSP message (50 MB).
// This prevents a malicious or buggy server from causing an OOM via a huge Content-Length.
const maxMessageSize = 50 * 1024 * 1024

// readMessage reads a single Content-Length-framed message.
func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLen := -1

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %s", val)
			}
			contentLen = n
		}
	}

	if contentLen < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	if contentLen > maxMessageSize {
		return nil, fmt.Errorf("Content-Length %d exceeds maximum %d", contentLen, maxMessageSize)
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}

// waitLoop waits for the process to exit and cleans up.
func (c *Client) waitLoop() {
	err := c.cmd.Wait()
	c.err = err
	close(c.done)

	// Fail all pending requests.
	c.pendingMu.Lock()
	for id, ch := range c.pending {
		ch <- &jsonrpcResponse{
			Error: &jsonrpcError{Code: -1, Message: "server exited"},
		}
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
}
