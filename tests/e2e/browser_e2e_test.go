package e2e

import (
	"encoding/json"
	"testing"

	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools/browser"
)

// TestBrowser_AllActionsInSchema verifies every action in the InputSchema enum.
func TestBrowser_AllActionsInSchema(t *testing.T) {
	tool := &browser.Tool{}
	schema := tool.InputSchema()

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema not valid JSON: %v", err)
	}

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

	// Collect all actions from schema.
	schemaActions := make(map[string]bool)
	for _, a := range enumRaw {
		schemaActions[a.(string)] = true
	}

	// All expected actions including new ones.
	expectedActions := []string{
		"open", "snapshot", "click", "type", "fill", "press", "wait", "get",
		"screenshot", "scroll", "back", "forward", "reload", "close", "hover",
		"focus", "download", "find", "tab",
		// New enhanced actions.
		"eval", "upload", "viewport", "pdf", "cookies", "console",
	}

	for _, action := range expectedActions {
		if !schemaActions[action] {
			t.Errorf("action %q missing from InputSchema enum", action)
		}
	}

	// Verify count matches.
	if len(schemaActions) != len(expectedActions) {
		t.Errorf("schema has %d actions, expected %d", len(schemaActions), len(expectedActions))
	}
}

// TestBrowser_ReadOnlyActions verifies that read-only actions return IsReadOnly=true.
func TestBrowser_ReadOnlyActions(t *testing.T) {
	tool := &browser.Tool{}

	readOnlyActions := []struct {
		name  string
		input string
	}{
		{"snapshot", `{"action":"snapshot"}`},
		{"get", `{"action":"get","get":"title"}`},
		{"wait", `{"action":"wait","target":"#elem"}`},
		{"scroll", `{"action":"scroll","direction":"down"}`},
		{"back", `{"action":"back"}`},
		{"forward", `{"action":"forward"}`},
		{"reload", `{"action":"reload"}`},
		{"close", `{"action":"close"}`},
		{"console", `{"action":"console"}`},
		{"cookies_get", `{"action":"cookies"}`},
		{"tab_list", `{"action":"tab","tab_action":"list"}`},
		{"tab_switch", `{"action":"tab","tab_action":"switch","tab_index":0}`},
	}

	for _, tt := range readOnlyActions {
		t.Run(tt.name, func(t *testing.T) {
			if !tool.IsReadOnly(json.RawMessage(tt.input)) {
				t.Errorf("action %s should be read-only", tt.name)
			}
		})
	}
}

// TestBrowser_WriteActions verifies that write actions return PermAsk.
func TestBrowser_WriteActions(t *testing.T) {
	tool := &browser.Tool{}

	writeActions := []struct {
		name  string
		input string
	}{
		{"open", `{"action":"open","url":"https://example.com"}`},
		{"click", `{"action":"click","selector":"#btn"}`},
		{"type", `{"action":"type","selector":"#input","text":"hello"}`},
		{"fill", `{"action":"fill","selector":"#input","text":"hello"}`},
		{"press", `{"action":"press","key":"Enter"}`},
		{"screenshot", `{"action":"screenshot"}`},
		{"hover", `{"action":"hover","selector":"#elem"}`},
		{"focus", `{"action":"focus","selector":"#elem"}`},
		{"download", `{"action":"download","selector":"#link","path":"/tmp/file"}`},
		{"find", `{"action":"find","locator":"role","value":"button","find_action":"click"}`},
		{"tab_new", `{"action":"tab","tab_action":"new"}`},
		{"tab_close", `{"action":"tab","tab_action":"close"}`},
		{"eval", `{"action":"eval","js":"document.title"}`},
		{"upload", `{"action":"upload","selector":"#file","file_path":"/tmp/test.txt"}`},
		{"viewport", `{"action":"viewport","width":1920,"height":1080}`},
		{"pdf", `{"action":"pdf","path":"/tmp/out.pdf"}`},
		{"cookies_set", `{"action":"cookies","cookies":[{"name":"s","value":"v"}]}`},
	}

	for _, tt := range writeActions {
		t.Run(tt.name, func(t *testing.T) {
			perm, err := tool.CheckPermissions(json.RawMessage(tt.input), nil)
			if err != nil {
				t.Fatalf("CheckPermissions: %v", err)
			}
			if perm.Behavior != models.PermAsk {
				t.Errorf("action %s: Behavior = %q, want ask", tt.name, perm.Behavior)
			}

			// Write actions should also NOT be read-only.
			if tool.IsReadOnly(json.RawMessage(tt.input)) {
				t.Errorf("action %s should NOT be read-only", tt.name)
			}
		})
	}
}

