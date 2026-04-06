package lsp

import (
	"encoding/json"
	"testing"
)

func TestPathToURI(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/file.go", "file:///home/user/file.go"},
		{"/tmp/test.py", "file:///tmp/test.py"},
	}
	for _, tt := range tests {
		got := PathToURI(tt.path)
		if got != tt.want {
			t.Errorf("PathToURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///home/user/file.go", "/home/user/file.go"},
		{"file:///tmp/test.py", "/tmp/test.py"},
		{"/just/a/path", "/just/a/path"},
	}
	for _, tt := range tests {
		got := URIToPath(tt.uri)
		if got != tt.want {
			t.Errorf("URIToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestSymbolKindString(t *testing.T) {
	tests := []struct {
		kind SymbolKind
		want string
	}{
		{SymbolKindFunction, "func"},
		{SymbolKindClass, "class"},
		{SymbolKindMethod, "method"},
		{SymbolKindVariable, "variable"},
		{SymbolKindStruct, "struct"},
		{SymbolKind(999), "unknown"},
	}
	for _, tt := range tests {
		got := tt.kind.String()
		if got != tt.want {
			t.Errorf("SymbolKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestDiagnosticSeverityValues(t *testing.T) {
	if SeverityError != 1 {
		t.Errorf("SeverityError = %d, want 1", SeverityError)
	}
	if SeverityWarning != 2 {
		t.Errorf("SeverityWarning = %d, want 2", SeverityWarning)
	}
	if SeverityInformation != 3 {
		t.Errorf("SeverityInformation = %d, want 3", SeverityInformation)
	}
	if SeverityHint != 4 {
		t.Errorf("SeverityHint = %d, want 4", SeverityHint)
	}
}

func TestPositionJSON(t *testing.T) {
	pos := Position{Line: 10, Character: 5}
	data, err := json.Marshal(pos)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Position
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != pos {
		t.Errorf("roundtrip got %+v, want %+v", decoded, pos)
	}
}

func TestRangeJSON(t *testing.T) {
	r := Range{
		Start: Position{Line: 1, Character: 0},
		End:   Position{Line: 1, Character: 10},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Range
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != r {
		t.Errorf("roundtrip got %+v, want %+v", decoded, r)
	}
}

func TestDiagnosticJSON(t *testing.T) {
	d := Diagnostic{
		Range:    Range{Start: Position{Line: 5, Character: 0}, End: Position{Line: 5, Character: 10}},
		Severity: SeverityError,
		Source:   "compiler",
		Message:  "undefined: foo",
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Diagnostic
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Message != d.Message || decoded.Severity != d.Severity || decoded.Source != d.Source {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, d)
	}
}

func TestInitializeParamsJSON(t *testing.T) {
	params := InitializeParams{
		ProcessID: 1234,
		RootURI:   "file:///tmp/project",
		WorkspaceFolders: []WorkspaceFolder{
			{URI: "file:///tmp/project", Name: "project"},
		},
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Synchronization: SyncCapabilities{DidSave: true},
			},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var decoded InitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ProcessID != params.ProcessID {
		t.Errorf("ProcessID = %d, want %d", decoded.ProcessID, params.ProcessID)
	}
	if decoded.RootURI != params.RootURI {
		t.Errorf("RootURI = %q, want %q", decoded.RootURI, params.RootURI)
	}
	if len(decoded.WorkspaceFolders) != 1 {
		t.Errorf("WorkspaceFolders len = %d, want 1", len(decoded.WorkspaceFolders))
	}
}

func TestDidOpenParamsJSON(t *testing.T) {
	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///tmp/test.go",
			LanguageID: "go",
			Version:    1,
			Text:       "package main\n",
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var decoded DidOpenTextDocumentParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TextDocument.URI != params.TextDocument.URI {
		t.Errorf("URI = %q, want %q", decoded.TextDocument.URI, params.TextDocument.URI)
	}
	if decoded.TextDocument.LanguageID != "go" {
		t.Errorf("LanguageID = %q, want %q", decoded.TextDocument.LanguageID, "go")
	}
}

func TestDidChangeParamsJSON(t *testing.T) {
	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			URI:     "file:///tmp/test.go",
			Version: 2,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: "package main\n\nfunc main() {}\n"},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var decoded DidChangeTextDocumentParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TextDocument.Version != 2 {
		t.Errorf("Version = %d, want 2", decoded.TextDocument.Version)
	}
	if len(decoded.ContentChanges) != 1 {
		t.Fatalf("ContentChanges len = %d, want 1", len(decoded.ContentChanges))
	}
}

func TestPublishDiagnosticsParamsJSON(t *testing.T) {
	params := PublishDiagnosticsParams{
		URI: "file:///tmp/test.go",
		Diagnostics: []Diagnostic{
			{
				Range:    Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 1, Character: 5}},
				Severity: SeverityError,
				Message:  "test error",
			},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}

	var decoded PublishDiagnosticsParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.URI != params.URI {
		t.Errorf("URI = %q, want %q", decoded.URI, params.URI)
	}
	if len(decoded.Diagnostics) != 1 {
		t.Fatalf("Diagnostics len = %d, want 1", len(decoded.Diagnostics))
	}
	if decoded.Diagnostics[0].Message != "test error" {
		t.Errorf("Message = %q, want %q", decoded.Diagnostics[0].Message, "test error")
	}
}
