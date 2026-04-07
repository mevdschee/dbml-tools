package interpreter

import (
	"dbml-tools/lexer"
	"dbml-tools/parser"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Database schema types (interpreter.json output)
// ---------------------------------------------------------------------------

type Database struct {
	Schemas       []interface{}  `json:"schemas"`
	Tables        []Table        `json:"tables"`
	Notes         []interface{}  `json:"notes"`
	Refs          []Ref          `json:"refs"`
	Enums         []Enum         `json:"enums"`
	TableGroups   []TableGroup   `json:"tableGroups"`
	Aliases       []interface{}  `json:"aliases"`
	Project       interface{}    `json:"project"`
	TablePartials []interface{}  `json:"tablePartials"`
	Records       []interface{}  `json:"records"`
}

type Table struct {
	Name       string        `json:"name"`
	SchemaName *string       `json:"schemaName"`
	Alias      *string       `json:"alias"`
	Fields     []Column      `json:"fields"`
	Token      TokenRange    `json:"token"`
	Indexes    []interface{} `json:"indexes"`
	Partials   []interface{} `json:"partials"`
	Checks     []interface{} `json:"checks"`
	Note       *Note         `json:"note,omitempty"`
}

type Column struct {
	Name       string        `json:"name"`
	Type       ColumnType    `json:"type"`
	Token      TokenRange    `json:"token"`
	InlineRefs []interface{} `json:"inline_refs"`
	PK         bool          `json:"pk"`
	Increment  *bool         `json:"increment,omitempty"`
	Unique     bool          `json:"unique"`
	NotNull    *bool         `json:"not_null,omitempty"`
	Note       *Note         `json:"note,omitempty"`
	DBDefault  *Default      `json:"dbdefault,omitempty"`
	Checks     *[]interface{} `json:"checks,omitempty"`
}

type ColumnType struct {
	SchemaName *string     `json:"schemaName"`
	TypeName   string      `json:"type_name"`
	Args       interface{} `json:"args"`
}

type TokenRange struct {
	Start TokenPos `json:"start"`
	End   TokenPos `json:"end"`
}

type TokenPos struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Note struct {
	Value string     `json:"value"`
	Token TokenRange `json:"token"`
}

type Default struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type Ref struct {
	Token      TokenRange    `json:"token"`
	Name       *string       `json:"name"`
	SchemaName *string       `json:"schemaName"`
	Endpoints  []RefEndpoint `json:"endpoints"`
}

type RefEndpoint struct {
	FieldNames []string   `json:"fieldNames"`
	TableName  string     `json:"tableName"`
	SchemaName *string    `json:"schemaName"`
	Relation   string     `json:"relation"`
	Token      TokenRange `json:"token"`
}

type Enum struct {
	Name       string      `json:"name"`
	SchemaName *string     `json:"schemaName"`
	Values     []EnumValue `json:"values"`
	Token      TokenRange  `json:"token"`
}

type EnumValue struct {
	Name string `json:"name"`
	Note *Note  `json:"note,omitempty"`
}

type TableGroup struct {
	Name   string   `json:"name"`
	Tables []string `json:"tables"`
	Token  TokenRange `json:"token"`
}

// ---------------------------------------------------------------------------
// Interpreter
// ---------------------------------------------------------------------------

type Interpreter struct {
	Errors []Error
}

type Error struct {
	Message  string
	Position lexer.Position
}

func (e Error) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Position.Line+1, e.Position.Column+1, e.Message)
}

func New() *Interpreter {
	return &Interpreter{}
}

