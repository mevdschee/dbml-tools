package analysis

import (
	"dbml-tools/lexer"
	"dbml-tools/parser"
	"sort"
	"strings"
)

// CompletionKind tags completion items so the LSP wire layer can map them to
// the LSP CompletionItemKind enum.
type CompletionKind int

const (
	CompletionKeyword CompletionKind = iota + 1
	CompletionTable
	CompletionColumn
	CompletionEnum
	CompletionEnumValue
	CompletionTypeName
	CompletionAttribute
	CompletionOperator
	CompletionValue
	CompletionAlias
)

// Completion is a proposed item with text, kind, doc, and the replacement range.
type Completion struct {
	Label        string
	Kind         CompletionKind
	Detail       string
	Doc          string
	InsertText   string // may include $-snippet placeholders; falls back to Label
	ReplaceRange Range
}

// Completions returns the list of completion items at offset. Returns an
// empty slice when nothing is appropriate.
func (a *Analysis) Completions(offset int) []Completion {
	ctx := a.classifyCompletion(offset)
	prefix, prefixRange := a.prefixAt(offset)
	items := a.itemsForContext(ctx)
	for i := range items {
		if items[i].ReplaceRange == (Range{}) {
			items[i].ReplaceRange = prefixRange
		}
		if items[i].InsertText == "" {
			items[i].InsertText = items[i].Label
		}
	}
	if prefix != "" {
		items = filterByPrefix(items, prefix)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Label < items[j].Label
	})
	return items
}

// ---------------------------------------------------------------------------
// Context classification
// ---------------------------------------------------------------------------

type completionCtx int

const (
	ctxNone completionCtx = iota
	ctxTopLevel
	ctxColumnType
	ctxAttributeName
	ctxAttributeValueDefault
	ctxRefEndpointTable
	ctxRefEndpointColumn // includes a "tableHint"
	ctxInlineRefTable
	ctxInlineRefColumn
	ctxTableGroupBody
	ctxProjectSettingKey
	ctxProjectSettingValueDB
	ctxRelOp
	ctxInsideString
)

type completionContext struct {
	kind      completionCtx
	tableHint string // for column-list completions
}

func (a *Analysis) classifyCompletion(offset int) completionContext {
	// Inside a string literal → no completion.
	if tok := a.TokenAt(offset); tok != nil &&
		(tok.Kind == lexer.KindString || tok.Kind == lexer.KindFuncExpr) {
		return completionContext{kind: ctxInsideString}
	}

	chain := a.Spans.Innermost(offset)

	// Determine the enclosing ElementDecl + relevant containers.
	// We fall back to a token-based lookup if the cursor is past the AST.
	var enclosingDecl *parser.ElementDeclNode
	var enclosingList *parser.ListExprNode
	var enclosingBlock *parser.BlockExprNode
	for _, n := range chain {
		switch nn := n.(type) {
		case *parser.ElementDeclNode:
			enclosingDecl = nn
		case *parser.ListExprNode:
			enclosingList = nn
		case *parser.BlockExprNode:
			enclosingBlock = nn
		}
	}
	if enclosingDecl == nil {
		enclosingDecl = a.enclosingDeclByToken(offset)
	}
	if enclosingDecl != nil && enclosingBlock == nil {
		if b, ok := enclosingDecl.Body.(*parser.BlockExprNode); ok {
			enclosingBlock = b
		}
	}

	// Inside [ ... ] settings list.
	if enclosingList != nil {
		// Determine if we're in an attribute *value* position (after a colon)
		// or in a name position. Walk attributes and check.
		if attr := attributeAtOffset(enclosingList, offset); attr != nil {
			if attr.ColonTok != nil && offset > attr.ColonTok.End {
				// We're in value position.
				name, _ := extractAttributeNameFull(attr.Name)
				switch strings.ToLower(name) {
				case "default":
					return completionContext{kind: ctxAttributeValueDefault}
				case "ref":
					// Inline ref: parse the value to figure out whether we
					// need a table or column.
					tableHint := inlineRefTableHint(attr.Value, offset)
					if tableHint != "" {
						return completionContext{kind: ctxInlineRefColumn, tableHint: tableHint}
					}
					return completionContext{kind: ctxInlineRefTable}
				}
				return completionContext{kind: ctxNone}
			}
		}
		// Otherwise, attribute name position.
		return completionContext{kind: ctxAttributeName}
	}

	// Inside Project { ... }
	if enclosingDecl != nil && enclosingDecl.Type.Value == "Project" && enclosingBlock != nil {
		// Determine if cursor is after "database_type:" on this line.
		key, afterColon := a.projectLineContext(offset)
		if afterColon {
			if strings.EqualFold(key, "database_type") {
				return completionContext{kind: ctxProjectSettingValueDB}
			}
			return completionContext{kind: ctxNone}
		}
		return completionContext{kind: ctxProjectSettingKey}
	}

	// Inside TableGroup { ... }
	if enclosingDecl != nil && enclosingDecl.Type.Value == "TableGroup" && enclosingBlock != nil {
		return completionContext{kind: ctxTableGroupBody}
	}

	// Inside a Ref declaration (body of Ref: ... or Ref { ... })
	if enclosingDecl != nil && enclosingDecl.Type.Value == "Ref" {
		ctxK, tableHint := a.classifyRefBodyByTokens(enclosingDecl, offset)
		return completionContext{kind: ctxK, tableHint: tableHint}
	}

	// Inside a Table block: classify by line tokens.
	if enclosingDecl != nil && enclosingDecl.Type.Value == "Table" && enclosingBlock != nil {
		if a.atColumnTypeSlot(offset, enclosingBlock) {
			return completionContext{kind: ctxColumnType}
		}
		return completionContext{kind: ctxNone}
	}

	// Top-level (outside any declaration body).
	return completionContext{kind: ctxTopLevel}
}

