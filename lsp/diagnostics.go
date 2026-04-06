package lsp

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	// MaxDiagnosticsPerFile is the maximum number of diagnostics returned for a single file.
	MaxDiagnosticsPerFile = 10

	// MaxDiagnosticsTotal is the maximum number of diagnostics returned across all files.
	MaxDiagnosticsTotal = 30
)

// DiagnosticRegistry collects and deduplicates diagnostics from language servers.
type DiagnosticRegistry struct {
	mu        sync.Mutex
	current   map[string][]Diagnostic // URI → latest diagnostics
	delivered map[string]string       // URI → hash of last delivered set
}

// NewDiagnosticRegistry creates a new DiagnosticRegistry.
func NewDiagnosticRegistry() *DiagnosticRegistry {
	return &DiagnosticRegistry{
		current:   make(map[string][]Diagnostic),
		delivered: make(map[string]string),
	}
}

// Update replaces diagnostics for a URI (called on publishDiagnostics).
func (r *DiagnosticRegistry) Update(uri string, diags []Diagnostic) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(diags) == 0 {
		delete(r.current, uri)
	} else {
		r.current[uri] = diags
	}
}

// Get returns current diagnostics for a file path, sorted by severity then line.
func (r *DiagnosticRegistry) Get(filePath string) []Diagnostic {
	uri := PathToURI(filePath)
	r.mu.Lock()
	diags := r.current[uri]
	r.mu.Unlock()

	if len(diags) == 0 {
		return nil
	}

	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	sortDiagnostics(sorted)
	return sorted
}

// GetNew returns diagnostics for a file only if they differ from the last delivery.
// Returns the diagnostics and true if they are new, or nil and false if unchanged.
func (r *DiagnosticRegistry) GetNew(filePath string) ([]Diagnostic, bool) {
	uri := PathToURI(filePath)
	r.mu.Lock()
	defer r.mu.Unlock()

	diags := r.current[uri]
	if len(diags) == 0 {
		// If we previously delivered something, this is "new" (cleared).
		if _, had := r.delivered[uri]; had {
			delete(r.delivered, uri)
			return nil, true
		}
		return nil, false
	}

	hash := hashDiagnostics(diags)
	if r.delivered[uri] == hash {
		return nil, false
	}

	r.delivered[uri] = hash

	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	sortDiagnostics(sorted)
	return sorted, true
}

// FormatDiagnostics returns a human-readable string for the agent.
// Applies volume limits: max MaxDiagnosticsPerFile per file.
func FormatDiagnostics(filePath string, diags []Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}

	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	sortDiagnostics(sorted)

	var errors, warnings, infos, hints int
	for _, d := range sorted {
		switch d.Severity {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInformation:
			infos++
		case SeverityHint:
			hints++
		}
	}

	var sb strings.Builder
	base := filepath.Base(filePath)
	sb.WriteString(fmt.Sprintf("Diagnostics for %s (", base))
	var counts []string
	if errors > 0 {
		counts = append(counts, fmt.Sprintf("%d error", errors))
		if errors > 1 {
			counts[len(counts)-1] += "s"
		}
	}
	if warnings > 0 {
		counts = append(counts, fmt.Sprintf("%d warning", warnings))
		if warnings > 1 {
			counts[len(counts)-1] += "s"
		}
	}
	if infos > 0 {
		counts = append(counts, fmt.Sprintf("%d info", infos))
	}
	if hints > 0 {
		counts = append(counts, fmt.Sprintf("%d hint", hints))
		if hints > 1 {
			counts[len(counts)-1] += "s"
		}
	}
	sb.WriteString(strings.Join(counts, ", "))
	sb.WriteString("):\n")

	shown := min(len(sorted), MaxDiagnosticsPerFile)

	for i := 0; i < shown; i++ {
		d := sorted[i]
		line := d.Range.Start.Line + 1 // Convert 0-based to 1-based
		sev := severityString(d.Severity)
		source := ""
		if d.Source != "" {
			source = fmt.Sprintf(" (%s)", d.Source)
		}
		sb.WriteString(fmt.Sprintf("  line %d: %s: %s%s\n", line, sev, d.Message, source))
	}

	if len(sorted) > MaxDiagnosticsPerFile {
		sb.WriteString(fmt.Sprintf("  (+ %d more diagnostics not shown)\n", len(sorted)-MaxDiagnosticsPerFile))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// FormatDiagnosticsShort returns a compact "file:line: severity: message" format
// suitable for appending to file tool results.
func FormatDiagnosticsShort(filePath string, diags []Diagnostic) string {
	if len(diags) == 0 {
		return ""
	}

	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	sortDiagnostics(sorted)

	shown := min(len(sorted), MaxDiagnosticsPerFile)

	var sb strings.Builder
	sb.WriteString("LSP diagnostics:\n")
	for i := 0; i < shown; i++ {
		d := sorted[i]
		line := d.Range.Start.Line + 1
		sev := severityString(d.Severity)
		sb.WriteString(fmt.Sprintf("  %s:%d: %s: %s\n", filepath.Base(filePath), line, sev, d.Message))
	}

	if len(sorted) > MaxDiagnosticsPerFile {
		sb.WriteString(fmt.Sprintf("  (+ %d more diagnostics not shown)\n", len(sorted)-MaxDiagnosticsPerFile))
	}

	return strings.TrimRight(sb.String(), "\n")
}

func severityString(s DiagnosticSeverity) string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

func sortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Severity != diags[j].Severity {
			return diags[i].Severity < diags[j].Severity // lower = more severe
		}
		return diags[i].Range.Start.Line < diags[j].Range.Start.Line
	})
}

func hashDiagnostics(diags []Diagnostic) string {
	h := sha256.New()
	for _, d := range diags {
		fmt.Fprintf(h, "%d:%d:%d:%s:%s\n",
			d.Range.Start.Line, d.Range.Start.Character,
			d.Severity, d.Source, d.Message)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
