package parser

import (
	"fmt"
	"strings"

	"github.com/mevdschee/dbml-tools/lexer"
)

// ---------------------------------------------------------------------------
// AST node interface
// ---------------------------------------------------------------------------

type Node interface {
	ToJSON() interface{}
	FirstToken() lexer.Token
	LastToken() lexer.Token
}

func nodeStart(n Node) int     { return n.FirstToken().Start }
func nodeEnd(n Node) int       { return n.LastToken().End }
func nodeFullStart(n Node) int { return n.FirstToken().FullStart() }
func nodeFullEnd(n Node) int   { return n.LastToken().FullEnd() }

func nodePosFields(n Node) map[string]interface{} {
	return map[string]interface{}{
		"startPos":  lexer.PosJSON(n.FirstToken().StartPos),
		"fullStart": nodeFullStart(n),
		"endPos":    lexer.PosJSON(n.LastToken().EndPos),
		"fullEnd":   nodeFullEnd(n),
		"start":     nodeStart(n),
		"end":       nodeEnd(n),
	}
}

// ---------------------------------------------------------------------------
// Concrete node types
// ---------------------------------------------------------------------------

// ProgramNode ---------------------------------------------------------------

type ProgramNode struct {
	Source string
	Body   []Node
	First  lexer.Token // first body token (or EOF)
	Last   lexer.Token // last body token (or EOF)
}

func (n *ProgramNode) FirstToken() lexer.Token { return n.First }
func (n *ProgramNode) LastToken() lexer.Token  { return n.Last }

func (n *ProgramNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<program>"
	m["source"] = n.Source
	body := make([]interface{}, len(n.Body))
	for i, c := range n.Body {
		body[i] = c.ToJSON()
	}
	m["body"] = body
	return m
}

// ElementDeclNode -----------------------------------------------------------

type ElementDeclNode struct {
	Type  lexer.Token
	Name  Node // may be nil
	As    *lexer.Token
	Alias Node
	// Columns is the column list of a top-level `Records tbl(a, b)`
	// declaration; nil for every other declaration.
	Columns *TupleExprNode
	Attrs   Node // ListExprNode, may be nil
	Colon   *lexer.Token
	Body    Node // BlockExprNode or expression
}

func (n *ElementDeclNode) FirstToken() lexer.Token { return n.Type }
func (n *ElementDeclNode) LastToken() lexer.Token {
	if n.Body != nil {
		return n.Body.LastToken()
	}
	if n.Attrs != nil {
		return n.Attrs.LastToken()
	}
	if n.Columns != nil {
		return n.Columns.LastToken()
	}
	if n.Name != nil {
		return n.Name.LastToken()
	}
	return n.Type
}

func (n *ElementDeclNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<element-declaration>"
	m["type"] = n.Type.ToJSON()
	if n.Name != nil {
		m["name"] = n.Name.ToJSON()
	}
	if n.As != nil {
		m["as"] = n.As.ToJSON()
	}
	if n.Alias != nil {
		m["alias"] = n.Alias.ToJSON()
	}
	if n.Columns != nil {
		m["columns"] = n.Columns.ToJSON()
	}
	if n.Attrs != nil {
		m["attributeList"] = n.Attrs.ToJSON()
	}
	if n.Colon != nil {
		m["bodyColon"] = n.Colon.ToJSON()
	}
	if n.Body != nil {
		m["body"] = n.Body.ToJSON()
	}
	return m
}

// BlockExprNode -------------------------------------------------------------

type BlockExprNode struct {
	Open  lexer.Token
	Body  []Node
	Close lexer.Token
}

func (n *BlockExprNode) FirstToken() lexer.Token { return n.Open }
func (n *BlockExprNode) LastToken() lexer.Token  { return n.Close }

func (n *BlockExprNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<block-expression>"
	m["blockOpenBrace"] = n.Open.ToJSON()
	body := make([]interface{}, len(n.Body))
	for i, c := range n.Body {
		body[i] = c.ToJSON()
	}
	m["body"] = body
	m["blockCloseBrace"] = n.Close.ToJSON()
	return m
}

// FuncAppNode ---------------------------------------------------------------

type FuncAppNode struct {
	Callee Node
	Args   []Node
}

func (n *FuncAppNode) FirstToken() lexer.Token { return n.Callee.FirstToken() }
func (n *FuncAppNode) LastToken() lexer.Token {
	if len(n.Args) > 0 {
		return n.Args[len(n.Args)-1].LastToken()
	}
	return n.Callee.LastToken()
}

