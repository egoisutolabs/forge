package lsp

import (
	"context"
	"testing"
)

func TestNewManager(t *testing.T) {
	configs := []ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Args:    []string{"serve"},
			Extensions: map[string]string{
				".go": "go",
			},
		},
		{
			Name:    "typescript",
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			Extensions: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
			},
		},
	}

	m := NewManager("/tmp/project", configs)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.workDir != "/tmp/project" {
		t.Errorf("workDir = %q, want %q", m.workDir, "/tmp/project")
	}
}

func TestManagerExtensionRouting(t *testing.T) {
	configs := []ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Extensions: map[string]string{
				".go":  "go",
				".mod": "go.mod",
			},
		},
		{
			Name:    "typescript",
			Command: "typescript-language-server",
			Extensions: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
				".js":  "javascript",
			},
		},
	}

	m := NewManager("/tmp/project", configs)

	// Check extension mapping.
	tests := []struct {
		ext      string
		wantName string
	}{
		{".go", "gopls"},
		{".mod", "gopls"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "typescript"},
	}

	for _, tt := range tests {
		name, ok := m.extMap[tt.ext]
		if !ok {
			t.Errorf("extension %q not mapped", tt.ext)
			continue
		}
		if name != tt.wantName {
			t.Errorf("extension %q maps to %q, want %q", tt.ext, name, tt.wantName)
		}
	}
}

func TestManagerUnknownExtension(t *testing.T) {
	configs := []ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Extensions: map[string]string{
				".go": "go",
			},
		},
	}

	m := NewManager("/tmp/project", configs)
	ctx := context.Background()

	server, err := m.ServerForFile(ctx, "/tmp/project/test.rb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != nil {
		t.Error("expected nil server for unknown extension")
	}
}

func TestManagerIsFileOpen(t *testing.T) {
	m := NewManager("/tmp/project", nil)
	if m.IsFileOpen("/tmp/test.go") {
		t.Error("expected file not to be open initially")
	}
}

func TestManagerExtensionDedup(t *testing.T) {
	// First config should win for conflicting extensions.
	configs := []ServerConfig{
		{
			Name:    "pyright",
			Command: "pyright-langserver",
			Extensions: map[string]string{
				".py": "python",
			},
		},
		{
			Name:    "pylsp",
			Command: "pylsp",
			Extensions: map[string]string{
				".py": "python",
			},
		},
	}

	m := NewManager("/tmp/project", configs)
	name := m.extMap[".py"]
	if name != "pyright" {
		t.Errorf(".py maps to %q, want %q (first config wins)", name, "pyright")
	}
}

func TestManagerShutdownEmpty(t *testing.T) {
	m := NewManager("/tmp/project", nil)
	ctx := context.Background()
	if err := m.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown on empty manager: %v", err)
	}
}

func TestManagerOnDiagnostics(t *testing.T) {
	m := NewManager("/tmp/project", nil)
	called := false
	m.OnDiagnostics(func(uri string, diags []Diagnostic) {
		called = true
	})
	if called {
		t.Error("callback should not be called before any events")
	}
}

func TestManagerDocuments(t *testing.T) {
	m := NewManager("/tmp/project", nil)
	dt := m.Documents()
	if dt == nil {
		t.Fatal("Documents() returned nil")
	}
}

func TestManagerLanguageMapping(t *testing.T) {
	configs := []ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Extensions: map[string]string{
				".go": "go",
			},
		},
	}

	m := NewManager("/tmp/project", configs)
	langID := m.langMap[".go"]
	if langID != "go" {
		t.Errorf("langMap[.go] = %q, want %q", langID, "go")
	}
}
