package lsp

import "encoding/json"

// Position in a text document (0-based line and character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location inside a resource.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// DiagnosticSeverity indicates the severity of a diagnostic.
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic represents a compiler/linter diagnostic.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     any                `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// TextDocumentIdentifier identifies a text document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentItem is an item to transfer a text document from client to server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int32  `json:"version"`
	Text       string `json:"text"`
}

// VersionedTextDocumentIdentifier identifies a specific version of a text document.
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int32  `json:"version"`
}

// TextDocumentContentChangeEvent describes a content change in a document.
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// TextEdit represents a textual edit applicable to a text document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CompletionItem represents a completion suggestion.
type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation any    `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
	SortText      string `json:"sortText,omitempty"`
}

// CompletionList represents a collection of completion items.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// MarkupContent represents a string value with a specific content type.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Hover represents the result of a hover request.
type Hover struct {
	Contents json.RawMessage `json:"contents"`
	Range    *Range          `json:"range,omitempty"`
}

// SymbolKind enumerates the kinds of symbols.
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)

// SymbolKindString returns a human-readable label for a SymbolKind.
func (k SymbolKind) String() string {
	switch k {
	case SymbolKindFile:
		return "file"
	case SymbolKindModule:
		return "module"
	case SymbolKindNamespace:
		return "namespace"
	case SymbolKindPackage:
		return "package"
	case SymbolKindClass:
		return "class"
	case SymbolKindMethod:
		return "method"
	case SymbolKindProperty:
		return "property"
	case SymbolKindField:
		return "field"
	case SymbolKindConstructor:
		return "constructor"
	case SymbolKindEnum:
		return "enum"
	case SymbolKindInterface:
		return "interface"
	case SymbolKindFunction:
		return "func"
	case SymbolKindVariable:
		return "variable"
	case SymbolKindConstant:
		return "const"
	case SymbolKindString:
		return "string"
	case SymbolKindNumber:
		return "number"
	case SymbolKindBoolean:
		return "boolean"
	case SymbolKindArray:
		return "array"
	case SymbolKindObject:
		return "object"
	case SymbolKindKey:
		return "key"
	case SymbolKindNull:
		return "null"
	case SymbolKindEnumMember:
		return "enum member"
	case SymbolKindStruct:
		return "struct"
	case SymbolKindEvent:
		return "event"
	case SymbolKindOperator:
		return "operator"
	case SymbolKindTypeParameter:
		return "type param"
	default:
		return "unknown"
	}
}

// SymbolInformation represents information about programming constructs.
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// DocumentSymbol represents a document symbol (hierarchical).
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// WorkspaceFolder represents a workspace folder.
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// InitializeParams are the parameters for the initialize request.
type InitializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri"`
	WorkspaceFolders      []WorkspaceFolder  `json:"workspaceFolders,omitempty"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	InitializationOptions any                `json:"initializationOptions,omitempty"`
}

// ClientCapabilities describe the client's capabilities.
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	General      GeneralCapabilities            `json:"general,omitempty"`
}

// TextDocumentClientCapabilities describe text document client capabilities.
type TextDocumentClientCapabilities struct {
	Synchronization    SyncCapabilities           `json:"synchronization,omitempty"`
	PublishDiagnostics DiagnosticsCapabilities    `json:"publishDiagnostics,omitempty"`
	Hover              HoverCapabilities          `json:"hover,omitempty"`
	Definition         DefinitionCapabilities     `json:"definition,omitempty"`
	References         ReferencesCapabilities     `json:"references,omitempty"`
	DocumentSymbol     DocumentSymbolCapabilities `json:"documentSymbol,omitempty"`
	Completion         CompletionCapabilities     `json:"completion,omitempty"`
}

// SyncCapabilities describe synchronization capabilities.
type SyncCapabilities struct {
	DidSave bool `json:"didSave,omitempty"`
}

// DiagnosticsCapabilities describe diagnostics capabilities.
type DiagnosticsCapabilities struct {
	RelatedInformation bool `json:"relatedInformation,omitempty"`
}

// HoverCapabilities describe hover capabilities.
type HoverCapabilities struct {
	ContentFormat []string `json:"contentFormat,omitempty"`
}

// DefinitionCapabilities describe definition capabilities.
type DefinitionCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

// ReferencesCapabilities describe references capabilities.
type ReferencesCapabilities struct{}

// DocumentSymbolCapabilities describe document symbol capabilities.
type DocumentSymbolCapabilities struct {
	HierarchicalSupport bool `json:"hierarchicalDocumentSymbolSupport,omitempty"`
}

// CompletionCapabilities describe completion capabilities.
type CompletionCapabilities struct {
	SnippetSupport bool `json:"snippetSupport,omitempty"`
}

// GeneralCapabilities describe general client capabilities.
type GeneralCapabilities struct {
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

// InitializeResult is the result of an initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities describe the server's capabilities.
type ServerCapabilities struct {
	TextDocumentSync        any `json:"textDocumentSync,omitempty"`
	CompletionProvider      any `json:"completionProvider,omitempty"`
	HoverProvider           any `json:"hoverProvider,omitempty"`
	DefinitionProvider      any `json:"definitionProvider,omitempty"`
	ReferencesProvider      any `json:"referencesProvider,omitempty"`
	DocumentSymbolProvider  any `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider any `json:"workspaceSymbolProvider,omitempty"`
	DiagnosticProvider      any `json:"diagnosticProvider,omitempty"`
	RenameProvider          any `json:"renameProvider,omitempty"`
}

// DidOpenTextDocumentParams are the parameters for textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams are the parameters for textDocument/didChange.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidSaveTextDocumentParams are the parameters for textDocument/didSave.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidCloseTextDocumentParams are the parameters for textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// TextDocumentPositionParams are the parameters for position-based requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ReferenceParams are the parameters for textDocument/references.
type ReferenceParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	Context      ReferenceContext       `json:"context"`
}

// ReferenceContext controls whether the declaration is included in references.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// CompletionParams are the parameters for textDocument/completion.
type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// DocumentSymbolParams are the parameters for textDocument/documentSymbol.
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// WorkspaceSymbolParams are the parameters for workspace/symbol.
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// PublishDiagnosticsParams are the parameters for textDocument/publishDiagnostics.
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// RenameParams are the parameters for textDocument/rename.
type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

// WorkspaceEdit represents changes to many resources managed in the workspace.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// PathToURI converts an absolute file path to a file:// URI.
func PathToURI(path string) string {
	return "file://" + path
}

// URIToPath converts a file:// URI back to an absolute path.
func URIToPath(uri string) string {
	const prefix = "file://"
	if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
		return uri[len(prefix):]
	}
	return uri
}
