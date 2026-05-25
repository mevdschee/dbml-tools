package analysis

import (
	"dbml-tools/interpreter"
	"dbml-tools/lexer"
	"dbml-tools/parser"
	"strings"
)

type SymbolKind int

const (
	SymTable SymbolKind = iota
	SymColumn
	SymEnum
	SymEnumValue
	SymTableGroup
	SymAlias
	SymRefName
)

func (k SymbolKind) String() string {
	switch k {
	case SymTable:
		return "table"
	case SymColumn:
		return "column"
	case SymEnum:
		return "enum"
	case SymEnumValue:
		return "enum-value"
	case SymTableGroup:
		return "table-group"
	case SymAlias:
		return "alias"
	case SymRefName:
		return "ref-name"
	}
	return "?"
}

// Symbol describes a single declared entity.
type Symbol struct {
	Kind      SymbolKind
	Name      string  // original casing
	Qualified string  // schema.table or table.column or enum.value
	Parent    *Symbol // column → its table; enum value → its enum
	// DefRange is the full declaration extent (table + body, column + settings, …).
	DefRange Range
	// NameRange is just the identifier token at the declaration site.
	NameRange Range

	// Hover-facing metadata
	TypeName string // for columns
	TypeArgs []string
	PK       bool
	Unique   bool
	NotNull  bool
	Note     string

	// For enums: the value list
	EnumValues []*Symbol
	// For tables: ordered columns
	Columns []*Symbol
}

// SymbolIndex is the name-keyed dictionary for the whole file.
type SymbolIndex struct {
	Tables      map[string]*Symbol            // key: lowercased qualified name
	Columns     map[string]map[string]*Symbol // tableKey → colKey → symbol
	Enums       map[string]*Symbol
	EnumValues  map[string]map[string]*Symbol
	Aliases     map[string]*Symbol // alias key → underlying table symbol
	TableGroups map[string]*Symbol
	RefNames    map[string]*Symbol

	// AllTables / AllEnums preserve declaration order for stable iteration.
	AllTables []*Symbol
	AllEnums  []*Symbol
}

func newSymbolIndex() *SymbolIndex {
	return &SymbolIndex{
		Tables:      map[string]*Symbol{},
		Columns:     map[string]map[string]*Symbol{},
		Enums:       map[string]*Symbol{},
		EnumValues:  map[string]map[string]*Symbol{},
		Aliases:     map[string]*Symbol{},
		TableGroups: map[string]*Symbol{},
		RefNames:    map[string]*Symbol{},
	}
}

// Key returns the case-insensitive key for a name.
func key(s string) string { return strings.ToLower(s) }

// BuildSymbolIndex walks the AST and resolved interpreter output to populate
// the index.
func BuildSymbolIndex(prog *parser.ProgramNode, db *interpreter.Database) *SymbolIndex {
	idx := newSymbolIndex()
	if prog == nil {
		return idx
	}

	for _, n := range prog.Body {
		decl, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		switch decl.Type.Value {
		case "Table":
			idx.addTable(decl)
		case "Enum":
			idx.addEnum(decl)
		case "TableGroup":
			idx.addTableGroup(decl)
		case "Ref":
			idx.addRefName(decl)
		}
	}

	// Augment columns with type/note info from the interpreter pass.
	for _, t := range db.Tables {
		tbl := idx.Tables[key(t.Name)]
		if tbl == nil {
			continue
		}
		colMap := idx.Columns[key(t.Name)]
		for i, f := range t.Fields {
			sym := colMap[key(f.Name)]
			if sym == nil {
				continue
			}
			sym.TypeName = f.Type.TypeName
			if args, ok := f.Type.Args.([]string); ok {
				sym.TypeArgs = args
			}
			sym.PK = f.PK
			sym.Unique = f.Unique
			if f.NotNull != nil {
				sym.NotNull = *f.NotNull
			}
			if f.Note != nil {
				sym.Note = f.Note.Value
			}
			_ = i
		}
		if t.Note != nil {
			tbl.Note = t.Note.Value
		}
	}

	return idx
}

func (idx *SymbolIndex) addTable(decl *parser.ElementDeclNode) {
	if decl.Name == nil {
		return
	}
	name, nameRange := extractDeclName(decl.Name)
	if name == "" {
		return
	}
	sym := &Symbol{
		Kind:      SymTable,
		Name:      name,
		Qualified: name,
		DefRange:  nodeFullRange(decl),
		NameRange: nameRange,
	}
	idx.Tables[key(name)] = sym
	idx.AllTables = append(idx.AllTables, sym)
	idx.Columns[key(name)] = map[string]*Symbol{}

	// Alias
	if decl.Alias != nil {
		aname, arange := extractDeclName(decl.Alias)
		if aname != "" {
			asym := &Symbol{
				Kind:      SymAlias,
				Name:      aname,
				Qualified: aname,
				DefRange:  arange,
				NameRange: arange,
				Parent:    sym,
			}
			idx.Aliases[key(aname)] = asym
		}
	}

	// Columns inside the block body
	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		return
	}
	for _, item := range block.Body {
		fa, ok := item.(*parser.FuncAppNode)
		if !ok {
			continue
		}
		// Skip "Note", "indexes", "records" pseudo-fields
		calleeName := strings.ToLower(extractFirstName(fa.Callee))
		if calleeName == "note" || calleeName == "indexes" || calleeName == "records" {
			continue
		}
		colName, colRange := extractDeclName(fa.Callee)
		if colName == "" {
			continue
		}
		col := &Symbol{
			Kind:      SymColumn,
			Name:      colName,
			Qualified: name + "." + colName,
			Parent:    sym,
			DefRange:  nodeFullRange(fa),
			NameRange: colRange,
		}
		idx.Columns[key(name)][key(colName)] = col
		sym.Columns = append(sym.Columns, col)
	}
}

