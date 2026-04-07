package e2e

import (
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/internal/lsp"
	lsptool "github.com/egoisutolabs/forge/internal/tools/lsp"
)

// TestLSPTool_RegisteredInToolsList verifies the LSP tool can be instantiated and has correct metadata.
func TestLSPTool_RegisteredInToolsList(t *testing.T) {
	tool := &lsptool.Tool{}

	if tool.Name() != "LSP" {
		t.Errorf("Name() = %q, want LSP", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if !tool.IsReadOnly(nil) {
		t.Error("LSP tool should be read-only")
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Error("LSP tool should be concurrency-safe")
	}
}

// TestLSPTool_InputSchemaValidJSON verifies the InputSchema is valid JSON with the action enum.
func TestLSPTool_InputSchemaValidJSON(t *testing.T) {
	tool := &lsptool.Tool{}
	schema := tool.InputSchema()

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema type = %v, want object", parsed["type"])
	}

	// Verify required fields.
	required, ok := parsed["required"].([]any)
	if !ok {
		t.Fatal("required field missing or not array")
	}
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	if !requiredSet["action"] || !requiredSet["file_path"] {
		t.Error("required should include 'action' and 'file_path'")
	}

	// Verify action enum values.
	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties missing")
	}
	actionProp, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatal("action property missing")
	}
	enumRaw, ok := actionProp["enum"].([]any)
	if !ok {
		t.Fatal("action enum missing")
	}
	expectedActions := map[string]bool{
		"diagnostics":       true,
		"definition":        true,
		"references":        true,
		"hover":             true,
		"completion":        true,
		"symbols":           true,
		"workspace_symbols": true,
		"rename_preview":    true,
	}
	for _, a := range enumRaw {
		delete(expectedActions, a.(string))
	}
	if len(expectedActions) > 0 {
		t.Errorf("missing actions in enum: %v", expectedActions)
	}
}

// TestLSPTool_AllActionsValidate verifies ValidateInput accepts all known actions with correct params.
func TestLSPTool_AllActionsValidate(t *testing.T) {
	tool := &lsptool.Tool{}

	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"diagnostics", `{"action":"diagnostics","file_path":"/tmp/test.go"}`, true},
		{"definition", `{"action":"definition","file_path":"/tmp/test.go","line":10,"character":5}`, true},
		{"references", `{"action":"references","file_path":"/tmp/test.go","line":10,"character":5}`, true},
		{"hover", `{"action":"hover","file_path":"/tmp/test.go","line":10,"character":5}`, true},
		{"completion", `{"action":"completion","file_path":"/tmp/test.go","line":10,"character":5}`, true},
		{"symbols", `{"action":"symbols","file_path":"/tmp/test.go"}`, true},
		{"workspace_symbols", `{"action":"workspace_symbols","file_path":"/tmp/test.go","query":"Foo"}`, true},
		{"rename_preview", `{"action":"rename_preview","file_path":"/tmp/test.go","line":10,"character":5,"new_name":"Bar"}`, true},

		// Invalid cases.
		{"empty_action", `{"action":"","file_path":"/tmp/test.go"}`, false},
		{"unknown_action", `{"action":"unknown","file_path":"/tmp/test.go"}`, false},
		{"missing_file_path", `{"action":"diagnostics","file_path":""}`, false},
		{"definition_no_line", `{"action":"definition","file_path":"/tmp/test.go"}`, false},
		{"rename_no_name", `{"action":"rename_preview","file_path":"/tmp/test.go","line":1,"character":1}`, false},
		{"workspace_symbols_no_query", `{"action":"workspace_symbols","file_path":"/tmp/test.go"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(json.RawMessage(tt.input))
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// TestLSP_ManagerExtensionRouting verifies that the Manager routes extensions to correct servers.
func TestLSP_ManagerExtensionRouting(t *testing.T) {
	configs := lsp.DefaultConfigs()

	// Build extension → server name mapping (same logic as Manager).
	extToServer := make(map[string]string)
	for _, cfg := range configs {
		for ext := range cfg.Extensions {
			if _, exists := extToServer[ext]; !exists {
				extToServer[ext] = cfg.Name
			}
		}
	}

	tests := []struct {
		ext      string
		wantName string
	}{
		{".go", "gopls"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "typescript"},
		{".jsx", "typescript"},
		{".py", "pyright"},
		{".pyi", "pyright"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			name, ok := extToServer[tt.ext]
			if !ok {
				t.Fatalf("extension %s not routed to any server", tt.ext)
			}
			if name != tt.wantName {
				t.Errorf("extension %s → %q, want %q", tt.ext, name, tt.wantName)
			}
		})
	}
}

// TestLSP_DefaultConfigsComplete verifies DefaultConfigs covers expected ecosystems.
func TestLSP_DefaultConfigsComplete(t *testing.T) {
	configs := lsp.DefaultConfigs()
	names := make(map[string]bool)
	for _, cfg := range configs {
		names[cfg.Name] = true
		if cfg.Command == "" {
			t.Errorf("config %q has empty Command", cfg.Name)
		}
		if len(cfg.Extensions) == 0 {
			t.Errorf("config %q has no extensions", cfg.Name)
		}
	}

	expected := []string{"gopls", "typescript", "pyright", "pylsp"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("DefaultConfigs missing %q", name)
		}
	}
}

// TestLSP_BinaryDetectionInstallHints verifies InstallHint returns known hints.
func TestLSP_BinaryDetectionInstallHints(t *testing.T) {
	tests := []struct {
		command string
		wantNon string // must contain this substring
	}{
		{"gopls", "go install"},
		{"typescript-language-server", "npm install"},
		{"pyright-langserver", "pip install"},
		{"pylsp", "pip install"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			hint := lsp.InstallHint(tt.command)
			if hint == "" {
				t.Errorf("InstallHint(%q) returned empty", tt.command)
			}
			if len(tt.wantNon) > 0 {
				found := false
				if len(hint) > 0 {
					found = true
					for _, sub := range []string{tt.wantNon} {
						if !contains(hint, sub) {
							found = false
						}
					}
				}
				if !found {
					t.Errorf("InstallHint(%q) = %q, want contains %q", tt.command, hint, tt.wantNon)
				}
			}
		})
	}

	// Unknown binary should return empty.
	if hint := lsp.InstallHint("nonexistent-lsp"); hint != "" {
		t.Errorf("InstallHint(nonexistent-lsp) = %q, want empty", hint)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
