// Package lsp implements a Language Server Protocol server for DBML.
// It wraps the analysis package and exposes diagnostics, hover, completion,
// definition, references, document symbols, and rename over JSON-RPC.
package lsp

import "encoding/json"

// JSON-RPC 2.0 message envelopes.

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}
	Error   *rpcError
}

// MarshalJSON enforces the JSON-RPC 2.0 invariant: a response carries exactly
// one of `result` or `error`. With `omitempty` on Result, a nil result (e.g.
// hover with no info) would drop the field entirely and the client would see
// neither — vscode-languageclient rejects that as an "invalid message".
func (r *rpcResponse) MarshalJSON() ([]byte, error) {
	if r.Error != nil {
		return json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *rpcError       `json:"error"`
		}{r.JSONRPC, r.ID, r.Error})
	}
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  interface{}     `json:"result"`
	}{r.JSONRPC, r.ID, r.Result})
}

type rpcNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errParse           = -32700
	errInvalidRequest  = -32600
	errMethodNotFound  = -32601
	errInvalidParams   = -32602
	errInternal        = -32603
)

// ---------------------------------------------------------------------------
// LSP types (subset)
// ---------------------------------------------------------------------------

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type LSPRange struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string   `json:"uri"`
	Range LSPRange `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type DidOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"` // full-sync only (we advertise sync=Full)
}

type DidCloseParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

type RenameParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
	NewName      string                 `json:"newName"`
}

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *LSPRange     `json:"range,omitempty"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Label            string      `json:"label"`
	Kind             int         `json:"kind,omitempty"`
	Detail           string      `json:"detail,omitempty"`
	Documentation    string      `json:"documentation,omitempty"`
	InsertText       string      `json:"insertText,omitempty"`
	InsertTextFormat int         `json:"insertTextFormat,omitempty"`
	TextEdit         *TextEdit   `json:"textEdit,omitempty"`
}

const (
	completionPlainText = 1
	completionSnippet   = 2
)

// CompletionItemKind values per the LSP spec.
const (
	ciText        = 1
	ciMethod      = 2
	ciFunction    = 3
	ciField       = 5
	ciVariable    = 6
	ciClass       = 7
	ciInterface   = 8
	ciProperty    = 10
	ciEnum        = 13
	ciKeyword     = 14
	ciSnippet     = 15
	ciOperator    = 24
	ciTypeParam   = 25
	ciEnumMember  = 20
)

type TextEdit struct {
	Range   LSPRange `json:"range"`
	NewText string   `json:"newText"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

type Diagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

const (
	severityError = 1
	severityWarn  = 2
	severityInfo  = 3
	severityHint  = 4
)

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          LSPRange         `json:"range"`
	SelectionRange LSPRange         `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolKind values per LSP.
const (
	skClass        = 5
	skField        = 8
	skEnum         = 10
	skNamespace    = 3
	skEnumMember   = 22
)

// Server capabilities advertised in initialize.
type ServerCapabilities struct {
	TextDocumentSync         int                  `json:"textDocumentSync"`
	HoverProvider            bool                 `json:"hoverProvider"`
	CompletionProvider       *CompletionOptions   `json:"completionProvider,omitempty"`
	DefinitionProvider       bool                 `json:"definitionProvider"`
	ReferencesProvider       bool                 `json:"referencesProvider"`
	RenameProvider           *RenameOptions       `json:"renameProvider,omitempty"`
	DocumentSymbolProvider   bool                 `json:"documentSymbolProvider"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type RenameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

type PrepareRenameResult struct {
	Range       LSPRange `json:"range"`
	Placeholder string   `json:"placeholder"`
}