func (interp *Interpreter) Interpret(prog *parser.ProgramNode) *Database {
	db := &Database{
		Schemas:       []interface{}{},
		Tables:        []Table{},
		Notes:         []interface{}{},
		Refs:          []Ref{},
		Enums:         []Enum{},
		TableGroups:   []TableGroup{},
		Aliases:       []interface{}{},
		Project:       map[string]interface{}{},
		TablePartials: []interface{}{},
		Records:       []interface{}{},
	}

	for _, n := range prog.Body {
		decl, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		switch decl.Type.Value {
		case "Table":
			interp.interpretTable(db, decl)
		case "Ref":
			interp.interpretRef(db, decl)
		case "Enum":
			interp.interpretEnum(db, decl)
		case "TableGroup":
			interp.interpretTableGroup(db, decl)
		case "Project":
			interp.interpretProject(db, decl)
		case "Note", "TablePartial":
			// recognised but not yet interpreted
		default:
			interp.Errors = append(interp.Errors, Error{
				Message:  fmt.Sprintf("unknown declaration type %q", decl.Type.Value),
				Position: decl.Type.StartPos,
			})
		}
	}

	return db
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretTable(db *Database, decl *parser.ElementDeclNode) {
	tbl := Table{
		SchemaName: nil,
		Alias:      nil,
		Indexes:    []interface{}{},
		Partials:   []interface{}{},
		Checks:     []interface{}{},
		Token:      nodeRange(decl),
	}

	tbl.Name = extractName(decl.Name)

	if decl.Alias != nil {
		a := extractName(decl.Alias)
		tbl.Alias = &a
	}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		db.Tables = append(db.Tables, tbl)
		return
	}

	for _, item := range block.Body {
		// Capture table-level note: 'value' before treating item as a column.
		if fa, ok := item.(*parser.FuncAppNode); ok {
			if strings.ToLower(extractName(fa.Callee)) == "note" && len(fa.Args) > 0 {
				v := extractStringValue(fa.Args[0])
				tbl.Note = &Note{Value: v, Token: nodeRange(fa)}
				continue
			}
		}
		col := interp.interpretField(item)
		if col != nil {
			tbl.Fields = append(tbl.Fields, *col)
		}
	}

	if tbl.Fields == nil {
		tbl.Fields = []Column{}
	}

	db.Tables = append(db.Tables, tbl)
}

func (interp *Interpreter) interpretField(n parser.Node) *Column {
	col := &Column{
		InlineRefs: []interface{}{},
	}

	switch node := n.(type) {
	case *parser.FuncAppNode:
		name := extractName(node.Callee)
		if !isColumnName(name) {
			return nil
		}
		col.Name = name
		col.Token = nodeRange(node)

		if len(node.Args) == 0 {
			interp.Errors = append(interp.Errors, Error{
				Message:  fmt.Sprintf("column %q is missing a type", col.Name),
				Position: node.FirstToken().StartPos,
			})
			return col
		}
		col.Type = extractType(node.Args[0])

		// process settings [...]
		for _, arg := range node.Args[1:] {
			if list, ok := arg.(*parser.ListExprNode); ok {
				interp.applyColumnSettings(col, list)
			}
		}

	case *parser.PrimaryExprNode:
		name := extractName(node)
		if !isColumnName(name) {
			return nil
		}
		col.Name = name
		col.Token = nodeRange(node)
		interp.Errors = append(interp.Errors, Error{
			Message:  fmt.Sprintf("column %q is missing a type", col.Name),
			Position: node.FirstToken().StartPos,
		})

	default:
		return nil
	}

	return col
}

// isColumnName reports whether s looks like a valid column name rather than a
// DBML keyword or a stray punctuation token leaking out of an indexes block.
func isColumnName(s string) bool {
	if s == "" || s == "indexes" || s == "Note" || s == "note" {
		return false
	}
	// Reject bare punctuation tokens produced when the parser walks into an
	// indexes { } block without special handling: (, ), {, }, etc.
	switch s[0] {
	case '(', ')', '{', '}', '[', ']', '<', '>', '.', ',', ';', ':', '!', '~':
		return false
	}
	return true
}