func (n *FuncAppNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<function-application>"
	m["callee"] = n.Callee.ToJSON()
	args := make([]interface{}, len(n.Args))
	for i, a := range n.Args {
		args[i] = a.ToJSON()
	}
	m["args"] = args
	return m
}

// PrimaryExprNode -----------------------------------------------------------

type PrimaryExprNode struct {
	Expr Node // VariableNode or LiteralNode
}

func (n *PrimaryExprNode) FirstToken() lexer.Token { return n.Expr.FirstToken() }
func (n *PrimaryExprNode) LastToken() lexer.Token  { return n.Expr.LastToken() }

func (n *PrimaryExprNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<primary-expression>"
	m["expression"] = n.Expr.ToJSON()
	return m
}

// VariableNode --------------------------------------------------------------

type VariableNode struct {
	Token lexer.Token
}

func (n *VariableNode) FirstToken() lexer.Token { return n.Token }
func (n *VariableNode) LastToken() lexer.Token  { return n.Token }

func (n *VariableNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<variable>"
	m["variable"] = n.Token.ToJSON()
	return m
}

// LiteralNode ---------------------------------------------------------------

type LiteralNode struct {
	Token lexer.Token
}

func (n *LiteralNode) FirstToken() lexer.Token { return n.Token }
func (n *LiteralNode) LastToken() lexer.Token  { return n.Token }

func (n *LiteralNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<literal>"
	m["literal"] = n.Token.ToJSON()
	return m
}

// InfixExprNode -------------------------------------------------------------

type InfixExprNode struct {
	Left  Node
	Op    lexer.Token
	Right Node
}

func (n *InfixExprNode) FirstToken() lexer.Token { return n.Left.FirstToken() }
func (n *InfixExprNode) LastToken() lexer.Token  { return n.Right.LastToken() }

func (n *InfixExprNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<infix-expression>"
	m["left"] = n.Left.ToJSON()
	m["op"] = n.Op.ToJSON()
	m["right"] = n.Right.ToJSON()
	return m
}

// ListExprNode (attribute list []) ------------------------------------------

type ListExprNode struct {
	Open  lexer.Token
	Items []Node
	Seps  []lexer.Token
	Close lexer.Token
}

func (n *ListExprNode) FirstToken() lexer.Token { return n.Open }
func (n *ListExprNode) LastToken() lexer.Token  { return n.Close }

func (n *ListExprNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<list-expression>"
	m["listOpenBracket"] = n.Open.ToJSON()
	items := make([]interface{}, len(n.Items))
	for i, c := range n.Items {
		items[i] = c.ToJSON()
	}
	m["items"] = items
	if len(n.Seps) > 0 {
		seps := make([]interface{}, len(n.Seps))
		for i, s := range n.Seps {
			seps[i] = s.ToJSON()
		}
		m["separators"] = seps
	}
	m["listCloseBracket"] = n.Close.ToJSON()
	return m
}

// AttributeNode -------------------------------------------------------------

type AttributeNode struct {
	Name     Node // PrimaryExprNode or IdentStreamNode
	ColonTok *lexer.Token
	Value    Node // may be nil
}

func (n *AttributeNode) FirstToken() lexer.Token { return n.Name.FirstToken() }
func (n *AttributeNode) LastToken() lexer.Token {
	if n.Value != nil {
		return n.Value.LastToken()
	}
	if n.ColonTok != nil {
		return *n.ColonTok
	}
	return n.Name.LastToken()
}

func (n *AttributeNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<attribute>"
	m["name"] = n.Name.ToJSON()
	if n.ColonTok != nil {
		m["colon"] = n.ColonTok.ToJSON()
	}
	if n.Value != nil {
		m["value"] = n.Value.ToJSON()
	}
	return m
}

// IdentStreamNode -----------------------------------------------------------

type IdentStreamNode struct {
	Tokens []lexer.Token
}

func (n *IdentStreamNode) FirstToken() lexer.Token { return n.Tokens[0] }
func (n *IdentStreamNode) LastToken() lexer.Token  { return n.Tokens[len(n.Tokens)-1] }

func (n *IdentStreamNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<identifier-stream>"
	ids := make([]interface{}, len(n.Tokens))
	for i, t := range n.Tokens {
		ids[i] = t.ToJSON()
	}
	m["identifiers"] = ids
	return m
}