// TestBrowser_ReadOnlyActionsGetPermAllow verifies read-only actions get PermAllow.
func TestBrowser_ReadOnlyActionsGetPermAllow(t *testing.T) {
	tool := &browser.Tool{}

	readOnlyActions := []struct {
		name  string
		input string
	}{
		{"snapshot", `{"action":"snapshot"}`},
		{"get", `{"action":"get","get":"title"}`},
		{"wait", `{"action":"wait","target":"1000"}`},
		{"scroll", `{"action":"scroll","direction":"up"}`},
		{"back", `{"action":"back"}`},
		{"forward", `{"action":"forward"}`},
		{"reload", `{"action":"reload"}`},
		{"close", `{"action":"close"}`},
		{"console", `{"action":"console"}`},
		{"cookies_get", `{"action":"cookies"}`},
		{"tab_list", `{"action":"tab","tab_action":"list"}`},
		{"tab_switch", `{"action":"tab","tab_action":"switch","tab_index":1}`},
	}

	for _, tt := range readOnlyActions {
		t.Run(tt.name, func(t *testing.T) {
			perm, err := tool.CheckPermissions(json.RawMessage(tt.input), nil)
			if err != nil {
				t.Fatalf("CheckPermissions: %v", err)
			}
			if perm.Behavior != models.PermAllow {
				t.Errorf("action %s: Behavior = %q, want allow", tt.name, perm.Behavior)
			}
		})
	}
}

// TestBrowser_NewActionsPresent verifies that the new enhanced actions exist in the schema.
func TestBrowser_NewActionsPresent(t *testing.T) {
	tool := &browser.Tool{}
	schema := tool.InputSchema()

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("schema parse: %v", err)
	}

	props := parsed["properties"].(map[string]any)
	actionProp := props["action"].(map[string]any)
	enumRaw := actionProp["enum"].([]any)

	schemaActions := make(map[string]bool)
	for _, a := range enumRaw {
		schemaActions[a.(string)] = true
	}

	newActions := []string{"eval", "upload", "viewport", "pdf", "cookies", "console"}
	for _, action := range newActions {
		if !schemaActions[action] {
			t.Errorf("new action %q missing from schema", action)
		}
	}

	// Verify new input fields exist in schema properties.
	newFields := []string{"js", "file_path", "width", "height", "cookies"}
	for _, field := range newFields {
		if _, ok := props[field]; !ok {
			t.Errorf("new field %q missing from schema properties", field)
		}
	}
}

// TestBrowser_ValidateInput verifies input validation for various actions.
func TestBrowser_ValidateInput(t *testing.T) {
	tool := &browser.Tool{}

	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"open_valid", `{"action":"open","url":"https://example.com"}`, true},
		{"open_no_url", `{"action":"open"}`, false},
		{"click_valid", `{"action":"click","selector":"#btn"}`, true},
		{"click_no_selector", `{"action":"click"}`, false},
		{"eval_valid", `{"action":"eval","js":"1+1"}`, true},
		{"eval_no_js", `{"action":"eval"}`, false},
		{"viewport_valid", `{"action":"viewport","width":1024,"height":768}`, true},
		{"viewport_no_height", `{"action":"viewport","width":1024}`, false},
		{"pdf_valid", `{"action":"pdf","path":"/tmp/out.pdf"}`, true},
		{"pdf_no_path", `{"action":"pdf"}`, false},
		{"upload_valid", `{"action":"upload","selector":"#f","file_path":"/tmp/a.txt"}`, true},
		{"upload_no_file", `{"action":"upload","selector":"#f"}`, false},
		{"snapshot_valid", `{"action":"snapshot"}`, true},
		{"console_valid", `{"action":"console"}`, true},
		{"unknown_action", `{"action":"invalid_action"}`, false},
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
