package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ServerState represents the lifecycle state of a language server.
type ServerState int

const (
	StateStopped ServerState = iota
	StateStarting
	StateRunning
	StateStopping
	StateError
)

var (
	// ErrServerCrashed is returned when the server has exceeded max crashes.
	ErrServerCrashed = errors.New("language server crashed too many times")

	// ErrServerNotRunning is returned for requests when the server isn't running.
	ErrServerNotRunning = errors.New("language server is not running")

	// ContentModifiedCode is the JSON-RPC error code for content modified.
	ContentModifiedCode = -32801
)

// Server wraps a Client with LSP protocol lifecycle.
type Server struct {
	config     ServerConfig
	client     *Client
	state      ServerState
	mu         sync.Mutex
	caps       ServerCapabilities
	initErr    error
	crashCount int
	maxCrashes int
	workDir    string
}

// NewServer creates a new Server with the given config.
func NewServer(config ServerConfig) *Server {
	maxCrashes := config.MaxCrashes
	if maxCrashes <= 0 {
		maxCrashes = 3
	}
	return &Server{
		config:     config,
		state:      StateStopped,
		maxCrashes: maxCrashes,
	}
}

// Start spawns the language server and performs the initialize handshake.
func (s *Server) Start(ctx context.Context, workDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateRunning {
		return nil
	}
	if s.state == StateError && s.crashCount >= s.maxCrashes {
		return ErrServerCrashed
	}

	s.state = StateStarting
	s.workDir = workDir

	// Build env from config.
	var env []string
	for k, v := range s.config.Env {
		env = append(env, k+"="+v)
	}

	client := NewClient()
	if err := client.Start(s.config.Command, s.config.Args, env, workDir); err != nil {
		s.state = StateError
		s.initErr = err
		return fmt.Errorf("start %s: %w", s.config.Name, err)
	}
	s.client = client

	// Perform initialize handshake with timeout.
	timeout := s.config.StartupTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	initCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	initParams := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   PathToURI(workDir),
		WorkspaceFolders: []WorkspaceFolder{
			{URI: PathToURI(workDir), Name: filepath.Base(workDir)},
		},
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Synchronization:    SyncCapabilities{DidSave: true},
				PublishDiagnostics: DiagnosticsCapabilities{RelatedInformation: true},
				Hover:              HoverCapabilities{ContentFormat: []string{"markdown", "plaintext"}},
				Definition:         DefinitionCapabilities{LinkSupport: true},
				References:         ReferencesCapabilities{},
				DocumentSymbol:     DocumentSymbolCapabilities{HierarchicalSupport: true},
				Completion:         CompletionCapabilities{SnippetSupport: false},
			},
			General: GeneralCapabilities{PositionEncodings: []string{"utf-16"}},
		},
		InitializationOptions: s.config.InitOptions,
	}

	result, err := client.Request(initCtx, "initialize", initParams)
	if err != nil {
		_ = client.Close()
		s.state = StateError
		s.initErr = err
		return fmt.Errorf("initialize %s: %w", s.config.Name, err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(result, &initResult); err != nil {
		_ = client.Close()
		s.state = StateError
		s.initErr = err
		return fmt.Errorf("parse initialize result: %w", err)
	}
	s.caps = initResult.Capabilities

	// Send initialized notification.
	if err := client.Notify(initCtx, "initialized", struct{}{}); err != nil {
		_ = client.Close()
		s.state = StateError
		s.initErr = err
		return fmt.Errorf("initialized notification: %w", err)
	}

	// Send workspace/didChangeConfiguration if settings are defined.
	if s.config.Settings != nil {
		_ = client.Notify(initCtx, "workspace/didChangeConfiguration", map[string]any{
			"settings": s.config.Settings,
		})
	}

	s.state = StateRunning

	// Monitor for crashes.
	go s.monitorProcess()

	return nil
}

// SendRequest sends an LSP request, with retry for content-modified errors.
func (s *Server) SendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	state := s.state
	client := s.client
	s.mu.Unlock()

	if state != StateRunning || client == nil {
		return nil, ErrServerNotRunning
	}

	// Retry up to 3 times for content-modified errors.
	var lastErr error
	backoff := 200 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		result, err := client.Request(ctx, method, params)
		if err == nil {
			return result, nil
		}

		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == ContentModifiedCode {
			lastErr = err
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
				continue
			}
		}
		return nil, err
	}
	return nil, lastErr
}

// SendNotification sends an LSP notification (fire-and-forget).
func (s *Server) SendNotification(ctx context.Context, method string, params any) error {
	s.mu.Lock()
	state := s.state
	client := s.client
	s.mu.Unlock()

	if state != StateRunning || client == nil {
		return ErrServerNotRunning
	}

	return client.Notify(ctx, method, params)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning || s.client == nil {
		s.state = StateStopped
		return nil
	}

	s.state = StateStopping
	err := s.client.Close()
	s.state = StateStopped
	s.client = nil
	return err
}

// State returns the current server state.
func (s *Server) State() ServerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Capabilities returns the server's declared capabilities.
func (s *Server) Capabilities() ServerCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.caps
}

// Config returns the server's configuration.
func (s *Server) Config() ServerConfig {
	return s.config
}

// monitorProcess watches for the server process to exit unexpectedly.
func (s *Server) monitorProcess() {
	if s.client == nil {
		return
	}
	<-s.client.done

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateStopping || s.state == StateStopped {
		return // expected shutdown
	}

	s.crashCount++
	if s.crashCount >= s.maxCrashes {
		s.state = StateError
		s.initErr = ErrServerCrashed
	} else {
		s.state = StateError
	}
}