// TupleExprNode (parenthesized list) ----------------------------------------

type TupleExprNode struct {
	Open  lexer.Token
	Items []Node
	Seps  []lexer.Token
	Close lexer.Token
}

func (n *TupleExprNode) FirstToken() lexer.Token { return n.Open }
func (n *TupleExprNode) LastToken() lexer.Token  { return n.Close }

func (n *TupleExprNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<tuple-expression>"
	m["tupleOpenParen"] = n.Open.ToJSON()
	items := make([]interface{}, len(n.Items))
	for i, c := range n.Items {
		items[i] = c.ToJSON()
	}
	m["items"] = items
	m["tupleCloseParen"] = n.Close.ToJSON()
	return m
}

// RecordRowNode (one data row of a records block) ---------------------------

// RecordRowNode is one line of a records block: values separated by commas.
// An empty field, as in `1, , 'x'`, is a nil entry in Values. There is
// always one more value than separators.
type RecordRowNode struct {
	Values []Node
	Seps   []lexer.Token
}

func (n *RecordRowNode) FirstToken() lexer.Token {
	if n.Values[0] != nil {
		return n.Values[0].FirstToken()
	}
	return n.Seps[0]
}

func (n *RecordRowNode) LastToken() lexer.Token {
	if last := n.Values[len(n.Values)-1]; last != nil {
		return last.LastToken()
	}
	return n.Seps[len(n.Seps)-1]
}

func (n *RecordRowNode) ToJSON() interface{} {
	m := nodePosFields(n)
	m["kind"] = "<record-row>"
	vals := make([]interface{}, len(n.Values))
	for i, v := range n.Values {
		if v != nil {
			vals[i] = v.ToJSON()
		}
	}
	m["values"] = vals
	return m
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

type Error struct {
	Message  string
	Position lexer.Position
}

func (e Error) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Position.Line+1, e.Position.Column+1, e.Message)
}

type Parser struct {
	tokens []lexer.Token
	pos    int
	source string
	Errors []Error
}

func New(tokens []lexer.Token, source string) *Parser {
	return &Parser{tokens: tokens, source: source}
}

func (p *Parser) cur() lexer.Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	// return an EOF-like token
	return lexer.Token{Kind: lexer.KindEOF}
}

func (p *Parser) consume() lexer.Token {
	t := p.cur()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *Parser) expect(k lexer.TokenKind) lexer.Token {
	t := p.cur()
	if t.Kind != k {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("expected %s, got %s", k, t.Kind),
			Position: t.StartPos,
		})
	}
	return p.consume()
}

func (p *Parser) hasNewlineBefore() bool {
	if p.pos == 0 {
		return false
	}
	return p.cur().StartPos.Line > p.tokens[p.pos-1].StartPos.Line
}

