package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigs(t *testing.T) {
	configs := DefaultConfigs()
	if len(configs) != 4 {
		t.Fatalf("DefaultConfigs() returned %d configs, want 4", len(configs))
	}

	names := map[string]bool{}
	for _, cfg := range configs {
		names[cfg.Name] = true
	}

	expected := []string{"gopls", "typescript", "pyright", "pylsp"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing config %q", name)
		}
	}
}

func TestDefaultConfigExtensions(t *testing.T) {
	configs := DefaultConfigs()

	// Check gopls extensions.
	var gopls ServerConfig
	for _, cfg := range configs {
		if cfg.Name == "gopls" {
			gopls = cfg
			break
		}
	}
	if gopls.Extensions[".go"] != "go" {
		t.Errorf("gopls .go extension = %q, want %q", gopls.Extensions[".go"], "go")
	}

	// Check typescript extensions.
	var ts ServerConfig
	for _, cfg := range configs {
		if cfg.Name == "typescript" {
			ts = cfg
			break
		}
	}
	if ts.Extensions[".ts"] != "typescript" {
		t.Errorf("typescript .ts extension = %q, want %q", ts.Extensions[".ts"], "typescript")
	}
	if ts.Extensions[".tsx"] != "typescriptreact" {
		t.Errorf("typescript .tsx extension = %q, want %q", ts.Extensions[".tsx"], "typescriptreact")
	}
}

func TestHasAnyFile(t *testing.T) {
	dir := t.TempDir()

	// No files exist.
	if hasAnyFile(dir, []string{"go.mod", "go.sum"}) {
		t.Error("expected false when no files exist")
	}

	// Create go.mod.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}

	if !hasAnyFile(dir, []string{"go.mod", "go.sum"}) {
		t.Error("expected true when go.mod exists")
	}
}

func TestDetectConfigsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	configs := DetectConfigs(dir)
	if len(configs) != 0 {
		t.Errorf("DetectConfigs on empty dir returned %d configs, want 0", len(configs))
	}
}

func TestServerConfigDefaults(t *testing.T) {
	configs := DefaultConfigs()
	for _, cfg := range configs {
		if cfg.StartupTimeout == 0 {
			t.Errorf("config %q has zero StartupTimeout", cfg.Name)
		}
		if cfg.MaxCrashes == 0 {
			t.Errorf("config %q has zero MaxCrashes", cfg.Name)
		}
	}
}
