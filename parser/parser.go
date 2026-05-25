package parser

import (
	"dbml-tools/lexer"
	"fmt"
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
	Name  Node        // may be nil
	As    *lexer.Token
	Alias Node
	Attrs Node // ListExprNode, may be nil
	Colon *lexer.Token
	Body  Node // BlockExprNode or expression
}

func (n *ElementDeclNode) FirstToken() lexer.Token { return n.Type }
func (n *ElementDeclNode) LastToken() lexer.Token {
	if n.Body != nil {
		return n.Body.LastToken()
	}
	if n.Attrs != nil {
		return n.Attrs.LastToken()
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
	Name  Node // PrimaryExprNode or IdentStreamNode
	ColonTok *lexer.Token
	Value Node // may be nil
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
}

// declRequiresName lists keywords whose BNF rule mandates a name.
var declRequiresName = map[string]bool{
	"Table": true, "Enum": true, "Project": true,
	"TableGroup": true, "TablePartial": true,
}

// declRequiresBlock lists keywords whose BNF rule mandates a block body {...}.
// (Ref and Note allow either a block or a colon body.)
var declRequiresBlock = map[string]bool{
	"Table": true, "Enum": true, "Project": true,
	"TableGroup": true, "TablePartial": true,
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

	// Validate declaration keyword.
	knownKeyword := validDeclKeywords[typeToken.Value]
	if !knownKeyword {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("unknown declaration keyword %q; expected Table, Enum, Ref, Note, Project, TableGroup, or TablePartial", typeToken.Value),
			Position: typeToken.StartPos,
		})
	}

	var name Node
	var asToken *lexer.Token
	var alias Node
	var attrs Node
	var colon *lexer.Token
	var body Node

	// optional name (anything that isn't : or {)
	if p.cur().Kind != lexer.KindColon && p.cur().Kind != lexer.KindLBrace &&
		p.cur().Kind != lexer.KindEOF && !p.hasNewlineBefore() {
		name = p.parseDotExpr()
	}

	// Validate required name for known keywords.
	if knownKeyword && declRequiresName[typeToken.Value] && name == nil {
		p.Errors = append(p.Errors, Error{
			Message:  fmt.Sprintf("%s declaration requires a name", typeToken.Value),
			Position: typeToken.StartPos,
		})
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
		body = p.parseBlockExpr()
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
	if knownKeyword && body != nil && declRequiresBlock[typeToken.Value] {
		if _, ok := body.(*BlockExprNode); !ok {
			p.Errors = append(p.Errors, Error{
				Message:  fmt.Sprintf("%s declaration requires a block body {...}", typeToken.Value),
				Position: typeToken.StartPos,
			})
		}
	}

	return &ElementDeclNode{
		Type:  typeToken,
		Name:  name,
		As:    asToken,
		Alias: alias,
		Attrs: attrs,
		Colon: colon,
		Body:  body,
	}
}

// ---------------------------------------------------------------------------
// Block expression { ... }
// ---------------------------------------------------------------------------

func (p *Parser) parseBlockExpr() *BlockExprNode {
	open := p.expect(lexer.KindLBrace)
	var body []Node
	for p.cur().Kind != lexer.KindRBrace && p.cur().Kind != lexer.KindEOF {
		// Nested brace block (e.g. records/indexes body inside a table block):
		// consume it as a sub-block so the inner } doesn't close the outer block.
		if p.cur().Kind == lexer.KindLBrace {
			body = append(body, p.parseBlockExpr())
			continue
		}
		item := p.parseFuncApp()
		if item != nil {
			body = append(body, item)
		}
	}
	close := p.expect(lexer.KindRBrace)
	return &BlockExprNode{Open: open, Body: body, Close: close}
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
		right := p.parsePrimaryExpr()
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
		return p.parseDotExpr()
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