func (p *Parser) isExprStart() bool {
	switch p.cur().Kind {
	case lexer.KindIdentifier, lexer.KindString, lexer.KindNumeric,
		lexer.KindLBracket, lexer.KindLParen, lexer.KindQuotedString,
		lexer.KindFuncExpr, lexer.KindColor:
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Parse entry point
// ---------------------------------------------------------------------------

func (p *Parser) Parse() *ProgramNode {
	var body []Node
	for p.cur().Kind != lexer.KindEOF {
		decl := p.parseDeclaration()
		if decl != nil {
			body = append(body, decl)
		}
	}

	eof := p.cur()
	first := eof
	last := eof
	if len(body) > 0 {
		first = body[0].FirstToken()
		last = body[len(body)-1].LastToken()
	}

	return &ProgramNode{
		Source: p.source,
		Body:   body,
		First:  first,
		Last:   last,
	}
}

// ---------------------------------------------------------------------------
// Declaration parsing
// ---------------------------------------------------------------------------

// validDeclKeywords is the set of recognised top-level DBML declaration keywords.
var validDeclKeywords = map[string]bool{
	"Table": true, "Enum": true, "Ref": true, "Note": true,
	"Project": true, "TableGroup": true, "TablePartial": true,
	"Records": true,
}

// IsRecordsKeyword reports whether v is the Records keyword. The DBML docs
// write it both as `Records` and `records`, so case is ignored.
func IsRecordsKeyword(v string) bool { return strings.EqualFold(v, "records") }

// declRequiresName lists keywords whose BNF rule mandates a name.
// Project is absent on purpose: the grammar makes its name optional.
var declRequiresName = map[string]bool{
	"Table": true, "Enum": true,
	"TableGroup": true, "TablePartial": true, "Records": true,
}

// declRequiresBlock lists keywords whose BNF rule mandates a block body {...}.
// (Ref and Note allow either a block or a colon body.)
var declRequiresBlock = map[string]bool{
	"Table": true, "Enum": true, "Project": true,
	"TableGroup": true, "TablePartial": true, "Records": true,
}

func (p *Parser) parseDeclaration() Node {
	if p.cur().Kind != lexer.KindIdentifier {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("expected declaration keyword, got %s (%q)", p.cur().Kind, p.cur().Value),
			Position: p.cur().StartPos,
		})
		p.consume() // skip
		return nil
	}

	typeToken := p.consume()
	isRecords := IsRecordsKeyword(typeToken.Value)
	keyword := typeToken.Value
	if isRecords {
		keyword = "Records"
	}

	// Validate declaration keyword.
	knownKeyword := validDeclKeywords[keyword]
	if !knownKeyword {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("unknown declaration keyword %q; expected Table, Enum, Ref, Note, Project, TableGroup, TablePartial, or Records", typeToken.Value),
			Position: typeToken.StartPos,
		})
	}

	var name Node
	var asToken *lexer.Token
	var alias Node
	var columns *TupleExprNode
	var attrs Node
	var colon *lexer.Token
	var body Node

	// optional name (anything that isn't : or {)
	if p.cur().Kind != lexer.KindColon && p.cur().Kind != lexer.KindLBrace &&
		p.cur().Kind != lexer.KindEOF && !p.hasNewlineBefore() {
		name = p.parseDotExpr()
	}

	// Validate required name for known keywords.
	if knownKeyword && declRequiresName[keyword] && name == nil {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("%s declaration requires a name", typeToken.Value),
			Position: typeToken.StartPos,
		})
	}

	// Records outside a table must list its columns: Records tbl(a, b).
	if isRecords && name != nil {
		if p.cur().Kind == lexer.KindLParen && !p.hasNewlineBefore() {
			columns = p.parseTupleExpr().(*TupleExprNode)
		} else {
			p.Errors = append(p.Errors, Error{
				Message:  "Records declaration requires a column list, as in Records users(id, name)",
				Position: typeToken.StartPos,
			})
		}
	}

	// optional "as" alias
	if p.cur().Kind == lexer.KindIdentifier && p.cur().Value == "as" && !p.hasNewlineBefore() {
		as := p.consume()
		asToken = &as
		alias = p.parsePrimaryExpr()
	}

	// optional attribute list
	if p.cur().Kind == lexer.KindLBracket && !p.hasNewlineBefore() {
		attrs = p.parseListExpr()
	}

	// body: block or colon-expression
	if p.cur().Kind == lexer.KindLBrace {
		if typeToken.Value == "Ref" {
			body = p.parseBlockExpr(p.parseColonBody)
		} else if isRecords {
			body = p.parseBlockExpr(p.parseRecordRow)
		} else {
			body = p.parseBlockExpr(nil)
		}
	} else if p.cur().Kind == lexer.KindColon && !p.hasNewlineBefore() {
		c := p.consume()
		colon = &c
		body = p.parseColonBody()
	}

	// Validate that a body is present.
	if knownKeyword && body == nil {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("%s declaration requires a body", typeToken.Value),
			Position: typeToken.StartPos,
		})
	}

	// Validate that keywords requiring a block body don't use colon syntax.
	if knownKeyword && body != nil && declRequiresBlock[keyword] {
		if _, ok := body.(*BlockExprNode); !ok {
			p.Errors = append(p.Errors, Error{
				Message:  fmt.Sprintf("%s declaration requires a block body {...}", typeToken.Value),
				Position: typeToken.StartPos,
			})
		}
	}

	return &ElementDeclNode{
		Type:    typeToken,
		Name:    name,
		As:      asToken,
		Alias:   alias,
		Columns: columns,
		Attrs:   attrs,
		Colon:   colon,
		Body:    body,
	}
}

// ---------------------------------------------------------------------------
// Block expression { ... }
// ---------------------------------------------------------------------------