// enclosingDeclByToken finds the declaration whose token range *or trailing
// whitespace/newline* contains offset. Used when the cursor is past the AST.
func (a *Analysis) enclosingDeclByToken(offset int) *parser.ElementDeclNode {
	if a.Program == nil {
		return nil
	}
	var best *parser.ElementDeclNode
	for _, n := range a.Program.Body {
		d, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		start := d.FirstToken().Start
		end := d.LastToken().End
		// Include trailing newline/whitespace via the next decl's start.
		if offset >= start && offset <= end {
			return d
		}
		if start <= offset {
			best = d
		} else {
			break
		}
	}
	// If the best decl has a brace body, accept when inside the body's braces.
	// For colon-bodied decls, accept when cursor is on the same line as the decl
	// (no newline between last token of decl and offset).
	if best != nil {
		if b, ok := best.Body.(*parser.BlockExprNode); ok {
			if offset >= b.Open.Start && offset <= b.Close.End {
				return best
			}
		} else if best.Colon != nil {
			lastEnd := best.LastToken().End
			runes := []rune(a.Source)
			ok := true
			for i := lastEnd; i < offset && i < len(runes); i++ {
				if runes[i] == '\n' {
					ok = false
					break
				}
			}
			if ok {
				return best
			}
		}
	}
	return nil
}

// classifyRefBodyByTokens drives ref-body classification from tokens, which
// handles cursor positions past the parsed AST.
func (a *Analysis) classifyRefBodyByTokens(decl *parser.ElementDeclNode, offset int) (completionCtx, string) {
	// Determine "ref body start" — after `Ref name? :` or inside `{ ... }`.
	var bodyStart int
	if b, ok := decl.Body.(*parser.BlockExprNode); ok {
		bodyStart = b.Open.End
	} else if decl.Colon != nil {
		bodyStart = decl.Colon.End
	} else {
		bodyStart = decl.Type.End
	}
	// The previous significant token before offset, within the body.
	prev, prevPrev := a.prevTwoSignificant(offset, bodyStart)
	if prev == nil {
		return ctxRefEndpointTable, ""
	}
	if prev.Kind == lexer.KindOp && prev.Value == "." {
		if prevPrev != nil && (prevPrev.Kind == lexer.KindIdentifier || prevPrev.Kind == lexer.KindQuotedString) {
			return ctxRefEndpointColumn, prevPrev.Value
		}
		return ctxRefEndpointColumn, ""
	}
	if prev.Kind == lexer.KindOp && IsRelationshipOp(prev.Value) {
		return ctxRefEndpointTable, ""
	}
	if prev.Kind == lexer.KindColon || prev.Kind == lexer.KindLBrace {
		return ctxRefEndpointTable, ""
	}
	if prev.Kind == lexer.KindNewline {
		return ctxRefEndpointTable, ""
	}
	if prev.Kind == lexer.KindIdentifier || prev.Kind == lexer.KindQuotedString {
		// After an identifier: expecting either `.` or a relop.
		return ctxRelOp, ""
	}
	return ctxRefEndpointTable, ""
}

