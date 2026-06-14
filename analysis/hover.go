package analysis

import (
	"fmt"
	"strings"

	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

// HoverResult is the markdown payload and the range of the hovered token.
type HoverResult struct {
	Markdown string
	Range    Range
}

// Hover returns a HoverResult for the symbol at offset, or nil.
func (a *Analysis) Hover(offset int) *HoverResult {
	tok := a.TokenAt(offset)
	if tok == nil {
		return nil
	}

	// 1) Ref site (use site): defer to its target's symbol.
	if r := a.RefAt(offset); r != nil {
		switch r.Kind {
		case RefSiteColumn:
			if r.Target != nil {
				return &HoverResult{Markdown: renderColumn(r.Target), Range: r.SiteRange}
			}
			return &HoverResult{Markdown: fmt.Sprintf("`%s` — _unresolved column_", r.SourceText), Range: r.SiteRange}
		case RefSiteTable, RefSiteTableInGroup:
			if r.Target != nil {
				return &HoverResult{Markdown: renderTable(r.Target), Range: r.SiteRange}
			}
			return &HoverResult{Markdown: fmt.Sprintf("`%s` — _unresolved table_", r.SourceText), Range: r.SiteRange}
		case RefSiteEnumType:
			if r.Target != nil {
				return &HoverResult{Markdown: renderEnum(r.Target), Range: r.SiteRange}
			}
		}
	}

	// 2) Declaration sites: leaf token equals a declared symbol's name range.
	if sym := a.symbolAtDecl(offset); sym != nil {
		var md string
		switch sym.Kind {
		case SymTable, SymAlias:
			if sym.Kind == SymAlias && sym.Parent != nil {
				md = renderTable(sym.Parent)
			} else {
				md = renderTable(sym)
			}
		case SymColumn:
			md = renderColumn(sym)
		case SymEnum:
			md = renderEnum(sym)
		case SymEnumValue:
			md = renderEnumValue(sym)
		case SymTableGroup:
			md = fmt.Sprintf("**TableGroup** `%s`", sym.Name)
		case SymRefName:
			md = fmt.Sprintf("**Ref** `%s`", sym.Name)
		}
		if md != "" {
			return &HoverResult{Markdown: md, Range: sym.NameRange}
		}
	}

	// 3) Top-level keyword (Table, Enum, Ref, …).
	if tok.Kind == lexer.KindIdentifier {
		if isDeclKeywordContext(a, tok) {
			if kw := KeywordByName(tok.Value); kw != nil {
				return &HoverResult{Markdown: renderKeyword(kw), Range: tokenRange(*tok)}
			}
		}
	}

	// 4) Attribute name inside [ … ].
	if tok.Kind == lexer.KindIdentifier && a.inAttributeName(offset) {
		// Try multi-word match first ("not null", "primary key").
		name := a.attributeNameAt(offset)
		if attr := AttributeByName(name); attr != nil {
			return &HoverResult{Markdown: renderAttribute(attr), Range: a.attributeNameRange(offset)}
		}
	}

	// 5) Builtin type token in column type position.
	if tok.Kind == lexer.KindIdentifier && a.isColumnTypePosition(offset) {
		if bt := BuiltinTypeByName(tok.Value); bt != nil {
			return &HoverResult{Markdown: renderBuiltinType(bt, a), Range: tokenRange(*tok)}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Context probes (used by Hover and Completion)
// ---------------------------------------------------------------------------

// symbolAtDecl returns the declaration symbol whose NameRange contains offset.
func (a *Analysis) symbolAtDecl(offset int) *Symbol {
	in := func(r Range) bool { return offset >= r.Start && offset <= r.End }
	for _, t := range a.Symbols.AllTables {
		if in(t.NameRange) {
			return t
		}
		for _, c := range t.Columns {
			if in(c.NameRange) {
				return c
			}
		}
	}
	for _, e := range a.Symbols.AllEnums {
		if in(e.NameRange) {
			return e
		}
		for _, v := range e.EnumValues {
			if in(v.NameRange) {
				return v
			}
		}
	}
	for _, tg := range a.Symbols.TableGroups {
		if in(tg.NameRange) {
			return tg
		}
	}
	for _, r := range a.Symbols.RefNames {
		if in(r.NameRange) {
			return r
		}
	}
	for _, al := range a.Symbols.Aliases {
		if in(al.NameRange) {
			return al
		}
	}
	return nil
}

// isDeclKeywordContext reports whether tok is at the start of an
// ElementDeclNode (i.e. it's the type keyword like "Table").
func isDeclKeywordContext(a *Analysis, tok *lexer.Token) bool {
	for _, n := range a.Program.Body {
		decl, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		if decl.Type.Start == tok.Start {
			return true
		}
	}
	return false
}

// inAttributeName reports whether offset is inside the *name* portion of an
// AttributeNode in a settings list (i.e. before a colon, or there's no colon).
func (a *Analysis) inAttributeName(offset int) bool {
	for _, n := range a.Spans.Innermost(offset) {
		attr, ok := n.(*parser.AttributeNode)
		if !ok {
			continue
		}
		nameRange := nodeFullRange(attr.Name)
		if offset >= nameRange.Start && offset <= nameRange.End {
			return true
		}
	}
	return false
}

// attributeNameAt returns the full attribute name (possibly multi-word) at offset.
func (a *Analysis) attributeNameAt(offset int) string {
	for _, n := range a.Spans.Innermost(offset) {
		attr, ok := n.(*parser.AttributeNode)
		if !ok {
			continue
		}
		nameRange := nodeFullRange(attr.Name)
		if offset >= nameRange.Start && offset <= nameRange.End {
			s, _ := extractAttributeNameFull(attr.Name)
			return strings.ToLower(s)
		}
	}
	return ""
}

func (a *Analysis) attributeNameRange(offset int) Range {
	for _, n := range a.Spans.Innermost(offset) {
		attr, ok := n.(*parser.AttributeNode)
		if !ok {
			continue
		}
		nameRange := nodeFullRange(attr.Name)
		if offset >= nameRange.Start && offset <= nameRange.End {
			return nameRange
		}
	}
	return Range{}
}

// extractAttributeNameFull returns the full text of an attribute name node.
// For IdentStreamNode this is "primary key"; for a single ident it's the name.
func extractAttributeNameFull(n parser.Node) (string, Range) {
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return extractAttributeNameFull(node.Expr)
	case *parser.VariableNode:
		return node.Token.Value, tokenRange(node.Token)
	case *parser.IdentStreamNode:
		parts := make([]string, len(node.Tokens))
		for i, t := range node.Tokens {
			parts[i] = t.Value
		}
		return strings.Join(parts, " "),
			Range{node.Tokens[0].Start, node.Tokens[len(node.Tokens)-1].End}
	}
	return "", Range{}
}

// isColumnTypePosition reports whether offset is at the *type* position of a
// column field — the second token on the line inside a Table block.
func (a *Analysis) isColumnTypePosition(offset int) bool {
	for _, n := range a.Spans.Innermost(offset) {
		fa, ok := n.(*parser.FuncAppNode)
		if !ok {
			continue
		}
		if len(fa.Args) == 0 {
			continue
		}
		typeNode := fa.Args[0]
		r := nodeFullRange(typeNode)
		if offset >= r.Start && offset <= r.End {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Renderers
// ---------------------------------------------------------------------------

func renderTable(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Table** `%s`\n\n", sym.Name)
	if len(sym.Columns) > 0 {
		b.WriteString("| Column | Type | Constraints |\n| --- | --- | --- |\n")
		for _, c := range sym.Columns {
			t := c.TypeName
			if len(c.TypeArgs) > 0 {
				t += "(" + strings.Join(c.TypeArgs, ",") + ")"
			}
			var cs []string
			if c.PK {
				cs = append(cs, "pk")
			}
			if c.NotNull {
				cs = append(cs, "not null")
			}
			if c.Unique {
				cs = append(cs, "unique")
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", c.Name, t, strings.Join(cs, ", "))
		}
	}
	if sym.Note != "" {
		fmt.Fprintf(&b, "\n> %s\n", sym.Note)
	}
	return b.String()
}

func renderColumn(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	var b strings.Builder
	t := sym.TypeName
	if len(sym.TypeArgs) > 0 {
		t += "(" + strings.Join(sym.TypeArgs, ",") + ")"
	}
	fmt.Fprintf(&b, "`%s` — `%s`\n", sym.Qualified, t)
	if sym.PK {
		b.WriteString("\n- primary key")
	}
	if sym.NotNull {
		b.WriteString("\n- not null")
	}
	if sym.Unique {
		b.WriteString("\n- unique")
	}
	if sym.Note != "" {
		fmt.Fprintf(&b, "\n\n> %s", sym.Note)
	}
	return b.String()
}

func renderEnum(sym *Symbol) string {
	if sym == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**Enum** `%s`\n", sym.Name)
	for _, v := range sym.EnumValues {
		fmt.Fprintf(&b, "\n- `%s`", v.Name)
	}
	return b.String()
}

func renderEnumValue(sym *Symbol) string {
	parent := ""
	if sym.Parent != nil {
		parent = sym.Parent.Name + "."
	}
	return fmt.Sprintf("**Enum value** `%s%s`", parent, sym.Name)
}

func renderKeyword(kw *Keyword) string {
	return fmt.Sprintf("`%s` — %s", kw.Name, kw.Doc)
}

func renderAttribute(a *ColumnAttribute) string {
	return fmt.Sprintf("`%s` — %s", a.Name, a.Doc)
}

func renderBuiltinType(t *BuiltinType, a *Analysis) string {
	return fmt.Sprintf("**type** `%s` — %s", t.Name, t.Doc)
}
