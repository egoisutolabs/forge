package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestReadMessage(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple message",
			input: "Content-Length: 15\r\n\r\n{\"id\":1,\"ok\":1}",
			want:  `{"id":1,"ok":1}`,
		},
		{
			name:  "message with extra header",
			input: "Content-Length: 2\r\nContent-Type: application/json\r\n\r\n{}",
			want:  "{}",
		},
		{
			name:    "missing content length",
			input:   "\r\n{}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			got, err := readMessage(reader)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("readMessage = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestWriteMessageFormat(t *testing.T) {
	// Test that writeMessage produces correct Content-Length framing
	// by using a pipe to capture the output.
	pr, pw := io.Pipe()

	c := &Client{
		stdin: pw,
	}

	msg := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  "test",
	}
	body, _ := json.Marshal(msg)
	expectedHeader := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	go func() {
		_ = c.writeMessage(msg)
		pw.Close()
	}()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, pr)
	output := buf.String()

	if !strings.HasPrefix(output, expectedHeader) {
		t.Errorf("expected header %q, got prefix %q", expectedHeader, output[:min(len(output), len(expectedHeader)+10)])
	}

	// Parse just the JSON body part.
	bodyStart := strings.Index(output, "\r\n\r\n")
	if bodyStart < 0 {
		t.Fatal("no header/body separator found")
	}
	jsonBody := output[bodyStart+4:]
	var parsed jsonrpcRequest
	if err := json.Unmarshal([]byte(jsonBody), &parsed); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if parsed.Method != "test" {
		t.Errorf("method = %q, want %q", parsed.Method, "test")
	}
}

// TestClientRequestResponse tests the full request/response cycle using pipes.
func TestClientRequestResponse(t *testing.T) {
	// Create a pair of pipes to simulate a language server process.
	clientToServer_r, clientToServer_w := io.Pipe() // client stdin -> server reads
	serverToClient_r, serverToClient_w := io.Pipe() // server writes -> client stdout

	c := &Client{
		stdin:   clientToServer_w,
		stdout:  serverToClient_r,
		pending: make(map[int64]chan *jsonrpcResponse),
		notify:  make(map[string]NotificationHandler),
		done:    make(chan struct{}),
	}

	// Start reading responses.
	go c.readLoop()

	// Mock server: read one request, send one response.
	go func() {
		reader := bufio.NewReader(clientToServer_r)
		body, err := readMessage(reader)
		if err != nil {
			t.Errorf("mock server read error: %v", err)
			return
		}

		var req struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("mock server parse error: %v", err)
			return
		}

		// Build a response.
		resp := jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"hello":"world"}`),
		}
		respBody, _ := json.Marshal(resp)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(respBody))
		_, _ = serverToClient_w.Write([]byte(header))
		_, _ = serverToClient_w.Write(respBody)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := c.Request(ctx, "test/method", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if parsed["hello"] != "world" {
		t.Errorf("result = %v, want hello:world", parsed)
	}

	// Cleanup.
	clientToServer_w.Close()
	serverToClient_w.Close()
}

// TestClientNotification tests that notifications from the server are dispatched.
func TestClientNotification(t *testing.T) {
	_, clientToServer_w := io.Pipe()
	serverToClient_r, serverToClient_w := io.Pipe()

	c := &Client{
		stdin:   clientToServer_w,
		stdout:  serverToClient_r,
		pending: make(map[int64]chan *jsonrpcResponse),
		notify:  make(map[string]NotificationHandler),
		done:    make(chan struct{}),
	}

	received := make(chan string, 1)
	c.OnNotification("textDocument/publishDiagnostics", func(method string, params json.RawMessage) {
		received <- string(params)
	})

	go c.readLoop()

	// Send a notification from the "server".
	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params":  map[string]string{"uri": "file:///test.go"},
	}
	body, _ := json.Marshal(notif)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, _ = serverToClient_w.Write([]byte(header))
	_, _ = serverToClient_w.Write(body)

	select {
	case got := <-received:
		if !strings.Contains(got, "test.go") {
			t.Errorf("notification params = %s, want contains test.go", got)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for notification")
	}

	clientToServer_w.Close()
	serverToClient_w.Close()
}

// TestClientRequestCanceled tests that context cancellation works.
func TestClientRequestCanceled(t *testing.T) {
	clientToServer_r, clientToServer_w := io.Pipe()
	serverToClient_r, serverToClient_w := io.Pipe()

	c := &Client{
		stdin:   clientToServer_w,
		stdout:  serverToClient_r,
		pending: make(map[int64]chan *jsonrpcResponse),
		notify:  make(map[string]NotificationHandler),
		done:    make(chan struct{}),
	}

	go c.readLoop()

	// Drain stdin so the write doesn't block.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := clientToServer_r.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.Request(ctx, "slow/method", nil)
	if err != ErrRequestCanceled {
		t.Errorf("expected ErrRequestCanceled, got %v", err)
	}

	clientToServer_w.Close()
	clientToServer_r.Close()
	serverToClient_w.Close()
}

// TestReadMessage_RejectsHugeContentLength verifies that readMessage rejects
// Content-Length values that exceed the maximum to prevent OOM.
func TestReadMessage_RejectsHugeContentLength(t *testing.T) {
	// 100 MB Content-Length (exceeds maxMessageSize of 50 MB)
	input := "Content-Length: 104857600\r\n\r\n"
	reader := bufio.NewReader(strings.NewReader(input))
	_, err := readMessage(reader)
	if err == nil {
		t.Fatal("expected error for huge Content-Length, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestReadMessage_AcceptsReasonableSize verifies that readMessage accepts
// Content-Length values within the limit.
func TestReadMessage_AcceptsReasonableSize(t *testing.T) {
	body := `{"id":1,"result":{}}`
	input := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	reader := bufio.NewReader(strings.NewReader(input))
	got, err := readMessage(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != body {
		t.Errorf("got %q, want %q", string(got), body)
	}
}

// TestClientRPCError tests that JSON-RPC errors are correctly returned.
func TestClientRPCError(t *testing.T) {
	clientToServer_r, clientToServer_w := io.Pipe()
	serverToClient_r, serverToClient_w := io.Pipe()

	c := &Client{
		stdin:   clientToServer_w,
		stdout:  serverToClient_r,
		pending: make(map[int64]chan *jsonrpcResponse),
		notify:  make(map[string]NotificationHandler),
		done:    make(chan struct{}),
	}

	go c.readLoop()

	go func() {
		reader := bufio.NewReader(clientToServer_r)
		body, _ := readMessage(reader)

		var req struct {
			ID *int64 `json:"id"`
		}
		_ = json.Unmarshal(body, &req)

		resp := jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: -32600, Message: "invalid request"},
		}
		respBody, _ := json.Marshal(resp)
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(respBody))
		_, _ = serverToClient_w.Write([]byte(header))
		_, _ = serverToClient_w.Write(respBody)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Request(ctx, "fail/method", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
	if rpcErr.Code != -32600 {
		t.Errorf("error code = %d, want -32600", rpcErr.Code)
	}

	clientToServer_w.Close()
	serverToClient_w.Close()
}
