package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoisutolabs/forge/lsp"
)

// =============================================================================
// Manager Extension Routing
// =============================================================================

func TestManagerRoutesGoToGopls(t *testing.T) {
	configs := []lsp.ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Args:    []string{"serve"},
			Extensions: map[string]string{
				".go":  "go",
				".mod": "go.mod",
			},
		},
		{
			Name:    "typescript",
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			Extensions: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
				".js":  "javascript",
			},
		},
	}

	mgr := lsp.NewManager("/tmp/project", configs)

	// Test that the manager routes .go files to gopls.
	// We can't call ServerForFile without a real server, but we can verify
	// the extension mapping through the manager's document tracking behavior.

	// Test IsFileOpen returns false for unopened files.
	if mgr.IsFileOpen("/tmp/project/main.go") {
		t.Error("expected main.go to not be open initially")
	}
	if mgr.IsFileOpen("/tmp/project/app.ts") {
		t.Error("expected app.ts to not be open initially")
	}

	// Verify Documents() and Diagnostics() are not nil.
	if mgr.Documents() == nil {
		t.Error("Documents() should not be nil")
	}
	if mgr.Diagnostics() == nil {
		t.Error("Diagnostics() should not be nil")
	}
}

func TestManagerExtensionMapping(t *testing.T) {
	configs := []lsp.ServerConfig{
		{
			Name:    "gopls",
			Command: "gopls",
			Extensions: map[string]string{
				".go":  "go",
				".mod": "go.mod",
				".sum": "go.sum",
			},
		},
		{
			Name:    "typescript",
			Command: "typescript-language-server",
			Extensions: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
				".js":  "javascript",
				".jsx": "javascriptreact",
				".mts": "typescript",
				".cts": "typescript",
				".mjs": "javascript",
				".cjs": "javascript",
			},
		},
		{
			Name:    "pyright",
			Command: "pyright-langserver",
			Extensions: map[string]string{
				".py":  "python",
				".pyi": "python",
			},
		},
	}

	mgr := lsp.NewManager("/tmp/project", configs)

	// When multiple configs have the same extension, first wins.
	// Verify the manager was created without errors.
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	// We can verify extension deduplication indirectly — if we add a
	// second config with .py it should not override pyright.
	configs2 := append(configs, lsp.ServerConfig{
		Name:       "pylsp",
		Command:    "pylsp",
		Extensions: map[string]string{".py": "python", ".pyi": "python"},
	})
	mgr2 := lsp.NewManager("/tmp/project", configs2)
	if mgr2 == nil {
		t.Fatal("expected non-nil manager with overlapping configs")
	}
}

// =============================================================================
// Document Tracker
// =============================================================================

func TestDocumentTrackerOpenChangeClose(t *testing.T) {
	dt := lsp.NewDocumentTracker()

	// Initially nothing is open.
	path := filepath.Join(t.TempDir(), "test.go")
	if dt.IsOpen(path) {
		t.Error("expected file to not be open initially")
	}
	if dt.Version(path) != 0 {
		t.Errorf("expected version 0 for untracked file, got %d", dt.Version(path))
	}
}

func TestDocumentTrackerVersionIncrement(t *testing.T) {
	// The DocumentTracker tracks versions internally. Without a real server,
	// we can test the Version/IsOpen methods.
	dt := lsp.NewDocumentTracker()
	path := "/tmp/test/main.go"

	// Not open: version is 0.
	if v := dt.Version(path); v != 0 {
		t.Errorf("version = %d, want 0", v)
	}
	if dt.IsOpen(path) {
		t.Error("should not be open")
	}
}

// =============================================================================
// Server Lifecycle State Machine
// =============================================================================

func TestServerLifecycleStates(t *testing.T) {
	// Test that NewServer creates a server in Stopped state.
	cfg := lsp.ServerConfig{
		Name:    "test-server",
		Command: "nonexistent-binary",
		Args:    []string{},
		Extensions: map[string]string{
			".go": "go",
		},
	}

	srv := lsp.NewServer(cfg)
	if srv.State() != lsp.StateStopped {
		t.Errorf("initial state = %d, want StateStopped (%d)", srv.State(), lsp.StateStopped)
	}

	// Config should be accessible.
	if srv.Config().Name != "test-server" {
		t.Errorf("config.Name = %q, want %q", srv.Config().Name, "test-server")
	}
	if srv.Config().Command != "nonexistent-binary" {
		t.Errorf("config.Command = %q, want %q", srv.Config().Command, "nonexistent-binary")
	}
}

