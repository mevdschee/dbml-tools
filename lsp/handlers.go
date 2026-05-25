package lsp

import (
	"dbml-tools/analysis"
	"dbml-tools/interpreter"
	"dbml-tools/lexer"
	"dbml-tools/parser"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (s *server) onInitialize(_ json.RawMessage) (interface{}, *rpcError) {
	res := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: 1, // Full
			HoverProvider:    true,
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{".", " ", "[", ":", ">"},
			},
			DefinitionProvider:     true,
			ReferencesProvider:     true,
			RenameProvider:         &RenameOptions{PrepareProvider: true},
			DocumentSymbolProvider: true,
		},
	}
	res.ServerInfo.Name = "dbml-tools"
	res.ServerInfo.Version = "0.1.0"
	return res, nil
}

func (s *server) onShutdown(_ json.RawMessage) (interface{}, *rpcError) {
	return nil, nil
}

func (s *server) onExit(_ json.RawMessage) {
	close(s.closed)
}

// ---------------------------------------------------------------------------
// Document sync
// ---------------------------------------------------------------------------

func (s *server) onDidOpen(raw json.RawMessage) {
	var p DidOpenParams
	if err := json.Unmarshal(raw, &p); err != nil {
		s.log.Printf("didOpen parse: %v", err)
		return
	}
	d := newDocument(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
	s.setDoc(d)
	s.publishDiagnostics(d)
}

func (s *server) onDidChange(raw json.RawMessage) {
	var p DidChangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		s.log.Printf("didChange parse: %v", err)
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		d = newDocument(p.TextDocument.URI, p.TextDocument.Version, text)
	} else {
		d.version = p.TextDocument.Version
		d.update(text)
	}
	s.setDoc(d)
	s.publishDiagnostics(d)
}

func (s *server) onDidClose(raw json.RawMessage) {
	var p DidCloseParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	s.deleteDoc(p.TextDocument.URI)
	// Clear diagnostics for the closed doc.
	s.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI: p.TextDocument.URI, Diagnostics: []Diagnostic{},
	})
}

func (s *server) publishDiagnostics(d *document) {
	a := d.analysis
	var diags []Diagnostic
	for _, e := range a.LexErrors {
		diags = append(diags, mkDiag(d, e.Position, e.Message, "lexer"))
	}
	for _, e := range a.ParseErrors {
		diags = append(diags, mkDiag(d, e.Position, e.Message, "parser"))
	}
	for _, e := range a.InterpErrors {
		diags = append(diags, mkDiag(d, e.Position, e.Message, "interpreter"))
	}
	if diags == nil {
		diags = []Diagnostic{}
	}
	s.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI: d.uri, Diagnostics: diags,
	})
}

func mkDiag(d *document, pos lexer.Position, msg, source string) Diagnostic {
	// Use the rune offset from pos and convert via the doc's mapping. We
	// produce a zero-length range at that position — clients render it as a
	// single-char squiggle.
	p := d.runeOffsetToPosition(pos.Offset)
	return Diagnostic{
		Range:    LSPRange{Start: p, End: p},
		Severity: severityError,
		Source:   source,
		Message:  msg,
	}
}

// ---------------------------------------------------------------------------
// Hover
// ---------------------------------------------------------------------------

func (s *server) onHover(raw json.RawMessage) (interface{}, *rpcError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return nil, nil
	}
	off := d.positionToRuneOffset(p.Position)
	h := d.analysis.Hover(off)
	if h == nil {
		return nil, nil
	}
	rng := d.analysisRange(h.Range)
	return Hover{
		Contents: MarkupContent{Kind: "markdown", Value: h.Markdown},
		Range:    &rng,
	}, nil
}

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

func (s *server) onCompletion(raw json.RawMessage) (interface{}, *rpcError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return CompletionList{Items: []CompletionItem{}}, nil
	}
	off := d.positionToRuneOffset(p.Position)
	items := d.analysis.Completions(off)
	out := make([]CompletionItem, 0, len(items))
	for _, it := range items {
		ci := CompletionItem{
			Label:         it.Label,
			Kind:          mapCompletionKind(it.Kind),
			Detail:        it.Detail,
			Documentation: it.Doc,
		}
		if hasSnippetSyntax(it.InsertText) {
			ci.InsertTextFormat = completionSnippet
		} else {
			ci.InsertTextFormat = completionPlainText
		}
		rng := d.analysisRange(it.ReplaceRange)
		ci.TextEdit = &TextEdit{Range: rng, NewText: it.InsertText}
		out = append(out, ci)
	}
	return CompletionList{Items: out}, nil
}

func mapCompletionKind(k analysis.CompletionKind) int {
	switch k {
	case analysis.CompletionKeyword:
		return ciKeyword
	case analysis.CompletionTable:
		return ciClass
	case analysis.CompletionColumn:
		return ciField
	case analysis.CompletionEnum:
		return ciEnum
	case analysis.CompletionEnumValue:
		return ciEnumMember
	case analysis.CompletionTypeName:
		return ciTypeParam
	case analysis.CompletionAttribute:
		return ciProperty
	case analysis.CompletionOperator:
		return ciOperator
	case analysis.CompletionValue:
		return ciVariable
	case analysis.CompletionAlias:
		return ciVariable
	}
	return ciText
}

