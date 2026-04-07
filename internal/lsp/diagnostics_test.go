package lsp

import (
	"strings"
	"testing"
)

func TestFormatDiagnostics_Empty(t *testing.T) {
	result := FormatDiagnostics("/test/file.go", nil)
	if result != "" {
		t.Errorf("expected empty string for nil diagnostics, got: %q", result)
	}
}

func TestFormatDiagnostics_Basic(t *testing.T) {
	diags := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 14, Character: 0}},
			Severity: SeverityError,
			Message:  "undefined: foo",
			Source:   "compiler",
		},
		{
			Range:    Range{Start: Position{Line: 77, Character: 0}},
			Severity: SeverityWarning,
			Message:  "unused variable 'x'",
			Source:   "staticcheck",
		},
	}

	result := FormatDiagnostics("/path/to/file.go", diags)

	// Should contain the file name and counts.
	if !strings.Contains(result, "file.go") {
		t.Errorf("expected file name in output, got: %s", result)
	}
	if !strings.Contains(result, "1 error") {
		t.Errorf("expected '1 error' in output, got: %s", result)
	}
	if !strings.Contains(result, "1 warning") {
		t.Errorf("expected '1 warning' in output, got: %s", result)
	}
	// Check 1-based line numbers.
	if !strings.Contains(result, "line 15") {
		t.Errorf("expected 'line 15' (1-based) in output, got: %s", result)
	}
	if !strings.Contains(result, "line 78") {
		t.Errorf("expected 'line 78' (1-based) in output, got: %s", result)
	}
	// Errors should appear before warnings.
	errIdx := strings.Index(result, "error")
	warnIdx := strings.Index(result, "warning")
	if errIdx > warnIdx {
		t.Errorf("expected errors before warnings, got error at %d, warning at %d", errIdx, warnIdx)
	}
}

func TestFormatDiagnostics_VolumeLimit(t *testing.T) {
	diags := make([]Diagnostic, 15)
	for i := range diags {
		diags[i] = Diagnostic{
			Range:    Range{Start: Position{Line: i}},
			Severity: SeverityError,
			Message:  "some error",
		}
	}

	result := FormatDiagnostics("/test/file.go", diags)
	if !strings.Contains(result, "+ 5 more diagnostics not shown") {
		t.Errorf("expected truncation message, got: %s", result)
	}
}

func TestFormatDiagnosticsShort(t *testing.T) {
	diags := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 9, Character: 0}},
			Severity: SeverityError,
			Message:  "missing return",
		},
	}

	result := FormatDiagnosticsShort("/path/to/file.go", diags)
	if !strings.Contains(result, "file.go:10: error: missing return") {
		t.Errorf("expected 'file.go:10: error: missing return', got: %s", result)
	}
}

func TestDiagnosticRegistry_UpdateAndGet(t *testing.T) {
	reg := NewDiagnosticRegistry()

	diags := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 5}},
			Severity: SeverityError,
			Message:  "test error",
		},
	}

	reg.Update(PathToURI("/test/file.go"), diags)

	got := reg.Get("/test/file.go")
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}
	if got[0].Message != "test error" {
		t.Errorf("expected message 'test error', got %q", got[0].Message)
	}
}

func TestDiagnosticRegistry_GetNew_Dedup(t *testing.T) {
	reg := NewDiagnosticRegistry()

	diags := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 5}},
			Severity: SeverityError,
			Message:  "test error",
		},
	}

	reg.Update(PathToURI("/test/file.go"), diags)

	// First call: should return new diagnostics.
	got, isNew := reg.GetNew("/test/file.go")
	if !isNew {
		t.Error("expected isNew=true on first GetNew")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}

	// Second call with same diagnostics: should be suppressed.
	got2, isNew2 := reg.GetNew("/test/file.go")
	if isNew2 {
		t.Error("expected isNew=false on second GetNew with same diagnostics")
	}
	if got2 != nil {
		t.Errorf("expected nil diagnostics on dedup, got %d", len(got2))
	}

	// Update with different diagnostics: should return new.
	diags2 := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 10}},
			Severity: SeverityWarning,
			Message:  "different error",
		},
	}
	reg.Update(PathToURI("/test/file.go"), diags2)

	got3, isNew3 := reg.GetNew("/test/file.go")
	if !isNew3 {
		t.Error("expected isNew=true after diagnostics changed")
	}
	if len(got3) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got3))
	}
}

func TestDiagnosticRegistry_Clear(t *testing.T) {
	reg := NewDiagnosticRegistry()

	diags := []Diagnostic{
		{Range: Range{Start: Position{Line: 1}}, Severity: SeverityError, Message: "err"},
	}
	reg.Update(PathToURI("/test/file.go"), diags)

	// Clear by updating with empty.
	reg.Update(PathToURI("/test/file.go"), nil)

	got := reg.Get("/test/file.go")
	if len(got) != 0 {
		t.Errorf("expected 0 diagnostics after clear, got %d", len(got))
	}
}

func TestSortDiagnostics(t *testing.T) {
	diags := []Diagnostic{
		{Range: Range{Start: Position{Line: 10}}, Severity: SeverityWarning, Message: "warn"},
		{Range: Range{Start: Position{Line: 5}}, Severity: SeverityError, Message: "err1"},
		{Range: Range{Start: Position{Line: 20}}, Severity: SeverityError, Message: "err2"},
		{Range: Range{Start: Position{Line: 1}}, Severity: SeverityHint, Message: "hint"},
	}

	sortDiagnostics(diags)

	// Errors first (sorted by line), then warnings, then hints.
	expected := []struct {
		sev  DiagnosticSeverity
		line int
	}{
		{SeverityError, 5},
		{SeverityError, 20},
		{SeverityWarning, 10},
		{SeverityHint, 1},
	}

	for i, exp := range expected {
		if diags[i].Severity != exp.sev || diags[i].Range.Start.Line != exp.line {
			t.Errorf("diags[%d]: expected severity=%d line=%d, got severity=%d line=%d",
				i, exp.sev, exp.line, diags[i].Severity, diags[i].Range.Start.Line)
		}
	}
}

func TestSeverityString(t *testing.T) {
	tests := []struct {
		sev  DiagnosticSeverity
		want string
	}{
		{SeverityError, "error"},
		{SeverityWarning, "warning"},
		{SeverityInformation, "info"},
		{SeverityHint, "hint"},
		{DiagnosticSeverity(99), "unknown"},
	}

	for _, tt := range tests {
		got := severityString(tt.sev)
		if got != tt.want {
			t.Errorf("severityString(%d) = %q, want %q", tt.sev, got, tt.want)
		}
	}
}