func (interp *Interpreter) applyColumnSettings(col *Column, list *parser.ListExprNode) {
	hasPKSetting := false
	for _, item := range list.Items {
		attr, ok := item.(*parser.AttributeNode)
		if !ok {
			continue
		}

		name := extractAttrName(attr)
		switch name {
		case "pk":
			col.PK = true
			hasPKSetting = true
		case "primary key":
			col.PK = true
			hasPKSetting = true
		case "not null":
			nn := true
			col.NotNull = &nn
		case "null":
			nn := false
			col.NotNull = &nn
		case "unique":
			col.Unique = true
		case "increment":
			inc := true
			col.Increment = &inc
		case "note":
			if attr.Value != nil {
				v := extractStringValue(attr.Value)
				col.Note = &Note{
					Value: v,
					Token: nodeRange(attr),
				}
			}
		case "default":
			if attr.Value != nil {
				col.DBDefault = extractDefault(attr.Value)
			}
		}
	}

	if hasPKSetting {
		if col.Increment == nil {
			inc := false
			col.Increment = &inc
		}
		checks := []interface{}{}
		col.Checks = &checks
	}

	if col.Note != nil {
		if col.Increment == nil {
			inc := false
			col.Increment = &inc
		}
		checks := []interface{}{}
		col.Checks = &checks
	}
}

// ---------------------------------------------------------------------------
// Ref
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretRef(db *Database, decl *parser.ElementDeclNode) {
	// Handle block-style Ref { ... }
	if block, ok := decl.Body.(*parser.BlockExprNode); ok {
		for _, item := range block.Body {
			ref := interp.extractRefFromExpr(item, decl)
			if ref != nil {
				db.Refs = append(db.Refs, *ref)
			}
		}
		return
	}

	// Colon-style Ref: a.b > c.d
	if decl.Body != nil {
		ref := interp.extractRefFromExpr(decl.Body, decl)
		if ref != nil {
			if decl.Name != nil {
				name := extractName(decl.Name)
				ref.Name = &name
			}
			db.Refs = append(db.Refs, *ref)
		}
	}
}

func (interp *Interpreter) extractRefFromExpr(expr parser.Node, decl *parser.ElementDeclNode) *Ref {
	infix, ok := expr.(*parser.InfixExprNode)
	if !ok {
		// Try FuncAppNode that wraps an infix
		if fa, ok := expr.(*parser.FuncAppNode); ok {
			return interp.extractRefFromExpr(fa.Callee, decl)
		}
		return nil
	}

	op := infix.Op.Value
	var leftRel, rightRel string
	switch op {
	case ">":
		leftRel = "*"
		rightRel = "1"
	case "<":
		leftRel = "1"
		rightRel = "*"
	case "-":
		leftRel = "1"
		rightRel = "1"
	case "<>":
		leftRel = "*"
		rightRel = "*"
	default:
		return nil
	}

	leftEP := extractEndpoint(infix.Left, leftRel)
	rightEP := extractEndpoint(infix.Right, rightRel)

	return &Ref{
		Token:      nodeRange(decl),
		Name:       nil,
		SchemaName: nil,
		Endpoints:  []RefEndpoint{leftEP, rightEP},
	}
}

func extractEndpoint(n parser.Node, relation string) RefEndpoint {
	ep := RefEndpoint{
		Relation:   relation,
		SchemaName: nil,
		Token:      nodeRange(n),
	}

	// expecting table.column (InfixExprNode with .)
	infix, ok := n.(*parser.InfixExprNode)
	if ok && infix.Op.Value == "." {
		// could be schema.table.column or table.column
		if inner, ok := infix.Left.(*parser.InfixExprNode); ok && inner.Op.Value == "." {
			// schema.table.column
			schema := extractName(inner.Left)
			ep.SchemaName = &schema
			ep.TableName = extractName(inner.Right)
		} else {
			ep.TableName = extractName(infix.Left)
		}
		ep.FieldNames = []string{extractName(infix.Right)}
	} else {
		ep.TableName = extractName(n)
		ep.FieldNames = []string{}
	}

	return ep
}

// ---------------------------------------------------------------------------
// Enum
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretEnum(db *Database, decl *parser.ElementDeclNode) {
	e := Enum{
		Name:       extractName(decl.Name),
		SchemaName: nil,
		Token:      nodeRange(decl),
	}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if ok {
		for _, item := range block.Body {
			name := extractName(item)
			if name != "" {
				e.Values = append(e.Values, EnumValue{Name: name})
			}
		}
	}

	if e.Values == nil {
		e.Values = []EnumValue{}
	}
	db.Enums = append(db.Enums, e)
}