func hasSnippetSyntax(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '$' && (s[i+1] == '{' || (s[i+1] >= '0' && s[i+1] <= '9')) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Definition + References
// ---------------------------------------------------------------------------

func (s *server) onDefinition(raw json.RawMessage) (interface{}, *rpcError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return nil, nil
	}
	off := d.positionToRuneOffset(p.Position)
	r := d.analysis.Definition(off)
	if r == nil {
		return nil, nil
	}
	return Location{URI: p.TextDocument.URI, Range: d.analysisRange(*r)}, nil
}

func (s *server) onReferences(raw json.RawMessage) (interface{}, *rpcError) {
	var p ReferenceParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return []Location{}, nil
	}
	off := d.positionToRuneOffset(p.Position)
	// Resolve the symbol at offset and find references.
	a := d.analysis
	var sym *analysis.Symbol
	if r := a.RefAt(off); r != nil {
		sym = r.Target
	}
	if sym == nil {
		// At a declaration?
		// Use Definition to land on the def-name range, then look up the
		// symbol by that range.
		if def := a.Definition(off); def != nil {
			sym = symbolAtNameRange(a, *def)
		}
	}
	if sym == nil {
		return []Location{}, nil
	}
	rngs := a.ReferencesOf(sym, p.Context.IncludeDeclaration)
	out := make([]Location, 0, len(rngs))
	for _, r := range rngs {
		out = append(out, Location{URI: p.TextDocument.URI, Range: d.analysisRange(r)})
	}
	return out, nil
}

// symbolAtNameRange returns the symbol whose NameRange exactly matches r.
func symbolAtNameRange(a *analysis.Analysis, r analysis.Range) *analysis.Symbol {
	for _, t := range a.Symbols.AllTables {
		if t.NameRange == r {
			return t
		}
		for _, c := range t.Columns {
			if c.NameRange == r {
				return c
			}
		}
	}
	for _, e := range a.Symbols.AllEnums {
		if e.NameRange == r {
			return e
		}
		for _, v := range e.EnumValues {
			if v.NameRange == r {
				return v
			}
		}
	}
	for _, tg := range a.Symbols.TableGroups {
		if tg.NameRange == r {
			return tg
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Rename
// ---------------------------------------------------------------------------

func (s *server) onPrepareRename(raw json.RawMessage) (interface{}, *rpcError) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return nil, nil
	}
	off := d.positionToRuneOffset(p.Position)
	pr, err := d.analysis.PrepareRename(off)
	if err != nil {
		return nil, &rpcError{Code: errInvalidRequest, Message: err.Error()}
	}
	return PrepareRenameResult{
		Range:       d.analysisRange(pr.Range),
		Placeholder: pr.Placeholder,
	}, nil
}

func (s *server) onRename(raw json.RawMessage) (interface{}, *rpcError) {
	var p RenameParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return nil, nil
	}
	off := d.positionToRuneOffset(p.Position)
	edits, err := d.analysis.Rename(off, p.NewName)
	if err != nil {
		return nil, &rpcError{Code: errInvalidRequest, Message: err.Error()}
	}
	out := make([]TextEdit, 0, len(edits))
	for _, e := range edits {
		out = append(out, TextEdit{Range: d.analysisRange(e.Range), NewText: e.NewText})
	}
	return WorkspaceEdit{Changes: map[string][]TextEdit{p.TextDocument.URI: out}}, nil
}

// ---------------------------------------------------------------------------
// Document symbol (outline)
// ---------------------------------------------------------------------------

func (s *server) onDocumentSymbol(raw json.RawMessage) (interface{}, *rpcError) {
	var p DocumentSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, paramsErr(err)
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		return []DocumentSymbol{}, nil
	}
	syms := buildDocumentSymbols(d, d.analysis.Database)
	return syms, nil
}

func buildDocumentSymbols(d *document, db *interpreter.Database) []DocumentSymbol {
	var out []DocumentSymbol
	for _, t := range db.Tables {
		ds := DocumentSymbol{
			Name:           t.Name,
			Kind:           skClass,
			Range:          d.analysisRange(analysis.Range{Start: t.Token.Start.Offset, End: t.Token.End.Offset}),
			SelectionRange: d.analysisRange(analysis.Range{Start: t.Token.Start.Offset, End: t.Token.End.Offset}),
		}
		for _, c := range t.Fields {
			r := analysis.Range{Start: c.Token.Start.Offset, End: c.Token.End.Offset}
			ds.Children = append(ds.Children, DocumentSymbol{
				Name:           c.Name,
				Detail:         c.Type.TypeName,
				Kind:           skField,
				Range:          d.analysisRange(r),
				SelectionRange: d.analysisRange(r),
			})
		}
		out = append(out, ds)
	}
	for _, e := range db.Enums {
		ds := DocumentSymbol{
			Name:           e.Name,
			Kind:           skEnum,
			Range:          d.analysisRange(analysis.Range{Start: e.Token.Start.Offset, End: e.Token.End.Offset}),
			SelectionRange: d.analysisRange(analysis.Range{Start: e.Token.Start.Offset, End: e.Token.End.Offset}),
		}
		out = append(out, ds)
	}
	return out
}

var _ = parser.ProgramNode{} // keep import; future use