// parseBlockExpr parses a { ... } body. itemParser parses a single body item;
// pass nil for the default function-application form. Ref blocks pass
// parseColonBody so their lines become proper relationship trees.
func (p *Parser) parseBlockExpr(itemParser func() Node) *BlockExprNode {
	open := p.expect(lexer.KindLBrace)
	var body []Node
	for p.cur().Kind != lexer.KindRBrace && p.cur().Kind != lexer.KindEOF {
		// Nested brace block (e.g. records/indexes body inside a table block):
		// consume it as a sub-block so the inner } doesn't close the outer block.
		// A block that follows a records head holds data rows; any other
		// nested block uses the default item parser.
		if p.cur().Kind == lexer.KindLBrace {
			if len(body) > 0 && isRecordsHead(body[len(body)-1]) {
				body = append(body, p.parseBlockExpr(p.parseRecordRow))
			} else {
				body = append(body, p.parseBlockExpr(nil))
			}
			continue
		}
		var item Node
		if itemParser != nil {
			item = itemParser()
		} else {
			item = p.parseFuncApp()
		}
		if item != nil {
			body = append(body, item)
		}
	}
	close := p.expect(lexer.KindRBrace)
	return &BlockExprNode{Open: open, Body: body, Close: close}
}

// isRecordsHead reports whether n is the head of a records block inside a
// table: `records`, `records (a, b)`, or either with a settings list.
func isRecordsHead(n Node) bool {
	var callee Node = n
	if fa, ok := n.(*FuncAppNode); ok {
		for _, a := range fa.Args {
			switch a.(type) {
			case *TupleExprNode, *ListExprNode:
			default:
				return false
			}
		}
		callee = fa.Callee
	}
	pe, ok := callee.(*PrimaryExprNode)
	if !ok {
		return false
	}
	v, ok := pe.Expr.(*VariableNode)
	return ok && v.Token.Kind == lexer.KindIdentifier && IsRecordsKeyword(v.Token.Value)
}

// ---------------------------------------------------------------------------
// Records rows
// ---------------------------------------------------------------------------

// parseRecordRow parses one line of a records block: comma separated values
// that end at the line break. A field may be empty (`1, , 3`).
func (p *Parser) parseRecordRow() Node {
	row := &RecordRowNode{}
	row.Values = append(row.Values, p.parseRecordValue(false))
	for p.cur().Kind == lexer.KindComma && !p.hasNewlineBefore() {
		row.Seps = append(row.Seps, p.consume())
		row.Values = append(row.Values, p.parseRecordValue(true))
	}
	if row.Values[0] == nil && len(row.Seps) == 0 {
		// Not a value at all: report it and skip so the block loop advances.
		tok := p.consume()
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("unexpected token %s (%q) in records row", tok.Kind, tok.Value),
			Position: tok.StartPos,
		})
		return nil
	}
	if !p.hasNewlineBefore() && p.cur().Kind != lexer.KindRBrace && p.cur().Kind != lexer.KindEOF {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("expected , or end of line in records row, got %s (%q)", p.cur().Kind, p.cur().Value),
			Position: p.cur().StartPos,
		})
	}
	return row
}

