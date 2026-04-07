package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
)

// Manager manages multiple language servers and routes files.
type Manager struct {
	mu          sync.RWMutex
	servers     map[string]*Server // config name → server
	extMap      map[string]string  // ".go" → config name
	langMap     map[string]string  // ".go" → language ID
	documents   *DocumentTracker
	diagnostics *DiagnosticRegistry
	workDir     string
	configs     []ServerConfig
	onDiag      func(uri string, diags []Diagnostic)

	// startMu prevents concurrent starts of the same server.
	startMu sync.Map // config name → *sync.Once
}

// NewManager creates a manager with the given configs and workspace root.
func NewManager(workDir string, configs []ServerConfig) *Manager {
	m := &Manager{
		servers:     make(map[string]*Server),
		extMap:      make(map[string]string),
		langMap:     make(map[string]string),
		documents:   NewDocumentTracker(),
		diagnostics: NewDiagnosticRegistry(),
		workDir:     workDir,
		configs:     configs,
	}

	// Build extension → config name mapping.
	for _, cfg := range configs {
		for ext, langID := range cfg.Extensions {
			if _, exists := m.extMap[ext]; !exists {
				m.extMap[ext] = cfg.Name
				m.langMap[ext] = langID
			}
		}
	}

	return m
}

// ServerForFile returns the server handling this file extension.
// Starts the server lazily if not yet running. Returns nil, nil if no server handles this type.
func (m *Manager) ServerForFile(ctx context.Context, filePath string) (*Server, error) {
	ext := filepath.Ext(filePath)
	m.mu.RLock()
	configName, ok := m.extMap[ext]
	m.mu.RUnlock()
	if !ok {
		return nil, nil
	}

	return m.getOrStartServer(ctx, configName)
}

// OpenFile notifies the appropriate server that a file was opened.
func (m *Manager) OpenFile(ctx context.Context, filePath string, content string) error {
	server, err := m.ServerForFile(ctx, filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}

	ext := filepath.Ext(filePath)
	langID := m.langMap[ext]
	_, err = m.documents.Open(ctx, server, filePath, langID, content)
	return err
}

// ChangeFile notifies the server of a content change.
func (m *Manager) ChangeFile(ctx context.Context, filePath string, newContent string) error {
	server, err := m.ServerForFile(ctx, filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}

	ext := filepath.Ext(filePath)
	langID := m.langMap[ext]
	return m.documents.Change(ctx, server, filePath, langID, newContent)
}

// SaveFile notifies the server that a file was saved.
func (m *Manager) SaveFile(ctx context.Context, filePath string) error {
	server, err := m.ServerForFile(ctx, filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}

	return m.documents.Save(ctx, server, filePath)
}

// CloseFile notifies the server that a file was closed.
func (m *Manager) CloseFile(ctx context.Context, filePath string) error {
	server, err := m.ServerForFile(ctx, filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}

	return m.documents.Close(ctx, server, filePath)
}

// IsFileOpen returns whether a file is currently tracked as open.
func (m *Manager) IsFileOpen(filePath string) bool {
	return m.documents.IsOpen(filePath)
}

// SendRequest routes a request to the correct server for the given file.
func (m *Manager) SendRequest(ctx context.Context, filePath string, method string, params any) (json.RawMessage, error) {
	server, err := m.ServerForFile(ctx, filePath)
	if err != nil {
		return nil, err
	}
	if server == nil {
		ext := filepath.Ext(filePath)
		return nil, fmt.Errorf("no language server handles %s files", ext)
	}

	return server.SendRequest(ctx, method, params)
}

// Shutdown gracefully shuts down all servers.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	servers := make([]*Server, 0, len(m.servers))
	for _, s := range m.servers {
		servers = append(servers, s)
	}
	m.mu.Unlock()

	var firstErr error
	for _, s := range servers {
		if err := s.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// OnDiagnostics registers a callback for published diagnostics.
func (m *Manager) OnDiagnostics(fn func(uri string, diags []Diagnostic)) {
	m.mu.Lock()
	m.onDiag = fn
	m.mu.Unlock()
}

// Documents returns the document tracker for external use.
func (m *Manager) Documents() *DocumentTracker {
	return m.documents
}

// Diagnostics returns the diagnostic registry for external use.
func (m *Manager) Diagnostics() *DiagnosticRegistry {
	return m.diagnostics
}

// getOrStartServer returns an existing server or starts a new one.
func (m *Manager) getOrStartServer(ctx context.Context, configName string) (*Server, error) {
	// Fast path: check if server is already running.
	m.mu.RLock()
	server, exists := m.servers[configName]
	m.mu.RUnlock()
	if exists && server.State() == StateRunning {
		return server, nil
	}

	// If server exists but is in error state, try restart if under crash limit.
	if exists && server.State() == StateError {
		s := server
		s.mu.Lock()
		if s.crashCount >= s.maxCrashes {
			s.mu.Unlock()
			return nil, ErrServerCrashed
		}
		s.mu.Unlock()
		// Fall through to start it.
	}

	// Use sync.Once-like pattern to prevent concurrent starts.
	onceVal, _ := m.startMu.LoadOrStore(configName, &sync.Once{})
	once := onceVal.(*sync.Once)

	var startErr error
	once.Do(func() {
		// Find the config.
		var cfg *ServerConfig
		for i := range m.configs {
			if m.configs[i].Name == configName {
				cfg = &m.configs[i]
				break
			}
		}
		if cfg == nil {
			startErr = fmt.Errorf("unknown server config: %s", configName)
			return
		}

		srv := NewServer(*cfg)
		startErr = srv.Start(ctx, m.workDir)
		if startErr != nil {
			return
		}

		// Register publishDiagnostics handler.
		srv.client.OnNotification("textDocument/publishDiagnostics", func(method string, params json.RawMessage) {
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(params, &p); err != nil {
				return
			}
			// Update the diagnostic registry.
			m.diagnostics.Update(p.URI, p.Diagnostics)

			m.mu.RLock()
			handler := m.onDiag
			m.mu.RUnlock()
			if handler != nil {
				handler(p.URI, p.Diagnostics)
			}
		})

		m.mu.Lock()
		m.servers[configName] = srv
		m.mu.Unlock()
	})

	if startErr != nil {
		// Reset the Once so a future attempt can retry.
		m.startMu.Delete(configName)
		return nil, startErr
	}

	m.mu.RLock()
	server = m.servers[configName]
	m.mu.RUnlock()
	return server, nil
}