func (idx *SymbolIndex) addEnum(decl *parser.ElementDeclNode) {
	if decl.Name == nil {
		return
	}
	name, nameRange := extractDeclName(decl.Name)
	if name == "" {
		return
	}
	sym := &Symbol{
		Kind:      SymEnum,
		Name:      name,
		Qualified: name,
		DefRange:  nodeFullRange(decl),
		NameRange: nameRange,
	}
	idx.Enums[key(name)] = sym
	idx.AllEnums = append(idx.AllEnums, sym)
	idx.EnumValues[key(name)] = map[string]*Symbol{}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		return
	}
	for _, item := range block.Body {
		vname, vrange := extractDeclName(item)
		if vname == "" {
			continue
		}
		v := &Symbol{
			Kind:      SymEnumValue,
			Name:      vname,
			Qualified: name + "." + vname,
			Parent:    sym,
			DefRange:  vrange,
			NameRange: vrange,
		}
		idx.EnumValues[key(name)][key(vname)] = v
		sym.EnumValues = append(sym.EnumValues, v)
	}
}

func (idx *SymbolIndex) addTableGroup(decl *parser.ElementDeclNode) {
	if decl.Name == nil {
		return
	}
	name, nameRange := extractDeclName(decl.Name)
	if name == "" {
		return
	}
	sym := &Symbol{
		Kind:      SymTableGroup,
		Name:      name,
		Qualified: name,
		DefRange:  nodeFullRange(decl),
		NameRange: nameRange,
	}
	idx.TableGroups[key(name)] = sym
}

func (idx *SymbolIndex) addRefName(decl *parser.ElementDeclNode) {
	if decl.Name == nil {
		return
	}
	name, nameRange := extractDeclName(decl.Name)
	if name == "" {
		return
	}
	sym := &Symbol{
		Kind:      SymRefName,
		Name:      name,
		Qualified: name,
		DefRange:  nodeFullRange(decl),
		NameRange: nameRange,
	}
	idx.RefNames[key(name)] = sym
}

// ResolveTable looks up a table by name or alias.
func (idx *SymbolIndex) ResolveTable(name string) *Symbol {
	if t, ok := idx.Tables[key(name)]; ok {
		return t
	}
	if a, ok := idx.Aliases[key(name)]; ok {
		return a.Parent
	}
	return nil
}

// ResolveColumn looks up a column on a table (resolves aliases first).
func (idx *SymbolIndex) ResolveColumn(table, col string) *Symbol {
	t := idx.ResolveTable(table)
	if t == nil {
		return nil
	}
	if m, ok := idx.Columns[key(t.Name)]; ok {
		return m[key(col)]
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractDeclName returns the name of an identifier-like node and the range
// of just the identifier token at the *end* (so "schema.table" returns
// "table" and the range of "table").
func extractDeclName(n parser.Node) (string, Range) {
	if n == nil {
		return "", Range{}
	}
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return extractDeclName(node.Expr)
	case *parser.VariableNode:
		return node.Token.Value, tokenRange(node.Token)
	case *parser.LiteralNode:
		return node.Token.Value, tokenRange(node.Token)
	case *parser.InfixExprNode:
		if node.Op.Value == "." {
			return extractDeclName(node.Right)
		}
		return extractDeclName(node.Left)
	case *parser.FuncAppNode:
		return extractDeclName(node.Callee)
	case *parser.IdentStreamNode:
		if len(node.Tokens) == 0 {
			return "", Range{}
		}
		s := ""
		for i, t := range node.Tokens {
			if i > 0 {
				s += " "
			}
			s += t.Value
		}
		return s, Range{node.Tokens[0].Start, node.Tokens[len(node.Tokens)-1].End}
	}
	return "", Range{}
}

// extractFirstName returns the leftmost identifier (used to detect "note" /
// "indexes" pseudo-fields whose callee is the keyword).
func extractFirstName(n parser.Node) string {
	if n == nil {
		return ""
	}
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return extractFirstName(node.Expr)
	case *parser.VariableNode:
		return node.Token.Value
	case *parser.InfixExprNode:
		if node.Op.Value == "." {
			return extractFirstName(node.Left)
		}
		return extractFirstName(node.Left)
	case *parser.FuncAppNode:
		return extractFirstName(node.Callee)
	}
	return ""
}

func nodeFullRange(n parser.Node) Range {
	return Range{n.FirstToken().Start, n.LastToken().End}
}

// Token-level utility for callers that need a token's range.
func TokenRange(t lexer.Token) Range { return tokenRange(t) }