// parseRecordValue parses a single records value, or returns nil for an
// empty field. Values are literals, identifiers such as null and true,
// enum constants such as status.active, and signed numbers.
func (p *Parser) parseRecordValue(afterSep bool) Node {
	// A comma at the end of a line leaves an empty last field.
	if afterSep && p.hasNewlineBefore() {
		return nil
	}
	switch p.cur().Kind {
	case lexer.KindComma, lexer.KindRBrace, lexer.KindEOF:
		return nil
	case lexer.KindIdentifier, lexer.KindQuotedString:
		return p.parseDotExpr()
	case lexer.KindOp:
		sign := p.cur()
		if (sign.Value == "-" || sign.Value == "+") && p.pos+1 < len(p.tokens) {
			num := p.tokens[p.pos+1]
			if num.Kind == lexer.KindNumeric && num.Start == sign.End {
				p.pos += 2
				merged := num
				merged.Value = sign.Value + num.Value
				merged.Start = sign.Start
				merged.StartPos = sign.StartPos
				merged.LeadingTrivia = sign.LeadingTrivia
				merged.LeadingInvalid = sign.LeadingInvalid
				return &PrimaryExprNode{Expr: &LiteralNode{Token: merged}}
			}
		}
	case lexer.KindString, lexer.KindNumeric, lexer.KindFuncExpr, lexer.KindColor:
		return p.parsePrimaryExpr()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Function application (field definitions etc.)
// ---------------------------------------------------------------------------

func (p *Parser) parseFuncApp() Node {
	if p.cur().Kind == lexer.KindOp && p.cur().Value == "~" {
		// partial injection ~name
		op := p.consume()
		if p.cur().Kind == lexer.KindIdentifier {
			name := p.parsePrimaryExpr()
			return &FuncAppNode{
				Callee: &PrimaryExprNode{Expr: &VariableNode{Token: op}},
				Args:   []Node{name},
			}
		}
		return &PrimaryExprNode{Expr: &VariableNode{Token: op}}
	}

	// A statement may begin with a parenthesised tuple — e.g. composite or
	// parenthesised single-column index entries: `(a, b) [unique]`, `(a)`.
	// A statement may also begin with a comma — records body rows like
	// `'1', 'foo'` are split by the block loop into a PrimaryExpr for the
	// first value plus subsequent FuncApps whose callee is the comma.
	var callee Node
	switch p.cur().Kind {
	case lexer.KindLParen:
		callee = p.parseTupleExpr()
	case lexer.KindComma:
		tok := p.consume()
		callee = &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
	default:
		callee = p.parsePrimaryExpr()
	}
	var args []Node
	for !p.hasNewlineBefore() &&
		p.cur().Kind != lexer.KindRBrace &&
		p.cur().Kind != lexer.KindEOF &&
		(p.isExprStart() || p.cur().Kind == lexer.KindColon ||
			(p.cur().Kind == lexer.KindOp && p.cur().Value != "}")) {

		var arg Node
		if p.cur().Kind == lexer.KindLBracket {
			arg = p.parseListExpr()
		} else if p.cur().Kind == lexer.KindLParen {
			arg = p.parseTupleExpr()
		} else if p.cur().Kind == lexer.KindOp {
			tok := p.consume()
			arg = &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
		} else if p.cur().Kind == lexer.KindColon {
			tok := p.consume()
			arg = &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
		} else {
			arg = p.parsePrimaryExpr()
		}
		args = append(args, arg)
	}

	if len(args) == 0 {
		return callee
	}
	return &FuncAppNode{Callee: callee, Args: args}
}

// ---------------------------------------------------------------------------
// Colon-body expression (for Ref: ... etc.)
// ---------------------------------------------------------------------------

func (p *Parser) parseColonBody() Node {
	left := p.parseDotExpr()

	// relationship operator
	if p.cur().Kind == lexer.KindOp && !p.hasNewlineBefore() {
		v := p.cur().Value
		if v == ">" || v == "<" || v == "-" || v == "<>" {
			op := p.consume()
			right := p.parseDotExpr()
			left = &InfixExprNode{Left: left, Op: op, Right: right}
		}
	}

	// optional additional args on same line (for settings etc.)
	var args []Node
	for !p.hasNewlineBefore() && p.cur().Kind != lexer.KindEOF && p.isExprStart() {
		if p.cur().Kind == lexer.KindLBracket {
			args = append(args, p.parseListExpr())
		} else {
			args = append(args, p.parsePrimaryExpr())
		}
	}

	if len(args) > 0 {
		return &FuncAppNode{Callee: left, Args: args}
	}
	return left
}

// ---------------------------------------------------------------------------
// Dot expression  (a.b.c)
// ---------------------------------------------------------------------------

func (p *Parser) parseDotExpr() Node {
	left := p.parsePrimaryExpr()
	for p.cur().Kind == lexer.KindOp && p.cur().Value == "." && !p.hasNewlineBefore() {
		op := p.consume()
		var right Node
		if p.cur().Kind == lexer.KindLParen {
			// Composite ref endpoint: table.(col1, col2)
			right = p.parseTupleExpr()
		} else {
			right = p.parsePrimaryExpr()
		}
		left = &InfixExprNode{Left: left, Op: op, Right: right}
	}
	return left
}

// ---------------------------------------------------------------------------
// Primary expression
// ---------------------------------------------------------------------------

func (p *Parser) parsePrimaryExpr() Node {
	tok := p.consume()
	switch tok.Kind {
	case lexer.KindIdentifier, lexer.KindQuotedString:
		return &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
	case lexer.KindString, lexer.KindNumeric, lexer.KindColor, lexer.KindFuncExpr:
		return &PrimaryExprNode{Expr: &LiteralNode{Token: tok}}
	default:
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("unexpected token %s (%q)", tok.Kind, tok.Value),
			Position: tok.StartPos,
		})
		return &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
	}
}