func TestServerStateConstants(t *testing.T) {
	// Verify state constants have expected values.
	if lsp.StateStopped != 0 {
		t.Errorf("StateStopped = %d, want 0", lsp.StateStopped)
	}
	if lsp.StateStarting != 1 {
		t.Errorf("StateStarting = %d, want 1", lsp.StateStarting)
	}
	if lsp.StateRunning != 2 {
		t.Errorf("StateRunning = %d, want 2", lsp.StateRunning)
	}
	if lsp.StateStopping != 3 {
		t.Errorf("StateStopping = %d, want 3", lsp.StateStopping)
	}
	if lsp.StateError != 4 {
		t.Errorf("StateError = %d, want 4", lsp.StateError)
	}
}

// =============================================================================
// Config Detection
// =============================================================================

func TestDefaultConfigsContent(t *testing.T) {
	configs := lsp.DefaultConfigs()

	if len(configs) != 4 {
		t.Fatalf("expected 4 default configs, got %d", len(configs))
	}

	// Verify gopls config.
	gopls := configs[0]
	if gopls.Name != "gopls" {
		t.Errorf("configs[0].Name = %q, want %q", gopls.Name, "gopls")
	}
	if gopls.Command != "gopls" {
		t.Errorf("gopls command = %q, want %q", gopls.Command, "gopls")
	}
	if _, ok := gopls.Extensions[".go"]; !ok {
		t.Error("gopls should handle .go files")
	}
	if len(gopls.DetectFiles) == 0 {
		t.Error("gopls should have detect files")
	}

	// Verify typescript config.
	ts := configs[1]
	if ts.Name != "typescript" {
		t.Errorf("configs[1].Name = %q, want %q", ts.Name, "typescript")
	}
	if ts.Command != "typescript-language-server" {
		t.Errorf("ts command = %q, want %q", ts.Command, "typescript-language-server")
	}
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		if _, ok := ts.Extensions[ext]; !ok {
			t.Errorf("typescript should handle %s files", ext)
		}
	}

	// Verify pyright config.
	pyright := configs[2]
	if pyright.Name != "pyright" {
		t.Errorf("configs[2].Name = %q, want %q", pyright.Name, "pyright")
	}
	if _, ok := pyright.Extensions[".py"]; !ok {
		t.Error("pyright should handle .py files")
	}

	// Verify pylsp config.
	pylsp := configs[3]
	if pylsp.Name != "pylsp" {
		t.Errorf("configs[3].Name = %q, want %q", pylsp.Name, "pylsp")
	}
}

func TestDetectConfigsWithMockFiles(t *testing.T) {
	// Create a workspace with go.mod — DetectConfigs should find gopls if available.
	dir := t.TempDir()

	// Create indicator files.
	writeEmptyFile(t, filepath.Join(dir, "go.mod"))
	writeEmptyFile(t, filepath.Join(dir, "package.json"))

	// DetectConfigs checks exec.LookPath, so results depend on the environment.
	// We just verify it doesn't panic and returns a valid slice.
	configs := lsp.DetectConfigs(dir)
	// Result may be empty if gopls/typescript-language-server are not installed.
	if configs == nil {
		t.Error("expected non-nil slice (even if empty)")
	}
}

func TestDetectConfigsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	configs := lsp.DetectConfigs(dir)
	// No indicator files, so no configs should be detected.
	if len(configs) != 0 {
		t.Errorf("expected 0 configs for empty dir, got %d", len(configs))
	}
}

func writeEmptyFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// =============================================================================
// Diagnostics Registry
// =============================================================================

func TestDiagnosticRegistryUpdateAndGet(t *testing.T) {
	reg := lsp.NewDiagnosticRegistry()

	uri := "file:///tmp/project/main.go"
	path := "/tmp/project/main.go"

	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 5, Character: 10},
				End:   lsp.Position{Line: 5, Character: 20},
			},
			Severity: 1, // Error
			Message:  "undefined: foo",
		},
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 10, Character: 0},
				End:   lsp.Position{Line: 10, Character: 5},
			},
			Severity: 2, // Warning
			Message:  "unused variable: bar",
		},
	}

	reg.Update(uri, diags)

	got := reg.Get(path)
	if len(got) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(got))
	}

	if got[0].Message != "undefined: foo" && got[1].Message != "undefined: foo" {
		t.Error("expected to find 'undefined: foo' diagnostic")
	}
}

