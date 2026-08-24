package analysis

import (
	"strings"

	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

type RefSiteKind int

const (
	RefSiteTable RefSiteKind = iota
	RefSiteColumn
	RefSiteEnumType
	RefSiteTableInGroup
)

func (k RefSiteKind) String() string {
	switch k {
	case RefSiteTable:
		return "table-ref"
	case RefSiteColumn:
		return "column-ref"
	case RefSiteEnumType:
		return "enum-type"
	case RefSiteTableInGroup:
		return "table-in-group"
	}
	return "?"
}

// ResolvedRef is a single use site of a symbol, with its resolution attempt.
type ResolvedRef struct {
	Kind       RefSiteKind
	SiteRange  Range   // identifier(s) at the use site (just the name token)
	SourceText string  // verbatim text at the site
	Target     *Symbol // nil if unresolved
	// ParentTableHint is the bare table name from the site context (e.g. "users"
	// in "users.id"); used by completion to know which table's columns to offer.
	ParentTableHint string
}

// ResolveRefs walks all reference sites in the program and resolves them
// against the symbol index.
func ResolveRefs(prog *parser.ProgramNode, idx *SymbolIndex) []ResolvedRef {
	if prog == nil {
		return nil
	}
	var out []ResolvedRef
	for _, n := range prog.Body {
		decl, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		switch decl.Type.Value {
		case "Ref":
			out = collectRefDecl(out, decl, idx)
		case "Table":
			out = collectTableBody(out, decl, idx)
		case "TableGroup":
			out = collectTableGroupBody(out, decl, idx)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Ref declarations
// ---------------------------------------------------------------------------

func collectRefDecl(out []ResolvedRef, decl *parser.ElementDeclNode, idx *SymbolIndex) []ResolvedRef {
	if decl.Body == nil {
		return out
	}
	// Block form: each body item is its own ref expression. Both forms are
	// parsed by parseColonBody, so both carry a proper InfixExpr tree.
	if blk, ok := decl.Body.(*parser.BlockExprNode); ok {
		for _, item := range blk.Body {
			out = collectRefExpr(out, item, idx)
		}
		return out
	}
	return collectRefExpr(out, decl.Body, idx)
}

// collectRefExpr extracts ref sites from a structured (parseColonBody) ref
// expression — left InfixExpr(REL) right.
func collectRefExpr(out []ResolvedRef, n parser.Node, idx *SymbolIndex) []ResolvedRef {
	// May be wrapped in a FuncAppNode (e.g. with trailing settings).
	if fa, ok := n.(*parser.FuncAppNode); ok {
		return collectRefExpr(out, fa.Callee, idx)
	}
	infix, ok := n.(*parser.InfixExprNode)
	if !ok {
		return out
	}
	op := infix.Op.Value
	if op != ">" && op != "<" && op != "-" && op != "<>" {
		return out
	}
	out = collectEndpoint(out, infix.Left, idx)
	out = collectEndpoint(out, infix.Right, idx)
	return out
}

// ---------------------------------------------------------------------------
// Token-stream flattening (used by completion to inspect partial expressions)
// ---------------------------------------------------------------------------

// refAtom is a piece of a flat ref expression after flattening: either an
// identifier (variable) token, an operator token (`.`, `>`, …), or a tuple.
type refAtom struct {
	tok   lexer.Token // for ident/op
	tuple *parser.TupleExprNode
}

func (a refAtom) isOp(v string) bool {
	return a.tuple == nil && a.tok.Kind == lexer.KindOp && a.tok.Value == v
}
func (a refAtom) isIdent() bool {
	return a.tuple == nil &&
		(a.tok.Kind == lexer.KindIdentifier || a.tok.Kind == lexer.KindQuotedString)
}

func flatten(n parser.Node) []refAtom {
	var out []refAtom
	var rec func(parser.Node)
	rec = func(n parser.Node) {
		if n == nil {
			return
		}
		switch node := n.(type) {
		case *parser.FuncAppNode:
			rec(node.Callee)
			for _, a := range node.Args {
				rec(a)
			}
		case *parser.PrimaryExprNode:
			rec(node.Expr)
		case *parser.VariableNode:
			out = append(out, refAtom{tok: node.Token})
		case *parser.LiteralNode:
			// e.g. quoted strings used as identifiers
			out = append(out, refAtom{tok: node.Token})
		case *parser.TupleExprNode:
			out = append(out, refAtom{tuple: node})
		case *parser.InfixExprNode:
			rec(node.Left)
			out = append(out, refAtom{tok: node.Op})
			rec(node.Right)
		}
	}
	rec(n)
	return out
}

// collectEndpoint handles an endpoint which is either:
//   - tbl.col                (InfixExprNode .)
//   - schema.tbl.col         (nested InfixExprNode .)
//   - (a, b, c)              (TupleExprNode)  — composite (column-only, parent assumed elsewhere)
//   - tbl.(a, b)             (InfixExprNode . with TupleExprNode on the right)
func collectEndpoint(out []ResolvedRef, n parser.Node, idx *SymbolIndex) []ResolvedRef {
	if n == nil {
		return out
	}
	switch node := n.(type) {
	case *parser.InfixExprNode:
		if node.Op.Value != "." {
			return out
		}
		// Right side might be a tuple of column names: tbl.(a, b)
		if tup, ok := node.Right.(*parser.TupleExprNode); ok {
			tblName, tblRange := extractDeclName(node.Left)
			out = appendTableSite(out, tblName, tblRange, idx)
			tbl := idx.ResolveTable(tblName)
			for _, item := range tup.Items {
				colName, colRange := extractDeclName(item)
				out = append(out, ResolvedRef{
					Kind:            RefSiteColumn,
					SiteRange:       colRange,
					SourceText:      colName,
					Target:          colSymbol(tbl, colName, idx),
					ParentTableHint: tblName,
				})
			}
			return out
		}
		// Nested . — could be schema.table.column
		if inner, ok := node.Left.(*parser.InfixExprNode); ok && inner.Op.Value == "." {
			tblName, tblRange := extractDeclName(inner.Right)
			out = appendTableSite(out, tblName, tblRange, idx)
			colName, colRange := extractDeclName(node.Right)
			tbl := idx.ResolveTable(tblName)
			out = append(out, ResolvedRef{
				Kind:            RefSiteColumn,
				SiteRange:       colRange,
				SourceText:      colName,
				Target:          colSymbol(tbl, colName, idx),
				ParentTableHint: tblName,
			})
			return out
		}
		// Simple tbl.col
		tblName, tblRange := extractDeclName(node.Left)
		out = appendTableSite(out, tblName, tblRange, idx)
		colName, colRange := extractDeclName(node.Right)
		tbl := idx.ResolveTable(tblName)
		out = append(out, ResolvedRef{
			Kind:            RefSiteColumn,
			SiteRange:       colRange,
			SourceText:      colName,
			Target:          colSymbol(tbl, colName, idx),
			ParentTableHint: tblName,
		})
		return out
	case *parser.TupleExprNode:
		// Parenthesized table list (rare). Treat each as a table ref.
		for _, item := range node.Items {
			out = collectEndpoint(out, item, idx)
		}
		return out
	default:
		// Bare table name (no dot) — uncommon but possible.
		name, rng := extractDeclName(n)
		if name != "" {
			out = appendTableSite(out, name, rng, idx)
		}
	}
	return out
}

func appendTableSite(out []ResolvedRef, name string, rng Range, idx *SymbolIndex) []ResolvedRef {
	return append(out, ResolvedRef{
		Kind:       RefSiteTable,
		SiteRange:  rng,
		SourceText: name,
		Target:     idx.ResolveTable(name),
	})
}

func colSymbol(tbl *Symbol, colName string, idx *SymbolIndex) *Symbol {
	if tbl == nil {
		return nil
	}
	if m, ok := idx.Columns[key(tbl.Name)]; ok {
		return m[key(colName)]
	}
	return nil
}

// ---------------------------------------------------------------------------
// Inside a Table body: type-as-enum + inline refs
// ---------------------------------------------------------------------------

func collectTableBody(out []ResolvedRef, decl *parser.ElementDeclNode, idx *SymbolIndex) []ResolvedRef {
	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		return out
	}
	for _, item := range block.Body {
		fa, ok := item.(*parser.FuncAppNode)
		if !ok {
			continue
		}
		// Skip pseudo-fields.
		first := strings.ToLower(extractFirstName(fa.Callee))
		if first == "note" || first == "indexes" || first == "records" {
			continue
		}
		// Args[0] is the type expression. The type's leftmost identifier
		// might match an enum name.
		if len(fa.Args) == 0 {
			continue
		}
		typeNode := fa.Args[0]
		typeName, typeRange := typeIdentToken(typeNode)
		if typeName != "" {
			if enum, ok := idx.Enums[key(typeName)]; ok {
				out = append(out, ResolvedRef{
					Kind:       RefSiteEnumType,
					SiteRange:  typeRange,
					SourceText: typeName,
					Target:     enum,
				})
			}
		}
		// Inline refs in the settings list (Args[1+] when it's a ListExpr).
		for _, arg := range fa.Args[1:] {
			list, ok := arg.(*parser.ListExprNode)
			if !ok {
				continue
			}
			out = collectInlineRefs(out, list, idx)
		}
	}
	return out
}

// typeIdentToken returns the bare type name token (callee of FuncApp or the
// primary expr's variable) along with its source range.
func typeIdentToken(n parser.Node) (string, Range) {
	switch node := n.(type) {
	case *parser.FuncAppNode:
		return typeIdentToken(node.Callee)
	case *parser.PrimaryExprNode:
		return typeIdentToken(node.Expr)
	case *parser.VariableNode:
		return node.Token.Value, tokenRange(node.Token)
	case *parser.InfixExprNode:
		// schema.type — use the right side
		if node.Op.Value == "." {
			return typeIdentToken(node.Right)
		}
	}
	return "", Range{}
}

func collectInlineRefs(out []ResolvedRef, list *parser.ListExprNode, idx *SymbolIndex) []ResolvedRef {
	for _, item := range list.Items {
		attr, ok := item.(*parser.AttributeNode)
		if !ok {
			continue
		}
		if strings.ToLower(extractFirstName(attr.Name)) != "ref" {
			continue
		}
		if attr.Value == nil {
			continue
		}
		// Value form: `> table.col` (FuncAppNode with op callee)
		fa, ok := attr.Value.(*parser.FuncAppNode)
		if !ok {
			continue
		}
		if len(fa.Args) == 0 {
			continue
		}
		// fa.Args[0] is the target — could be table.col or schema.table.col.
		out = collectEndpoint(out, fa.Args[0], idx)
	}
	return out
}

// ---------------------------------------------------------------------------
// TableGroup body
// ---------------------------------------------------------------------------

func collectTableGroupBody(out []ResolvedRef, decl *parser.ElementDeclNode, idx *SymbolIndex) []ResolvedRef {
	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		return out
	}
	for _, item := range block.Body {
		name, rng := extractDeclName(item)
		if name == "" {
			continue
		}
		out = append(out, ResolvedRef{
			Kind:       RefSiteTableInGroup,
			SiteRange:  rng,
			SourceText: name,
			Target:     idx.ResolveTable(name),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// Lookup helpers
// ---------------------------------------------------------------------------

// RefAt returns the ResolvedRef whose SiteRange contains offset, if any.
func (a *Analysis) RefAt(offset int) *ResolvedRef {
	for i := range a.Refs {
		r := &a.Refs[i]
		if offset >= r.SiteRange.Start && offset <= r.SiteRange.End {
			return r
		}
	}
	return nil
}

// TokenAt returns the significant token whose [Start,End] contains offset.
func (a *Analysis) TokenAt(offset int) *lexer.Token {
	for i := range a.Tokens {
		t := &a.Tokens[i]
		if t.Kind == lexer.KindEOF {
			continue
		}
		if offset >= t.Start && offset <= t.End {
			return t
		}
	}
	return nil
}
