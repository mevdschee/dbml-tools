package interpreter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

// ---------------------------------------------------------------------------
// Database schema types (interpreter.json output)
// ---------------------------------------------------------------------------

type Database struct {
	Schemas       []interface{} `json:"schemas"`
	Tables        []Table       `json:"tables"`
	Notes         []interface{} `json:"notes"`
	Refs          []Ref         `json:"refs"`
	Enums         []Enum        `json:"enums"`
	TableGroups   []TableGroup  `json:"tableGroups"`
	Aliases       []interface{} `json:"aliases"`
	Project       interface{}   `json:"project"`
	TablePartials []interface{} `json:"tablePartials"`
	Records       []interface{} `json:"records"`
}

// TableRecords holds parsed records data for a table.
type TableRecords struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// Example marks sample data ([example]), which is not exported as
	// INSERT statements.
	Example bool `json:"example,omitempty"`

	rowNodes []*parser.RecordRowNode // for error positions
}

type Table struct {
	Name       string        `json:"name"`
	SchemaName *string       `json:"schemaName"`
	Alias      *string       `json:"alias"`
	Fields     []Column      `json:"fields"`
	Token      TokenRange    `json:"token"`
	Indexes    []Index       `json:"indexes"`
	Partials   []interface{} `json:"partials"`
	Checks     []interface{} `json:"checks"`
	Note       *Note         `json:"note,omitempty"`
	Records    *TableRecords `json:"records,omitempty"`
}

// Index is one entry of a table's `indexes { }` block.
type Index struct {
	Columns []IndexColumn `json:"columns"`
	Name    *string       `json:"name,omitempty"`
	Type    *string       `json:"type,omitempty"`
	Unique  bool          `json:"unique,omitempty"`
	PK      bool          `json:"pk,omitempty"`
	Note    *Note         `json:"note,omitempty"`
	Token   TokenRange    `json:"token"`
}

// IndexColumn is a column name (Type "column") or a backtick expression
// (Type "expression") in an index.
type IndexColumn struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

type Column struct {
	Name       string         `json:"name"`
	Type       ColumnType     `json:"type"`
	Token      TokenRange     `json:"token"`
	InlineRefs []interface{}  `json:"inline_refs"`
	PK         bool           `json:"pk"`
	Increment  *bool          `json:"increment,omitempty"`
	Unique     bool           `json:"unique"`
	NotNull    *bool          `json:"not_null,omitempty"`
	Note       *Note          `json:"note,omitempty"`
	DBDefault  *Default       `json:"dbdefault,omitempty"`
	Checks     *[]interface{} `json:"checks,omitempty"`
}