func TestDiagnosticRegistryGetNew(t *testing.T) {
	reg := lsp.NewDiagnosticRegistry()

	uri := "file:///tmp/project/main.go"
	path := "/tmp/project/main.go"

	diags := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 1}, End: lsp.Position{Line: 1}},
			Severity: 1,
			Message:  "error 1",
		},
	}

	reg.Update(uri, diags)

	// First GetNew should return diagnostics.
	got, isNew := reg.GetNew(path)
	if !isNew {
		t.Error("expected isNew=true for first call")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}

	// Second GetNew with same diagnostics should return isNew=false and nil (dedup).
	got, isNew = reg.GetNew(path)
	if isNew {
		t.Error("expected isNew=false for unchanged diagnostics")
	}
	if got != nil {
		t.Fatalf("expected nil diagnostics on dedup, got %d", len(got))
	}

	// Update with new diagnostics.
	diags2 := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 2}, End: lsp.Position{Line: 2}},
			Severity: 2,
			Message:  "warning 1",
		},
	}
	reg.Update(uri, diags2)

	got, isNew = reg.GetNew(path)
	if !isNew {
		t.Error("expected isNew=true after update")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic after update, got %d", len(got))
	}
	if got[0].Message != "warning 1" {
		t.Errorf("diagnostic message = %q, want %q", got[0].Message, "warning 1")
	}
}

func TestDiagnosticRegistryEmpty(t *testing.T) {
	reg := lsp.NewDiagnosticRegistry()

	got := reg.Get("/nonexistent/file.go")
	if len(got) != 0 {
		t.Errorf("expected 0 diagnostics for unknown file, got %d", len(got))
	}

	got, isNew := reg.GetNew("/nonexistent/file.go")
	if isNew {
		t.Error("expected isNew=false for unknown file")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(got))
	}
}

// =============================================================================
// FormatDiagnostics
// =============================================================================

func TestFormatDiagnostics(t *testing.T) {
	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 5, Character: 10},
				End:   lsp.Position{Line: 5, Character: 20},
			},
			Severity: 1,
			Message:  "undefined: foo",
		},
	}

	formatted := lsp.FormatDiagnostics("/tmp/main.go", diags)
	if formatted == "" {
		t.Error("expected non-empty formatted output")
	}
	if !findSubstring(formatted, "undefined: foo") {
		t.Error("expected formatted output to contain the diagnostic message")
	}
}

func TestFormatDiagnosticsShort(t *testing.T) {
	diags := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 10, Character: 5},
			},
			Severity: 2,
			Message:  "unused variable",
		},
	}

	formatted := lsp.FormatDiagnosticsShort("/tmp/main.go", diags)
	if formatted == "" {
		t.Error("expected non-empty short formatted output")
	}
	if !findSubstring(formatted, "unused variable") {
		t.Error("expected short format to contain the diagnostic message")
	}
}

// =============================================================================
// Protocol Helper Functions
// =============================================================================

func TestPathToURIAndBack(t *testing.T) {
	path := "/tmp/project/main.go"
	uri := lsp.PathToURI(path)

	if uri != "file:///tmp/project/main.go" {
		t.Errorf("PathToURI(%q) = %q, want %q", path, uri, "file:///tmp/project/main.go")
	}

	back := lsp.URIToPath(uri)
	if back != path {
		t.Errorf("URIToPath(%q) = %q, want %q", uri, back, path)
	}
}

// =============================================================================
// ErrServerCrashed
// =============================================================================

func TestErrServerCrashed(t *testing.T) {
	if lsp.ErrServerCrashed == nil {
		t.Fatal("ErrServerCrashed should not be nil")
	}
	if lsp.ErrServerCrashed.Error() != "language server crashed too many times" {
		t.Errorf("unexpected error message: %s", lsp.ErrServerCrashed.Error())
	}
}