// ---------------------------------------------------------------------------
// List expression [...]
// ---------------------------------------------------------------------------

func (p *Parser) parseListExpr() Node {
	open := p.expect(lexer.KindLBracket)
	var items []Node
	var seps []lexer.Token

	if p.cur().Kind != lexer.KindRBracket {
		items = append(items, p.parseAttribute())
		for p.cur().Kind == lexer.KindComma {
			seps = append(seps, p.consume())
			items = append(items, p.parseAttribute())
		}
	}

	close := p.expect(lexer.KindRBracket)
	return &ListExprNode{Open: open, Items: items, Seps: seps, Close: close}
}

// ---------------------------------------------------------------------------
// Attribute inside [...]
// ---------------------------------------------------------------------------

func (p *Parser) parseAttribute() Node {
	// Collect consecutive identifiers (for multi-word like "primary key")
	var idents []lexer.Token
	for p.cur().Kind == lexer.KindIdentifier {
		idents = append(idents, p.consume())
		if p.cur().Kind == lexer.KindColon {
			break
		}
		if p.cur().Kind == lexer.KindComma || p.cur().Kind == lexer.KindRBracket {
			break
		}
	}

	if p.cur().Kind == lexer.KindColon && len(idents) > 0 {
		colon := p.consume()
		value := p.parseAttrValue()
		var name Node
		if len(idents) == 1 {
			name = &PrimaryExprNode{Expr: &VariableNode{Token: idents[0]}}
		} else {
			name = &IdentStreamNode{Tokens: idents}
		}
		return &AttributeNode{Name: name, ColonTok: &colon, Value: value}
	}

	if len(idents) == 1 {
		return &AttributeNode{Name: &PrimaryExprNode{Expr: &VariableNode{Token: idents[0]}}}
	}
	if len(idents) > 1 {
		return &AttributeNode{Name: &IdentStreamNode{Tokens: idents}}
	}

	// fallback: consume whatever is there
	tok := p.consume()
	return &AttributeNode{Name: &PrimaryExprNode{Expr: &VariableNode{Token: tok}}}
}

func (p *Parser) parseAttrValue() Node {
	switch p.cur().Kind {
	case lexer.KindString, lexer.KindNumeric, lexer.KindColor, lexer.KindFuncExpr:
		tok := p.consume()
		return &PrimaryExprNode{Expr: &LiteralNode{Token: tok}}
	case lexer.KindIdentifier:
		return p.parseIdentAttrValue()
	case lexer.KindOp:
		// e.g. [ref: > table.col]
		op := p.consume()
		right := p.parseDotExpr()
		return &FuncAppNode{
			Callee: &PrimaryExprNode{Expr: &VariableNode{Token: op}},
			Args:   []Node{right},
		}
	default:
		tok := p.consume()
		return &PrimaryExprNode{Expr: &VariableNode{Token: tok}}
	}
}

// parseIdentAttrValue parses an identifier-valued attribute. Multi-word values
// such as `set null`, `set default` and `no action` are written unquoted in
// DBML, so consecutive identifiers are gathered into a single value.
func (p *Parser) parseIdentAttrValue() Node {
	first := p.parseDotExpr()
	if _, ok := first.(*PrimaryExprNode); !ok {
		return first // dotted (e.g. table.column); not a multi-word value
	}
	idents := []lexer.Token{first.FirstToken()}
	for p.cur().Kind == lexer.KindIdentifier && !p.hasNewlineBefore() {
		idents = append(idents, p.consume())
	}
	if len(idents) == 1 {
		return first
	}
	return &IdentStreamNode{Tokens: idents}
}

// ---------------------------------------------------------------------------
// Tuple expression (...)
// ---------------------------------------------------------------------------

func (p *Parser) parseTupleExpr() Node {
	open := p.expect(lexer.KindLParen)
	var items []Node
	var seps []lexer.Token

	if p.cur().Kind != lexer.KindRParen {
		items = append(items, p.parseDotExpr())
		for p.cur().Kind == lexer.KindComma {
			seps = append(seps, p.consume())
			items = append(items, p.parseDotExpr())
		}
	}

	close := p.expect(lexer.KindRParen)
	return &TupleExprNode{Open: open, Items: items, Seps: seps, Close: close}
}