// ---------------------------------------------------------------------------
// TableGroup
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretTableGroup(db *Database, decl *parser.ElementDeclNode) {
	tg := TableGroup{
		Name:  extractName(decl.Name),
		Token: nodeRange(decl),
	}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if ok {
		for _, item := range block.Body {
			name := extractName(item)
			if name != "" {
				tg.Tables = append(tg.Tables, name)
			}
		}
	}
	if tg.Tables == nil {
		tg.Tables = []string{}
	}

	db.TableGroups = append(db.TableGroups, tg)
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretProject(db *Database, decl *parser.ElementDeclNode) {
	proj := map[string]interface{}{}
	if decl.Name != nil {
		proj["name"] = extractName(decl.Name)
	}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if ok {
		for _, item := range block.Body {
			if fa, ok := item.(*parser.FuncAppNode); ok {
				key := strings.ToLower(extractName(fa.Callee))
				if len(fa.Args) > 0 {
					val := extractStringValue(fa.Args[0])
					switch key {
					case "database_type":
						proj["databaseType"] = val
					case "note":
						proj["note"] = val
					}
				}
			}
		}
	}

	db.Project = proj
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractName(n parser.Node) string {
	if n == nil {
		return ""
	}
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return extractName(node.Expr)
	case *parser.VariableNode:
		return node.Token.Value
	case *parser.LiteralNode:
		return node.Token.Value
	case *parser.InfixExprNode:
		if node.Op.Value == "." {
			return extractName(node.Left) + "." + extractName(node.Right)
		}
		return extractName(node.Left)
	case *parser.FuncAppNode:
		return extractName(node.Callee)
	case *parser.IdentStreamNode:
		s := ""
		for i, t := range node.Tokens {
			if i > 0 {
				s += " "
			}
			s += t.Value
		}
		return s
	}
	return ""
}

func extractType(n parser.Node) ColumnType {
	ct := ColumnType{SchemaName: nil, Args: nil}
	if fa, ok := n.(*parser.FuncAppNode); ok {
		ct.TypeName = extractName(fa.Callee)
		if len(fa.Args) > 0 {
			args := make([]string, len(fa.Args))
			for i, arg := range fa.Args {
				args[i] = extractStringValue(arg)
			}
			ct.Args = args
		}
	} else {
		ct.TypeName = extractName(n)
	}
	return ct
}

func extractStringValue(n parser.Node) string {
	if n == nil {
		return ""
	}
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return extractStringValue(node.Expr)
	case *parser.LiteralNode:
		return node.Token.Value
	case *parser.VariableNode:
		return node.Token.Value
	}
	return ""
}

func extractAttrName(attr *parser.AttributeNode) string {
	return extractName(attr.Name)
}

func extractDefault(n parser.Node) *Default {
	v := extractStringValue(n)
	d := &Default{Value: v}
	// determine type
	switch n.(type) {
	case *parser.PrimaryExprNode:
		inner := n.(*parser.PrimaryExprNode).Expr
		if lit, ok := inner.(*parser.LiteralNode); ok {
			switch lit.Token.Kind {
			case lexer.KindNumeric:
				d.Type = "number"
			case lexer.KindString:
				d.Type = "string"
			case lexer.KindFuncExpr:
				d.Type = "expression"
			default:
				d.Type = "string"
			}
		} else {
			// identifier like true/false/null
			switch v {
			case "true", "false":
				d.Type = "boolean"
			case "null":
				d.Type = "boolean"
			default:
				d.Type = "string"
			}
		}
	default:
		d.Type = "string"
	}
	return d
}

func nodeRange(n parser.Node) TokenRange {
	first := n.FirstToken()
	last := n.LastToken()
	return TokenRange{
		Start: TokenPos{
			Offset: first.Start,
			Line:   first.StartPos.Line + 1,
			Column: first.StartPos.Column + 1,
		},
		End: TokenPos{
			Offset: last.End,
			Line:   last.EndPos.Line + 1,
			Column: last.EndPos.Column + 1,
		},
	}
}
