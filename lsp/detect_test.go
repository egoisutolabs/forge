package lsp

import (
	"testing"
)

func TestDetectBinaryNotFound(t *testing.T) {
	path := DetectBinary("nonexistent-binary-xyz-123")
	if path != "" {
		t.Errorf("DetectBinary returned %q for nonexistent binary", path)
	}
}

func TestDetectBinaryFound(t *testing.T) {
	// "ls" should exist on all Unix systems.
	path := DetectBinary("ls")
	if path == "" {
		t.Error("DetectBinary(\"ls\") returned empty string")
	}
}

func TestDetectAllServers(t *testing.T) {
	result := DetectAllServers()
	if len(result) == 0 {
		t.Error("DetectAllServers returned empty map")
	}

	// All default configs should be represented.
	for _, cfg := range DefaultConfigs() {
		status, ok := result[cfg.Name]
		if !ok {
			t.Errorf("missing status for %q", cfg.Name)
			continue
		}
		if status.Name != cfg.Name {
			t.Errorf("status.Name = %q, want %q", status.Name, cfg.Name)
		}
	}
}

func TestInstallHint(t *testing.T) {
	hint := InstallHint("gopls")
	if hint == "" {
		t.Error("InstallHint(\"gopls\") returned empty string")
	}

	hint = InstallHint("unknown-server")
	if hint != "" {
		t.Errorf("InstallHint(\"unknown-server\") = %q, want empty", hint)
	}
}