type ColumnType struct {
	SchemaName *string     `json:"schemaName"`
	TypeName   string      `json:"type_name"`
	Args       interface{} `json:"args"`
	// Suffix holds the words after the arguments, as in bigint(20) unsigned.
	Suffix string `json:"suffix,omitempty"`
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
	OnDelete   *string       `json:"onDelete,omitempty"`
	OnUpdate   *string       `json:"onUpdate,omitempty"`
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
	Name   string     `json:"name"`
	Tables []string   `json:"tables"`
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

	var records []*parser.ElementDeclNode
	for _, n := range prog.Body {
		decl, ok := n.(*parser.ElementDeclNode)
		if !ok {
			continue
		}
		if parser.IsRecordsKeyword(decl.Type.Value) {
			// Attached once all tables are known: Records may come first.
			records = append(records, decl)
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
	for _, decl := range records {
		interp.interpretRecordsDecl(db, decl)
	}

	return db
}

// interpretRecordsDecl attaches a top-level `Records tbl(a, b) { ... }` to
// its table, as if it had been written inside the table.
func (interp *Interpreter) interpretRecordsDecl(db *Database, decl *parser.ElementDeclNode) {
	blk, ok := decl.Body.(*parser.BlockExprNode)
	if decl.Name == nil || decl.Columns == nil || !ok {
		return // the parser has reported what is missing
	}
	schema, name := splitQualifiedName(decl.Name)
	var tbl *Table
	for i := range db.Tables {
		t := &db.Tables[i]
		if schema != nil && (t.SchemaName == nil || *t.SchemaName != *schema) {
			continue
		}
		if t.Name == name || (schema == nil && t.Alias != nil && *t.Alias == name) {
			tbl = t
			break
		}
	}
	if tbl == nil {
		interp.Errors = append(interp.Errors, Error{
			Message:  fmt.Sprintf("Records refers to unknown table %q", extractName(decl.Name)),
			Position: decl.Name.FirstToken().StartPos,
		})
		return
	}
	if tbl.Records != nil {
		interp.Errors = append(interp.Errors, Error{
			Message:  fmt.Sprintf("table %q already has records", tbl.Name),
			Position: decl.Type.StartPos,
		})
		return
	}
	cols := make([]string, len(decl.Columns.Items))
	for i, item := range decl.Columns.Items {
		cols[i] = extractName(item)
	}
	tbl.Records = interp.interpretRecords(cols, blk)
	tbl.Records.Example = hasExampleSetting(decl.Attrs)
}

// ---------------------------------------------------------------------------
// Table
// ---------------------------------------------------------------------------

func (interp *Interpreter) interpretTable(db *Database, decl *parser.ElementDeclNode) {
	tbl := Table{
		Alias:    nil,
		Indexes:  []Index{},
		Partials: []interface{}{},
		Checks:   []interface{}{},
		Token:    nodeRange(decl),
	}

	tbl.SchemaName, tbl.Name = splitQualifiedName(decl.Name)

	if decl.Alias != nil {
		a := extractName(decl.Alias)
		tbl.Alias = &a
	}

	block, ok := decl.Body.(*parser.BlockExprNode)
	if !ok {
		db.Tables = append(db.Tables, tbl)
		return
	}

	var recordsCols []string
	var recordsHead parser.Node
	recordsExample := false
	awaitRecordsBlock := false
	awaitIndexesBlock := false
	for _, item := range block.Body {
		// After seeing "indexes", the next item is the BlockExpr with the indexes.
		if awaitIndexesBlock {
			awaitIndexesBlock = false
			if blk, ok := item.(*parser.BlockExprNode); ok {
				for _, entry := range blk.Body {
					idx, ok := interpretIndex(entry)
					if !ok {
						continue
					}
					if idx.PK && slices.ContainsFunc(idx.Columns, func(c IndexColumn) bool {
						return c.Type == "expression"
					}) {
						interp.Errors = append(interp.Errors, Error{
							Message:  "a primary key cannot contain an expression",
							Position: entry.FirstToken().StartPos,
						})
					}
					tbl.Indexes = append(tbl.Indexes, idx)
				}
				continue
			}
		}
		if pe, ok := item.(*parser.PrimaryExprNode); ok && extractName(pe) == "indexes" {
			awaitIndexesBlock = true
			continue
		}
		// After seeing "records (...)", the next item is the BlockExpr with data rows.
		if awaitRecordsBlock {
			awaitRecordsBlock = false
			if blk, ok := item.(*parser.BlockExprNode); ok {
				if tbl.Records != nil {
					interp.Errors = append(interp.Errors, Error{
						Message:  fmt.Sprintf("table %q already has records", tbl.Name),
						Position: recordsHead.FirstToken().StartPos,
					})
					continue
				}
				// Without a column list the rows hold every column in
				// table order; the columns are filled in below.
				tbl.Records = interp.interpretRecords(recordsCols, blk)
				tbl.Records.Example = recordsExample
				continue
			}
			// No block found — not a records block after all, fall through.
		}
		if pe, ok := item.(*parser.PrimaryExprNode); ok && parser.IsRecordsKeyword(extractName(pe)) {
			recordsCols = nil
			recordsExample = false
			recordsHead = item
			awaitRecordsBlock = true
			continue
		}
		if fa, ok := item.(*parser.FuncAppNode); ok {
			callee := strings.ToLower(extractName(fa.Callee))
			// Capture table-level note: 'value' before treating item as a column.
			if callee == "note" && len(fa.Args) > 0 {
				v := extractKeyValue(fa.Args)
				tbl.Note = &Note{Value: v, Token: nodeRange(fa)}
				continue
			}
			if callee == "records" {
				recordsCols = extractRecordsCols(fa)
				recordsExample = false
				for _, arg := range fa.Args {
					recordsExample = recordsExample || hasExampleSetting(arg)
				}
				recordsHead = item
				awaitRecordsBlock = true
				continue
			}
		}
		col := interp.interpretField(item)
		if col != nil {
			tbl.Fields = append(tbl.Fields, *col)
			db.Refs = append(db.Refs, inlineRefs(tbl, *col)...)
		}
	}

	// The columns of a [pk] index are the table's primary key, as if marked
	// [pk] themselves. The block may come before the columns, so this is
	// done last.
	for _, idx := range tbl.Indexes {
		if !idx.PK {
			continue
		}
		for _, c := range idx.Columns {
			for i := range tbl.Fields {
				if c.Type == "column" && tbl.Fields[i].Name == c.Value {
					tbl.Fields[i].PK = true
				}
			}
		}
	}

	if tbl.Fields == nil {
		tbl.Fields = []Column{}
	}

	if tbl.Records != nil && tbl.Records.Columns == nil {
		tbl.Records.Columns = make([]string, len(tbl.Fields))
		for i, f := range tbl.Fields {
			tbl.Records.Columns[i] = f.Name
		}
		interp.checkRecordWidths(tbl.Records, recordsHead)
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

		// A schema-qualified type (`my_schema.my_type`) parses as three
		// separate args: the schema, the dot and the type name.
		rest := node.Args[1:]
		if len(rest) >= 2 && extractName(rest[0]) == "." {
			schema := col.Type.TypeName
			col.Type = extractType(rest[1])
			col.Type.SchemaName = &schema
			rest = rest[2:]
		}

		// process remaining args: type args (tuple), more words of the type
		// and settings (list)
		for _, arg := range rest {
			if word, ok := typeWord(arg); ok {
				// A word before the arguments belongs to the name, as in
				// double precision; after them it is a suffix, as in
				// bigint(20) unsigned.
				if col.Type.Args == nil {
					col.Type.TypeName += " " + word
				} else if col.Type.Suffix == "" {
					col.Type.Suffix = word
				} else {
					col.Type.Suffix += " " + word
				}
			} else if list, ok := arg.(*parser.ListExprNode); ok {
				interp.applyColumnSettings(col, list)
			} else if tuple, ok := arg.(*parser.TupleExprNode); ok {
				// Type arguments, e.g. (255) in varchar(255) or (10,2) in decimal(10,2)
				args := make([]string, len(tuple.Items))
				for i, item := range tuple.Items {
					args[i] = extractStringValue(item)
				}
				col.Type.Args = args
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

// typeWord returns the bare identifier n holds, as the parser leaves the
// further words of a multi-word type such as double precision.
func typeWord(n parser.Node) (string, bool) {
	pe, ok := n.(*parser.PrimaryExprNode)
	if !ok {
		return "", false
	}
	v, ok := pe.Expr.(*parser.VariableNode)
	if !ok || v.Token.Kind != lexer.KindIdentifier {
		return "", false
	}
	return v.Token.Value, true
}

// isColumnName reports whether s looks like a valid column name rather than a
// DBML keyword or a stray punctuation token leaking out of an indexes block.
func isColumnName(s string) bool {
	if s == "" || s == "indexes" || s == "records" || s == "Note" || s == "note" {
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
		case "ref":
			if ep, ok := extractInlineRef(attr); ok {
				col.InlineRefs = append(col.InlineRefs, ep)
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

// extractInlineRef reads a column setting such as `ref: > users.id`, which
// parses as the relation operator applied to the target endpoint. As in the
// upstream DBML JSON, the endpoint's Relation holds the operator itself.
func extractInlineRef(attr *parser.AttributeNode) (RefEndpoint, bool) {
	fa, ok := attr.Value.(*parser.FuncAppNode)
	if !ok || len(fa.Args) != 1 {
		return RefEndpoint{}, false
	}
	op := extractName(fa.Callee)
	if _, _, ok := refRelations(op); !ok {
		return RefEndpoint{}, false
	}
	ep := extractEndpoint(fa.Args[0], op)
	if ep.TableName == "" || len(ep.FieldNames) == 0 {
		return RefEndpoint{}, false
	}
	ep.Token = nodeRange(attr)
	return ep, true
}

// inlineRefs turns the inline refs of a column into Refs from that column to
// each target, so they are handled like standalone Ref declarations.
func inlineRefs(tbl Table, col Column) []Ref {
	var refs []Ref
	for _, ir := range col.InlineRefs {
		target, ok := ir.(RefEndpoint)
		if !ok {
			continue
		}
		colRel, targetRel, ok := refRelations(target.Relation)
		if !ok {
			continue
		}
		source := RefEndpoint{
			FieldNames: []string{col.Name},
			TableName:  tbl.Name,
			SchemaName: tbl.SchemaName,
			Relation:   colRel,
			Token:      col.Token,
		}
		target.Relation = targetRel
		refs = append(refs, Ref{
			Token:     target.Token,
			Endpoints: []RefEndpoint{source, target},
		})
	}
	return refs
}

// refRelations maps a relation operator to the relation of its left and
// right endpoints.
func refRelations(op string) (left, right string, ok bool) {
	switch op {
	case ">":
		return "*", "1", true
	case "<":
		return "1", "*", true
	case "-":
		return "1", "1", true
	case "<>":
		return "*", "*", true
	}
	return "", "", false
}

// interpretIndex reads one entry of an indexes block: a column, a backtick
// expression or a tuple of those, optionally followed by [settings].
func interpretIndex(n parser.Node) (Index, bool) {
	idx := Index{Token: nodeRange(n)}
	target := n
	var settings []parser.Node
	if fa, ok := n.(*parser.FuncAppNode); ok {
		target = fa.Callee
		settings = fa.Args
	}

	if tup, ok := target.(*parser.TupleExprNode); ok {
		for _, item := range tup.Items {
			if col, ok := indexColumn(item); ok {
				idx.Columns = append(idx.Columns, col)
			}
		}
	} else if col, ok := indexColumn(target); ok {
		idx.Columns = append(idx.Columns, col)
	}
	if len(idx.Columns) == 0 {
		return Index{}, false
	}

	for _, arg := range settings {
		list, ok := arg.(*parser.ListExprNode)
		if !ok {
			continue
		}
		for _, item := range list.Items {
			attr, ok := item.(*parser.AttributeNode)
			if !ok {
				continue
			}
			switch strings.ToLower(extractAttrName(attr)) {
			case "unique":
				idx.Unique = true
			case "pk", "primary key":
				idx.PK = true
			case "name":
				if attr.Value != nil {
					v := extractStringValue(attr.Value)
					idx.Name = &v
				}
			case "type":
				if attr.Value != nil {
					v := strings.ToLower(extractStringValue(attr.Value))
					idx.Type = &v
				}
			case "note":
				if attr.Value != nil {
					idx.Note = &Note{Value: extractStringValue(attr.Value), Token: nodeRange(attr)}
				}
			}
		}
	}
	return idx, true
}

// indexColumn reads a single index column: a backtick literal is an
// expression, anything else names a column.
func indexColumn(n parser.Node) (IndexColumn, bool) {
	if pe, ok := n.(*parser.PrimaryExprNode); ok {
		if lit, ok := pe.Expr.(*parser.LiteralNode); ok && lit.Token.Kind == lexer.KindFuncExpr {
			return IndexColumn{Value: lit.Token.Value, Type: "expression"}, true
		}
	}
	name := extractName(n)
	if name == "" {
		return IndexColumn{}, false
	}
	return IndexColumn{Value: name, Type: "column"}, true
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
		// Try FuncAppNode that wraps an infix, carrying the [...] settings
		if fa, ok := expr.(*parser.FuncAppNode); ok {
			ref := interp.extractRefFromExpr(fa.Callee, decl)
			if ref != nil {
				applyRefSettings(ref, fa.Args)
			}
			return ref
		}
		return nil
	}

	leftRel, rightRel, ok := refRelations(infix.Op.Value)
	if !ok {
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

// applyRefSettings reads the `delete:` and `update:` referential actions from
// the [...] settings list of a Ref declaration.
func applyRefSettings(ref *Ref, args []parser.Node) {
	for _, arg := range args {
		list, ok := arg.(*parser.ListExprNode)
		if !ok {
			continue
		}
		for _, item := range list.Items {
			attr, ok := item.(*parser.AttributeNode)
			if !ok || attr.Value == nil {
				continue
			}
			action := normaliseRefAction(extractName(attr.Value))
			if action == "" {
				continue
			}
			switch strings.ToLower(extractAttrName(attr)) {
			case "delete":
				ref.OnDelete = &action
			case "update":
				ref.OnUpdate = &action
			}
		}
	}
}

// normaliseRefAction canonicalises a referential action to the lowercase,
// single-spaced DBML spelling (e.g. `SET  NULL` -> `set null`).
func normaliseRefAction(v string) string {
	return strings.ToLower(strings.Join(strings.Fields(v), " "))
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
		if tup, ok := infix.Right.(*parser.TupleExprNode); ok {
			// composite endpoint: table.(col1, col2)
			ep.FieldNames = make([]string, 0, len(tup.Items))
			for _, item := range tup.Items {
				ep.FieldNames = append(ep.FieldNames, extractName(item))
			}
		} else {
			ep.FieldNames = []string{extractName(infix.Right)}
		}
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
		Token: nodeRange(decl),
	}
	e.SchemaName, e.Name = splitQualifiedName(decl.Name)

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
					val := extractKeyValue(fa.Args)
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

// splitQualifiedName splits a declaration name into its optional schema part
// and the object name: `my_schema.my_table` yields ("my_schema", "my_table").
// A quoted name containing a dot is a single identifier and keeps its schema
// unset.
func splitQualifiedName(n parser.Node) (*string, string) {
	switch node := n.(type) {
	case *parser.PrimaryExprNode:
		return splitQualifiedName(node.Expr)
	case *parser.InfixExprNode:
		if node.Op.Value == "." {
			schema := extractName(node.Left)
			name := extractName(node.Right)
			if schema != "" && name != "" {
				return &schema, name
			}
		}
	}
	return nil, extractName(n)
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

// extractKeyValue returns the value from a FuncAppNode's Args, skipping
// a leading colon separator if present (e.g. `key: 'value'` parses as
// Args[0]=colon, Args[1]=value).
func extractKeyValue(args []parser.Node) string {
	if len(args) == 0 {
		return ""
	}
	// Check if first arg is just a colon separator
	if len(args) >= 2 && extractStringValue(args[0]) == ":" {
		return extractStringValue(args[1])
	}
	return extractStringValue(args[0])
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

// hasExampleSetting reports whether n is a settings list holding
// [example], which marks records as sample data.
func hasExampleSetting(n parser.Node) bool {
	list, ok := n.(*parser.ListExprNode)
	if !ok {
		return false
	}
	for _, item := range list.Items {
		if attr, ok := item.(*parser.AttributeNode); ok && strings.EqualFold(extractAttrName(attr), "example") {
			return true
		}
	}
	return false
}

// extractRecordsCols extracts column names from a records FuncApp, e.g.
// records ("id", "name") → ["id", "name"].
func extractRecordsCols(fa *parser.FuncAppNode) []string {
	for _, arg := range fa.Args {
		if tuple, ok := arg.(*parser.TupleExprNode); ok {
			cols := make([]string, len(tuple.Items))
			for i, item := range tuple.Items {
				cols[i] = extractName(item)
			}
			return cols
		}
	}
	return nil
}

// interpretRecords extracts row data from a records block, whose items are
// the parser's RecordRowNodes. An empty field is stored as null. When cols
// is nil (records inside a table without a column list) the row widths are
// checked once the table's columns are known.
func (interp *Interpreter) interpretRecords(cols []string, blk *parser.BlockExprNode) *TableRecords {
	rec := &TableRecords{Columns: cols}
	for _, item := range blk.Body {
		row, ok := item.(*parser.RecordRowNode)
		if !ok {
			continue
		}
		vals := make([]string, len(row.Values))
		for i, v := range row.Values {
			vals[i] = recordValue(v)
		}
		rec.Rows = append(rec.Rows, vals)
		rec.rowNodes = append(rec.rowNodes, row)
	}
	if cols != nil {
		interp.checkRecordWidths(rec, blk)
	}
	return rec
}

// checkRecordWidths reports rows whose value count differs from the column
// count.
func (interp *Interpreter) checkRecordWidths(rec *TableRecords, at parser.Node) {
	for i, row := range rec.Rows {
		if len(row) == len(rec.Columns) {
			continue
		}
		pos := at.FirstToken().StartPos
		if i < len(rec.rowNodes) {
			pos = rec.rowNodes[i].FirstToken().StartPos
		}
		interp.Errors = append(interp.Errors, Error{
			Message:  fmt.Sprintf("records row has %d values, expected %d", len(row), len(rec.Columns)),
			Position: pos,
		})
	}
}

// recordValue extracts a value string from a records row item.
func recordValue(n parser.Node) string {
	if n == nil {
		return "null" // empty field
	}
	if infix, ok := n.(*parser.InfixExprNode); ok && infix.Op.Value == "." {
		// An enum constant such as status.active stores the value itself.
		return extractName(infix.Right)
	}
	name := extractName(n)
	if strings.ToLower(name) == "null" {
		return "null"
	}
	return name
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