// prevTwoSignificant returns the last two significant tokens with End <= offset
// and Start >= minOffset.
func (a *Analysis) prevTwoSignificant(offset, minOffset int) (*lexer.Token, *lexer.Token) {
	var p1, p2 *lexer.Token
	for i := range a.Tokens {
		t := &a.Tokens[i]
		if t.Kind == lexer.KindEOF {
			break
		}
		if t.Start < minOffset {
			continue
		}
		if t.End > offset {
			break
		}
		p2 = p1
		p1 = t
	}
	return p1, p2
}

// atColumnTypeSlot returns true if the cursor is positioned where a column
// type token would be entered: same line as a column identifier, after it.
func (a *Analysis) atColumnTypeSlot(offset int, block *parser.BlockExprNode) bool {
	// AST-based first: is the offset inside an existing type-arg node?
	if a.isColumnTypePosition(offset) {
		return true
	}
	// Token-based: look at the tokens on the same line within the block.
	lineStart, lineEnd := a.lineExtent(offset)
	if lineEnd > block.Close.Start {
		lineEnd = block.Close.Start
	}
	if lineStart < block.Open.End {
		lineStart = block.Open.End
	}
	// Count significant tokens on this line that come before the cursor.
	var idents []*lexer.Token
	for i := range a.Tokens {
		t := &a.Tokens[i]
		if t.Kind == lexer.KindEOF {
			break
		}
		if t.End <= lineStart {
			continue
		}
		if t.Start >= offset {
			break
		}
		if t.Start >= lineEnd {
			break
		}
		// Skip newlines/spaces (they're stored as trivia, not significant).
		if t.Kind == lexer.KindLBracket || t.Kind == lexer.KindRBracket {
			// already past the type slot
			return false
		}
		if t.Kind == lexer.KindIdentifier || t.Kind == lexer.KindQuotedString {
			idents = append(idents, t)
		}
	}
	// Exactly one identifier on this line before the cursor → at type slot.
	return len(idents) == 1
}

