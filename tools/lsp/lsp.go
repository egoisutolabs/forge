// Package lsp implements the LSPTool — an agent-facing tool for querying
// language servers for code intelligence: diagnostics, definitions, references,
// hover info, completions, and symbols.
package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoisutolabs/forge/lsp"
	"github.com/egoisutolabs/forge/models"
	"github.com/egoisutolabs/forge/tools"
)

// lspInput is the JSON schema for LSPTool input.
type lspInput struct {
	Action    string `json:"action"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line,omitempty"`
	Character int    `json:"character,omitempty"`
	Query     string `json:"query,omitempty"`
	NewName   string `json:"new_name,omitempty"`
}

// Tool implements the LSP tool — query language servers for code intelligence.
type Tool struct{}

func (t *Tool) Name() string { return "LSP" }

func (t *Tool) Description() string {
	return `Query language servers for code intelligence: diagnostics, definitions, references, hover info, completions, and symbols. Requires a running language server (gopls, typescript-language-server, pyright) — servers start automatically when you first access a file of that language.`
}

func (t *Tool) SearchHint() string {
	return "lsp language server diagnostics definition references hover completion symbols goto"
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["action", "file_path"],
		"properties": {
			"action": {
				"type": "string",
				"enum": ["diagnostics", "definition", "references", "hover", "completion", "symbols", "workspace_symbols", "rename_preview"],
				"description": "The LSP operation to perform."
			},
			"file_path": {
				"type": "string",
				"description": "Absolute path to the file. Required for all actions."
			},
			"line": {
				"type": "integer",
				"description": "1-based line number. Required for: definition, references, hover, completion, rename_preview."
			},
			"character": {
				"type": "integer",
				"description": "1-based column number. Required for: definition, references, hover, completion, rename_preview."
			},
			"query": {
				"type": "string",
				"description": "Search query. Required for: workspace_symbols."
			},
			"new_name": {
				"type": "string",
				"description": "The proposed new name. Required for: rename_preview."
			}
		}
	}`)
}

func (t *Tool) ValidateInput(input json.RawMessage) error {
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if in.Action == "" {
		return fmt.Errorf("action is required")
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return fmt.Errorf("file_path is required")
	}

	switch in.Action {
	case "diagnostics", "symbols":
		// No position required
	case "definition", "references", "hover", "completion":
		if in.Line < 1 {
			return fmt.Errorf("line is required and must be >= 1 for %s", in.Action)
		}
		if in.Character < 1 {
			return fmt.Errorf("character is required and must be >= 1 for %s", in.Action)
		}
	case "rename_preview":
		if in.Line < 1 {
			return fmt.Errorf("line is required and must be >= 1 for rename_preview")
		}
		if in.Character < 1 {
			return fmt.Errorf("character is required and must be >= 1 for rename_preview")
		}
		if in.NewName == "" {
			return fmt.Errorf("new_name is required for rename_preview")
		}
	case "workspace_symbols":
		if in.Query == "" {
			return fmt.Errorf("query is required for workspace_symbols")
		}
	default:
		return fmt.Errorf("unknown action: %s", in.Action)
	}
	return nil
}

func (t *Tool) CheckPermissions(_ json.RawMessage, _ *tools.ToolContext) (*models.PermissionDecision, error) {
	return &models.PermissionDecision{Behavior: models.PermAllow}, nil
}

func (t *Tool) IsConcurrencySafe(_ json.RawMessage) bool { return true }
func (t *Tool) IsReadOnly(_ json.RawMessage) bool        { return true }

func (t *Tool) Execute(ctx context.Context, input json.RawMessage, tctx *tools.ToolContext) (*models.ToolResult, error) {
	var in lspInput
	if err := json.Unmarshal(input, &in); err != nil {
		return errResult("Invalid input: %s", err), nil
	}

	mgr := tctx.LSPManager
	if mgr == nil {
		return errResult("LSP not available: no language servers detected for this workspace"), nil
	}

	// Ensure server is running for this file type.
	server, err := mgr.ServerForFile(ctx, in.FilePath)
	if err != nil {
		return errResult("No language server available for %s: %s", filepath.Ext(in.FilePath), err), nil
	}
	if server == nil {
		return errResult("No language server handles %s files", filepath.Ext(in.FilePath)), nil
	}

	// Ensure the file is open in the server.
	if !mgr.IsFileOpen(in.FilePath) {
		content, err := os.ReadFile(in.FilePath)
		if err != nil {
			return errResult("Cannot read %s: %s", in.FilePath, err), nil
		}
		_ = mgr.OpenFile(ctx, in.FilePath, string(content))
	}

	// Convert 1-based to 0-based position.
	pos := lsp.Position{Line: in.Line - 1, Character: in.Character - 1}

	switch in.Action {
	case "diagnostics":
		return t.diagnostics(ctx, mgr, in)
	case "definition":
		return t.definition(ctx, mgr, in, pos)
	case "references":
		return t.references(ctx, mgr, in, pos)
	case "hover":
		return t.hover(ctx, mgr, in, pos)
	case "completion":
		return t.completion(ctx, mgr, in, pos)
	case "symbols":
		return t.symbols(ctx, mgr, in)
	case "workspace_symbols":
		return t.workspaceSymbols(ctx, mgr, in)
	case "rename_preview":
		return t.renamePreview(ctx, mgr, in, pos)
	default:
		return errResult("Unknown action: %s", in.Action), nil
	}
}

func (t *Tool) diagnostics(_ context.Context, mgr *lsp.Manager, in lspInput) (*models.ToolResult, error) {
	diags := mgr.Diagnostics().Get(in.FilePath)
	if len(diags) == 0 {
		return &models.ToolResult{Content: fmt.Sprintf("No diagnostics for %s", in.FilePath)}, nil
	}
	return &models.ToolResult{Content: lsp.FormatDiagnostics(in.FilePath, diags)}, nil
}

func (t *Tool) definition(ctx context.Context, mgr *lsp.Manager, in lspInput, pos lsp.Position) (*models.ToolResult, error) {
	params := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
		Position:     pos,
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/definition", params)
	if err != nil {
		return errResult("Definition request failed: %s", err), nil
	}

	locations, err := parseLocations(result)
	if err != nil {
		return errResult("Failed to parse definition response: %s", err), nil
	}
	if len(locations) == 0 {
		return &models.ToolResult{Content: "No definition found"}, nil
	}

	var sb strings.Builder
	sb.WriteString("Definition:\n")
	for _, loc := range locations {
		path := lsp.URIToPath(loc.URI)
		sb.WriteString(fmt.Sprintf("  %s:%d:%d\n", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1))
	}
	return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

func (t *Tool) references(ctx context.Context, mgr *lsp.Manager, in lspInput, pos lsp.Position) (*models.ToolResult, error) {
	params := lsp.ReferenceParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
		Position:     pos,
		Context:      lsp.ReferenceContext{IncludeDeclaration: true},
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/references", params)
	if err != nil {
		return errResult("References request failed: %s", err), nil
	}

	var locations []lsp.Location
	if err := json.Unmarshal(result, &locations); err != nil {
		return errResult("Failed to parse references response: %s", err), nil
	}
	if len(locations) == 0 {
		return &models.ToolResult{Content: "No references found"}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("References (%d found):\n", len(locations)))
	for _, loc := range locations {
		path := lsp.URIToPath(loc.URI)
		sb.WriteString(fmt.Sprintf("  %s:%d:%d\n", path, loc.Range.Start.Line+1, loc.Range.Start.Character+1))
	}
	return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

func (t *Tool) hover(ctx context.Context, mgr *lsp.Manager, in lspInput, pos lsp.Position) (*models.ToolResult, error) {
	params := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
		Position:     pos,
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/hover", params)
	if err != nil {
		return errResult("Hover request failed: %s", err), nil
	}

	if result == nil || string(result) == "null" {
		return &models.ToolResult{Content: "No hover information available"}, nil
	}

	var hover lsp.Hover
	if err := json.Unmarshal(result, &hover); err != nil {
		return errResult("Failed to parse hover response: %s", err), nil
	}

	content := extractHoverContent(hover)
	if content == "" {
		return &models.ToolResult{Content: "No hover information available"}, nil
	}
	return &models.ToolResult{Content: content}, nil
}

func (t *Tool) completion(ctx context.Context, mgr *lsp.Manager, in lspInput, pos lsp.Position) (*models.ToolResult, error) {
	params := lsp.CompletionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
		Position:     pos,
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/completion", params)
	if err != nil {
		return errResult("Completion request failed: %s", err), nil
	}

	items, err := parseCompletionItems(result)
	if err != nil {
		return errResult("Failed to parse completion response: %s", err), nil
	}
	if len(items) == 0 {
		return &models.ToolResult{Content: "No completions available"}, nil
	}

	const maxItems = 20
	shown := min(len(items), maxItems)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Completions at %s:%d:%d:\n", filepath.Base(in.FilePath), in.Line, in.Character))
	for i := 0; i < shown; i++ {
		item := items[i]
		detail := ""
		if item.Detail != "" {
			detail = "  " + item.Detail
		}
		sb.WriteString(fmt.Sprintf("  %-20s%s\n", item.Label, detail))
	}
	if len(items) > maxItems {
		sb.WriteString(fmt.Sprintf("  (+ %d more not shown)\n", len(items)-maxItems))
	}
	return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

func (t *Tool) symbols(ctx context.Context, mgr *lsp.Manager, in lspInput) (*models.ToolResult, error) {
	params := lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/documentSymbol", params)
	if err != nil {
		return errResult("Symbols request failed: %s", err), nil
	}

	// Try hierarchical DocumentSymbol[] first, then flat SymbolInformation[].
	var docSymbols []lsp.DocumentSymbol
	if err := json.Unmarshal(result, &docSymbols); err == nil && len(docSymbols) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Symbols in %s:\n", filepath.Base(in.FilePath)))
		formatDocSymbols(&sb, docSymbols, 0)
		return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
	}

	var symInfos []lsp.SymbolInformation
	if err := json.Unmarshal(result, &symInfos); err == nil && len(symInfos) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Symbols in %s:\n", filepath.Base(in.FilePath)))
		for _, s := range symInfos {
			sb.WriteString(fmt.Sprintf("  [%s] %s  line %d\n", s.Kind.String(), s.Name, s.Location.Range.Start.Line+1))
		}
		return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
	}

	return &models.ToolResult{Content: "No symbols found"}, nil
}

func (t *Tool) workspaceSymbols(ctx context.Context, mgr *lsp.Manager, in lspInput) (*models.ToolResult, error) {
	params := lsp.WorkspaceSymbolParams{
		Query: in.Query,
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "workspace/symbol", params)
	if err != nil {
		return errResult("Workspace symbols request failed: %s", err), nil
	}

	var symbols []lsp.SymbolInformation
	if err := json.Unmarshal(result, &symbols); err != nil {
		return errResult("Failed to parse workspace symbols response: %s", err), nil
	}
	if len(symbols) == 0 {
		return &models.ToolResult{Content: fmt.Sprintf("No workspace symbols matching '%s'", in.Query)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workspace symbols matching '%s' (%d found):\n", in.Query, len(symbols)))
	shown := min(len(symbols), lsp.MaxDiagnosticsTotal)
	for i := 0; i < shown; i++ {
		s := symbols[i]
		path := lsp.URIToPath(s.Location.URI)
		sb.WriteString(fmt.Sprintf("  [%s] %-20s %s:%d\n", s.Kind.String(), s.Name, path, s.Location.Range.Start.Line+1))
	}
	if len(symbols) > lsp.MaxDiagnosticsTotal {
		sb.WriteString(fmt.Sprintf("  (+ %d more not shown)\n", len(symbols)-lsp.MaxDiagnosticsTotal))
	}
	return &models.ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

func (t *Tool) renamePreview(ctx context.Context, mgr *lsp.Manager, in lspInput, pos lsp.Position) (*models.ToolResult, error) {
	params := lsp.RenameParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.PathToURI(in.FilePath)},
		Position:     pos,
		NewName:      in.NewName,
	}
	result, err := mgr.SendRequest(ctx, in.FilePath, "textDocument/rename", params)
	if err != nil {
		return errResult("Rename request failed: %s", err), nil
	}

	var edit lsp.WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return errResult("Failed to parse rename response: %s", err), nil
	}

	if len(edit.Changes) == 0 {
		return &models.ToolResult{Content: "No changes from rename"}, nil
	}

	totalEdits := 0
	for _, edits := range edit.Changes {
		totalEdits += len(edits)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rename preview → '%s' would change %d locations in %d files:\n", in.NewName, totalEdits, len(edit.Changes)))
	for uri, edits := range edit.Changes {
		path := lsp.URIToPath(uri)
		for _, e := range edits {
			sb.WriteString(fmt.Sprintf("  %s:%d:%d\n", path, e.Range.Start.Line+1, e.Range.Start.Character+1))
		}
	}
	sb.WriteString("NOTE: Changes NOT applied. Use Edit tool to apply the rename.")
	return &models.ToolResult{Content: sb.String()}, nil
}

// parseLocations parses the result of textDocument/definition which may be
// a single Location, a Location[], or a LocationLink[].
func parseLocations(data json.RawMessage) ([]lsp.Location, error) {
	// Try single location first.
	var loc lsp.Location
	if err := json.Unmarshal(data, &loc); err == nil && loc.URI != "" {
		return []lsp.Location{loc}, nil
	}

	// Try array of locations.
	var locations []lsp.Location
	if err := json.Unmarshal(data, &locations); err == nil && len(locations) > 0 && locations[0].URI != "" {
		return locations, nil
	}

	// Try LocationLink[] (extract targetUri + targetRange).
	var links []struct {
		TargetURI   string    `json:"targetUri"`
		TargetRange lsp.Range `json:"targetRange"`
	}
	if err := json.Unmarshal(data, &links); err == nil && len(links) > 0 && links[0].TargetURI != "" {
		locs := make([]lsp.Location, len(links))
		for i, l := range links {
			locs[i] = lsp.Location{URI: l.TargetURI, Range: l.TargetRange}
		}
		return locs, nil
	}

	return nil, fmt.Errorf("unrecognized definition response format")
}

// parseCompletionItems handles both CompletionItem[] and CompletionList responses.
func parseCompletionItems(data json.RawMessage) ([]lsp.CompletionItem, error) {
	var items []lsp.CompletionItem
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}

	var list lsp.CompletionList
	if err := json.Unmarshal(data, &list); err == nil {
		return list.Items, nil
	}

	return nil, fmt.Errorf("unrecognized completion response format")
}

// extractHoverContent pulls text from the Hover.Contents field which can be
// a string, a MarkupContent, or a MarkedString.
func extractHoverContent(hover lsp.Hover) string {
	// Try MarkupContent (has "kind" field).
	var mc lsp.MarkupContent
	if err := json.Unmarshal(hover.Contents, &mc); err == nil && mc.Kind != "" && mc.Value != "" {
		return mc.Value
	}

	// Try plain string.
	var s string
	if err := json.Unmarshal(hover.Contents, &s); err == nil && s != "" {
		return s
	}

	// Try MarkedString {language, value}.
	var ms struct {
		Language string `json:"language"`
		Value    string `json:"value"`
	}
	if err := json.Unmarshal(hover.Contents, &ms); err == nil && ms.Value != "" {
		if ms.Language != "" {
			return fmt.Sprintf("```%s\n%s\n```", ms.Language, ms.Value)
		}
		return ms.Value
	}

	return ""
}

// formatDocSymbols formats hierarchical DocumentSymbol trees with indentation.
func formatDocSymbols(sb *strings.Builder, symbols []lsp.DocumentSymbol, depth int) {
	indent := strings.Repeat("  ", depth+1)
	for _, s := range symbols {
		sb.WriteString(fmt.Sprintf("%s[%s] %s  line %d\n", indent, s.Kind.String(), s.Name, s.Range.Start.Line+1))
		if len(s.Children) > 0 {
			formatDocSymbols(sb, s.Children, depth+1)
		}
	}
}

func errResult(format string, args ...any) *models.ToolResult {
	return &models.ToolResult{
		Content: fmt.Sprintf(format, args...),
		IsError: true,
	}
}
