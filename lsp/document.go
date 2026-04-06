package lsp

import (
	"context"
	"path/filepath"
	"sync"
)

// OpenDocument tracks the state of a document open in a language server.
type OpenDocument struct {
	URI        string
	LanguageID string
	Version    int32
	ServerName string
}

// DocumentTracker tracks open documents across all language servers.
type DocumentTracker struct {
	mu   sync.Mutex
	docs map[string]*OpenDocument // absolute path → state
}

// NewDocumentTracker creates a new DocumentTracker.
func NewDocumentTracker() *DocumentTracker {
	return &DocumentTracker{
		docs: make(map[string]*OpenDocument),
	}
}

// Open sends textDocument/didOpen if not already open. Returns true if newly opened.
func (dt *DocumentTracker) Open(ctx context.Context, server *Server, path string, langID string, content string) (bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()

	if _, exists := dt.docs[abs]; exists {
		return false, nil // already open
	}

	uri := PathToURI(abs)
	doc := &OpenDocument{
		URI:        uri,
		LanguageID: langID,
		Version:    1,
		ServerName: server.config.Name,
	}

	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: langID,
			Version:    doc.Version,
			Text:       content,
		},
	}

	if err := server.SendNotification(ctx, "textDocument/didOpen", params); err != nil {
		return false, err
	}

	dt.docs[abs] = doc
	return true, nil
}

// Change sends textDocument/didChange with full content sync. Auto-opens if needed.
func (dt *DocumentTracker) Change(ctx context.Context, server *Server, path string, langID string, content string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()

	doc, exists := dt.docs[abs]
	if !exists {
		// Auto-open first.
		uri := PathToURI(abs)
		doc = &OpenDocument{
			URI:        uri,
			LanguageID: langID,
			Version:    1,
			ServerName: server.config.Name,
		}

		openParams := DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{
				URI:        uri,
				LanguageID: langID,
				Version:    doc.Version,
				Text:       content,
			},
		}
		if err := server.SendNotification(ctx, "textDocument/didOpen", openParams); err != nil {
			return err
		}
		dt.docs[abs] = doc
		return nil
	}

	doc.Version++
	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     doc.URI,
			Version: doc.Version,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: content},
		},
	}

	return server.SendNotification(ctx, "textDocument/didChange", params)
}

// Save sends textDocument/didSave.
func (dt *DocumentTracker) Save(ctx context.Context, server *Server, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	dt.mu.Lock()
	doc, exists := dt.docs[abs]
	dt.mu.Unlock()

	if !exists {
		return nil // not tracked, nothing to save
	}

	params := DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: doc.URI},
	}
	return server.SendNotification(ctx, "textDocument/didSave", params)
}

// Close sends textDocument/didClose and removes tracking.
func (dt *DocumentTracker) Close(ctx context.Context, server *Server, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	dt.mu.Lock()
	doc, exists := dt.docs[abs]
	if exists {
		delete(dt.docs, abs)
	}
	dt.mu.Unlock()

	if !exists {
		return nil
	}

	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: doc.URI},
	}
	return server.SendNotification(ctx, "textDocument/didClose", params)
}

// IsOpen returns whether a file is currently tracked as open.
func (dt *DocumentTracker) IsOpen(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()
	_, exists := dt.docs[abs]
	return exists
}

// Version returns the current version for a tracked document, or 0 if not tracked.
func (dt *DocumentTracker) Version(path string) int32 {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0
	}

	dt.mu.Lock()
	defer dt.mu.Unlock()
	doc, exists := dt.docs[abs]
	if !exists {
		return 0
	}
	return doc.Version
}