// lineExtent returns the [start, end) rune offsets of the line containing offset.
func (a *Analysis) lineExtent(offset int) (int, int) {
	runes := []rune(a.Source)
	start := offset
	for start > 0 && runes[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(runes) && runes[end] != '\n' {
		end++
	}
	return start, end
}

// projectLineContext returns the setting key on the current line of a Project
// block and whether the cursor is past the `:`.
func (a *Analysis) projectLineContext(offset int) (string, bool) {
	lineStart, lineEnd := a.lineExtent(offset)
	var key string
	var colonEnd int
	for i := range a.Tokens {
		t := &a.Tokens[i]
		if t.Kind == lexer.KindEOF {
			break
		}
		if t.End <= lineStart {
			continue
		}
		if t.Start >= lineEnd || t.Start >= offset {
			break
		}
		if t.Kind == lexer.KindIdentifier && key == "" {
			key = t.Value
			continue
		}
		if t.Kind == lexer.KindColon {
			colonEnd = t.End
		}
	}
	if key != "" && colonEnd > 0 && offset > colonEnd {
		return key, true
	}
	return key, false
}

// classifyRefBody figures out where the cursor is inside a Ref's body.
func classifyRefBody(decl *parser.ElementDeclNode, offset int, tokens []lexer.Token) (completionCtx, string) {
	// Compute the body range.
	if decl.Body == nil {
		return ctxNone, ""
	}
	bodyStart := decl.Body.FirstToken().Start
	bodyEnd := decl.Body.LastToken().End
	if offset < bodyStart || offset > bodyEnd {
		return ctxNone, ""
	}

	// Look at the previous non-whitespace, non-newline significant token.
	// Walk back through tokens.
	var prev *lexer.Token
	for i := range tokens {
		t := tokens[i]
		if t.Kind == lexer.KindEOF {
			break
		}
		if t.End > offset {
			break
		}
		// On exact boundary, prefer the token whose End == offset.
		prev = &tokens[i]
	}
	if prev == nil {
		return ctxNone, ""
	}

	// Just after `.` → column position. The token before the dot is the
	// table name (or alias).
	if prev.Kind == lexer.KindOp && prev.Value == "." {
		// Find token before this dot.
		for i := range tokens {
			if tokens[i].Start == prev.Start && tokens[i].End == prev.End {
				if i > 0 {
					before := tokens[i-1]
					if before.Kind == lexer.KindIdentifier || before.Kind == lexer.KindQuotedString {
						return ctxRefEndpointColumn, before.Value
					}
				}
			}
		}
		return ctxRefEndpointColumn, ""
	}
	// Just after a relop or `:` (Ref: ...) or `{` or newline (block body) →
	// table position. We approximate by recognizing the relop / colon / brace.
	if prev.Kind == lexer.KindOp && IsRelationshipOp(prev.Value) {
		return ctxRefEndpointTable, ""
	}
	if prev.Kind == lexer.KindColon || prev.Kind == lexer.KindLBrace {
		return ctxRefEndpointTable, ""
	}
	// Identifier with no dot following → could be relop position next.
	if prev.Kind == lexer.KindIdentifier || prev.Kind == lexer.KindQuotedString {
		// If the cursor is on a new line vs. prev, treat as ref-table start.
		// Otherwise expect a `.` or relop next.
		// For an identifier already typed (with space following), suggest relops.
		// Heuristic: if the next char after prev.End is whitespace, suggest relops.
		return ctxRelOp, ""
	}
	return ctxRefEndpointTable, ""
}

// attributeAtOffset returns the AttributeNode under offset within a list.
func attributeAtOffset(list *parser.ListExprNode, offset int) *parser.AttributeNode {
	for _, it := range list.Items {
		attr, ok := it.(*parser.AttributeNode)
		if !ok {
			continue
		}
		r := Range{attr.FirstToken().Start, attr.LastToken().End}
		if offset >= r.Start && offset <= r.End {
			return attr
		}
	}
	return nil
}

// projectSettingAtOffset reports the current setting key + whether cursor is
// after the `:` on this line.
func projectSettingAtOffset(block *parser.BlockExprNode, offset int) (string, bool) {
	for _, item := range block.Body {
		fa, ok := item.(*parser.FuncAppNode)
		if !ok {
			continue
		}
		r := Range{fa.FirstToken().Start, fa.LastToken().End}
		if offset < r.Start || offset > r.End {
			continue
		}
		key := strings.ToLower(extractFirstName(fa.Callee))
		// look for colon in args
		for _, arg := range fa.Args {
			pe, ok := arg.(*parser.PrimaryExprNode)
			if !ok {
				continue
			}
			vn, ok := pe.Expr.(*parser.VariableNode)
			if !ok {
				continue
			}
			if vn.Token.Kind == lexer.KindColon && offset > vn.Token.End {
				return key, true
			}
		}
		return key, false
	}
	return "", false
}

// inlineRefTableHint inspects the value of `[ref: ...]` to decide if we're
// already past `<table>.` and need a column completion. Returns the table
// hint or "".
func inlineRefTableHint(value parser.Node, offset int) string {
	if value == nil {
		return ""
	}
	atoms := flatten(value)
	// Find a `.` before the offset whose preceding atom is an identifier;
	// the identifier becomes the table hint.
	var lastIdent string
	for _, at := range atoms {
		if at.tuple != nil {
			continue
		}
		if at.tok.End > offset {
			break
		}
		if at.isOp(".") {
			return lastIdent
		}
		if at.isIdent() {
			lastIdent = at.tok.Value
		} else {
			lastIdent = ""
		}
	}
	return ""
}

// prefixAt returns the partial identifier ending at offset and its rune range.
func (a *Analysis) prefixAt(offset int) (string, Range) {
	runes := []rune(a.Source)
	end := offset
	if end > len(runes) {
		end = len(runes)
	}
	start := end
	for start > 0 {
		r := runes[start-1]
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			break
		}
		start--
	}
	return string(runes[start:end]), Range{start, end}
}

func filterByPrefix(items []Completion, prefix string) []Completion {
	low := strings.ToLower(prefix)
	out := items[:0]
	for _, it := range items {
		if strings.HasPrefix(strings.ToLower(it.Label), low) {
			out = append(out, it)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Items per context
// ---------------------------------------------------------------------------

func (a *Analysis) itemsForContext(ctx completionContext) []Completion {
	switch ctx.kind {
	case ctxTopLevel:
		return keywordItems()
	case ctxColumnType:
		out := builtinTypeItems()
		for _, e := range a.Symbols.AllEnums {
			out = append(out, Completion{Label: e.Name, Kind: CompletionEnum, Detail: "enum"})
		}
		return out
	case ctxAttributeName:
		return attributeItems()
	case ctxAttributeValueDefault:
		return []Completion{
			{Label: "null", Kind: CompletionValue},
			{Label: "true", Kind: CompletionValue},
			{Label: "false", Kind: CompletionValue},
			{Label: "`now()`", Kind: CompletionValue, Detail: "expression"},
		}
	case ctxRefEndpointTable, ctxInlineRefTable:
		return a.tableItems()
	case ctxRefEndpointColumn, ctxInlineRefColumn:
		return a.columnItemsFor(ctx.tableHint)
	case ctxTableGroupBody:
		return a.tableItems()
	case ctxProjectSettingKey:
		out := make([]Completion, 0, len(ProjectSettings))
		for _, s := range ProjectSettings {
			out = append(out, Completion{Label: s.Name, Kind: CompletionKeyword, Doc: s.Doc})
		}
		return out
	case ctxProjectSettingValueDB:
		return []Completion{
			{Label: "'MariaDB'", Kind: CompletionValue},
			{Label: "'PostgreSQL'", Kind: CompletionValue},
			{Label: "'SQLite'", Kind: CompletionValue},
			{Label: "'MariaDB normalized'", Kind: CompletionValue},
			{Label: "'PostgreSQL normalized'", Kind: CompletionValue},
			{Label: "'SQLite normalized'", Kind: CompletionValue},
		}
	case ctxRelOp:
		return []Completion{
			{Label: ">", Kind: CompletionOperator, Doc: "many-to-one"},
			{Label: "<", Kind: CompletionOperator, Doc: "one-to-many"},
			{Label: "-", Kind: CompletionOperator, Doc: "one-to-one"},
			{Label: "<>", Kind: CompletionOperator, Doc: "many-to-many"},
		}
	case ctxInsideString, ctxNone:
		return nil
	}
	return nil
}

func keywordItems() []Completion {
	out := make([]Completion, 0, len(Keywords))
	for _, kw := range Keywords {
		out = append(out, Completion{
			Label:      kw.Name,
			Kind:       CompletionKeyword,
			Doc:        kw.Doc,
			InsertText: kw.Insert,
		})
	}
	return out
}

func builtinTypeItems() []Completion {
	out := make([]Completion, 0, len(BuiltinTypes))
	for _, bt := range BuiltinTypes {
		ins := bt.Name + bt.SnippetArgs
		out = append(out, Completion{
			Label:      bt.Name,
			Kind:       CompletionTypeName,
			Doc:        bt.Doc,
			InsertText: ins,
		})
	}
	return out
}

func attributeItems() []Completion {
	out := make([]Completion, 0, len(ColumnAttributes))
	for _, at := range ColumnAttributes {
		ins := at.Name
		if at.TakesValue {
			ins = at.Name + ": $0"
		}
		out = append(out, Completion{
			Label:      at.Name,
			Kind:       CompletionAttribute,
			Doc:        at.Doc,
			InsertText: ins,
		})
	}
	return out
}

func (a *Analysis) tableItems() []Completion {
	var out []Completion
	for _, t := range a.Symbols.AllTables {
		out = append(out, Completion{
			Label:  t.Name,
			Kind:   CompletionTable,
			Detail: "table",
		})
	}
	for _, al := range a.Symbols.Aliases {
		out = append(out, Completion{
			Label:  al.Name,
			Kind:   CompletionAlias,
			Detail: "alias of " + al.Parent.Name,
		})
	}
	return out
}

func (a *Analysis) columnItemsFor(tableHint string) []Completion {
	tbl := a.Symbols.ResolveTable(tableHint)
	if tbl == nil {
		return nil
	}
	out := make([]Completion, 0, len(tbl.Columns))
	for _, c := range tbl.Columns {
		t := c.TypeName
		if len(c.TypeArgs) > 0 {
			t += "(" + strings.Join(c.TypeArgs, ",") + ")"
		}
		out = append(out, Completion{
			Label:  c.Name,
			Kind:   CompletionColumn,
			Detail: t,
		})
	}
	return out
}
