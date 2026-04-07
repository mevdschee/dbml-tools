package lexer

import (
	"fmt"
	"unicode"
)

// ---------------------------------------------------------------------------
// Position & token types
// ---------------------------------------------------------------------------

type Position struct {
	Offset int `json:"offset"` // rune offset from start of source
	Line   int `json:"line"`
	Column int `json:"column"` // rune offset from start of line
}

type TokenKind string

const (
	KindIdentifier    TokenKind = "<identifier>"
	KindString        TokenKind = "<string>"
	KindNumeric       TokenKind = "<numeric-literal>"
	KindColor         TokenKind = "<color-literal>"
	KindFuncExpr      TokenKind = "<function-expression>"
	KindQuotedString  TokenKind = "<quoted-string>"
	KindLBrace        TokenKind = "<lbrace>"
	KindRBrace        TokenKind = "<rbrace>"
	KindLBracket      TokenKind = "<lbracket>"
	KindRBracket      TokenKind = "<rbracket>"
	KindLParen        TokenKind = "<lparen>"
	KindRParen        TokenKind = "<rparen>"
	KindColon         TokenKind = "<colon>"
	KindComma         TokenKind = "<comma>"
	KindSemicolon     TokenKind = "<semicolon>"
	KindOp            TokenKind = "<op>"
	KindEOF           TokenKind = "<eof>"

	// trivia
	KindSpace         TokenKind = "<space>"
	KindTab           TokenKind = "<tab>"
	KindNewline       TokenKind = "<newline>"
	KindSingleComment TokenKind = "<single-line-comment>"
	KindMultiComment  TokenKind = "<multiline-comment>"
)

// Token is a lexical token with trivia attached.
type Token struct {
	Kind            TokenKind `json:"kind"`
	Value           string    `json:"value"`
	StartPos        Position  `json:"startPos"`
	EndPos          Position  `json:"endPos"`
	LeadingTrivia   []Token   `json:"leadingTrivia"`
	TrailingTrivia  []Token   `json:"trailingTrivia"`
	LeadingInvalid  []Token   `json:"leadingInvalid"`
	TrailingInvalid []Token   `json:"trailingInvalid"`
	IsInvalid       bool      `json:"isInvalid"`
	Start           int       `json:"start"` // rune offset
	End             int       `json:"end"`   // rune offset
}

func (t Token) IsTrivia() bool {
	switch t.Kind {
	case KindSpace, KindTab, KindNewline, KindSingleComment, KindMultiComment:
		return true
	}
	return false
}

func (t Token) FullStart() int {
	if len(t.LeadingTrivia) > 0 {
		return t.LeadingTrivia[0].Start
	}
	return t.Start
}

func (t Token) FullEnd() int {
	if len(t.TrailingTrivia) > 0 {
		return t.TrailingTrivia[len(t.TrailingTrivia)-1].End
	}
	return t.End
}

// ToJSON returns the token in the parser-JSON format (with trivia).
func (t Token) ToJSON() map[string]interface{} {
	return map[string]interface{}{
		"kind":            string(t.Kind),
		"startPos":        PosJSON(t.StartPos),
		"endPos":          PosJSON(t.EndPos),
		"value":           t.Value,
		"leadingTrivia":   triviaSlice(t.LeadingTrivia),
		"trailingTrivia":  triviaSlice(t.TrailingTrivia),
		"leadingInvalid":  triviaSlice(t.LeadingInvalid),
		"trailingInvalid": triviaSlice(t.TrailingInvalid),
		"isInvalid":       t.IsInvalid,
		"start":           t.Start,
		"end":             t.End,
	}
}

func PosJSON(p Position) map[string]interface{} {
	return map[string]interface{}{
		"offset": p.Offset,
		"line":   p.Line,
		"column": p.Column,
	}
}

func triviaSlice(ts []Token) []interface{} {
	out := make([]interface{}, len(ts))
	for i, t := range ts {
		out[i] = t.ToJSON()
	}
	return out
}

// ---------------------------------------------------------------------------
// Simplified token for lexer.json output
// ---------------------------------------------------------------------------

type SimpleToken struct {
	Kind     TokenKind      `json:"kind"`
	Value    string         `json:"value"`
	Position SimplePosition `json:"position"`
}

type SimplePosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

func ToSimpleTokens(tokens []Token) []SimpleToken {
	out := make([]SimpleToken, len(tokens))
	for i, t := range tokens {
		out[i] = SimpleToken{
			Kind:  t.Kind,
			Value: t.Value,
			Position: SimplePosition{
				Line:   t.StartPos.Line + 1,
				Column: t.StartPos.Column + 1,
			},
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

type Error struct {
	Message  string   `json:"message"`
	Position Position `json:"position"`
}

func (e Error) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Position.Line+1, e.Position.Column+1, e.Message)
}

// ---------------------------------------------------------------------------
// Lexer
// ---------------------------------------------------------------------------

type Lexer struct {
	src    []rune
	pos    int
	line   int
	col    int
	Errors []Error
}

func New(source string) *Lexer {
	return &Lexer{src: []rune(source)}
}

func (l *Lexer) curPos() Position { return Position{l.pos, l.line, l.col} }

func (l *Lexer) peek() rune {
	if l.pos+1 < len(l.src) {
		return l.src[l.pos+1]
	}
	return 0
}

func (l *Lexer) advance() {
	if l.pos < len(l.src) {
		if l.src[l.pos] == '\n' {
			l.line++
			l.col = 0
		} else {
			l.col++
		}
		l.pos++
	}
}

// Lex returns significant tokens with trivia attached.
func (l *Lexer) Lex() []Token {
	raw := l.scanAll()
	return attachTrivia(raw)
}

// ---------------------------------------------------------------------------
// Raw scanning – produces ALL tokens (trivia + significant) in order
// ---------------------------------------------------------------------------

func (l *Lexer) scanAll() []Token {
	var out []Token
	for l.pos < len(l.src) {
		out = append(out, l.next())
	}
	p := l.curPos()
	out = append(out, Token{
		Kind: KindEOF, Value: "",
		StartPos: p, EndPos: p,
		Start: l.pos, End: l.pos,
		LeadingTrivia: emptyTrivia(), TrailingTrivia: emptyTrivia(),
		LeadingInvalid: emptyTrivia(), TrailingInvalid: emptyTrivia(),
	})
	return out
}

func (l *Lexer) next() Token {
	ch := l.src[l.pos]
	switch {
	case ch == ' ':
		return l.single(KindSpace, " ")
	case ch == '\t':
		return l.single(KindTab, "\t")
	case ch == '\n':
		return l.newline()
	case ch == '\r':
		if l.peek() == '\n' {
			s, sp := l.pos, l.curPos()
			l.advance()
			l.advance()
			return l.tok(KindNewline, "\n", sp, l.curPos(), s, l.pos)
		}
		return l.newline()
	case ch == '/' && l.peek() == '/':
		return l.singleLineComment()
	case ch == '/' && l.peek() == '*':
		return l.multiLineComment()
	case ch == '{':
		return l.single(KindLBrace, "{")
	case ch == '}':
		return l.single(KindRBrace, "}")
	case ch == '[':
		return l.single(KindLBracket, "[")
	case ch == ']':
		return l.single(KindRBracket, "]")
	case ch == '(':
		return l.single(KindLParen, "(")
	case ch == ')':
		return l.single(KindRParen, ")")
	case ch == ':':
		return l.single(KindColon, ":")
	case ch == ',':
		return l.single(KindComma, ",")
	case ch == ';':
		return l.single(KindSemicolon, ";")
	case ch == '\'':
		return l.scanString()
	case ch == '"':
		return l.scanQuoted()
	case ch == '`':
		return l.scanFuncExpr()
	case ch == '#':
		return l.scanColor()
	case ch == '<' && l.peek() == '>':
		return l.double(KindOp, "<>")
	case ch == '.' || ch == '>' || ch == '<' || ch == '-' || ch == '~' ||
		ch == '+' || ch == '*' || ch == '%' || ch == '=' || ch == '!' ||
		ch == '&' || ch == '|':
		return l.single(KindOp, string(ch))
	case ch == '/': // standalone / (not comment)
		return l.single(KindOp, "/")
	case isDigit(ch):
		return l.scanNumeric()
	case (ch == 'X' || ch == 'x') && l.peek() == '\'':
		return l.scanHexString()
	case isIdentStart(ch):
		return l.scanIdent()
	default:
		sp := l.curPos()
		s := l.pos
		l.advance()
		t := l.tok(KindIdentifier, string(l.src[s:l.pos]), sp, l.curPos(), s, l.pos)
		t.IsInvalid = true
		l.Errors = append(l.Errors, Error{
			Message:  fmt.Sprintf("unexpected character %q", string(ch)),
			Position: sp,
		})
		return t
	}
}

// helpers -------------------------------------------------------------------

func (l *Lexer) single(k TokenKind, v string) Token {
	sp := l.curPos()
	s := l.pos
	l.advance()
	return l.tok(k, v, sp, l.curPos(), s, l.pos)
}

func (l *Lexer) double(k TokenKind, v string) Token {
	sp := l.curPos()
	s := l.pos
	l.advance()
	l.advance()
	return l.tok(k, v, sp, l.curPos(), s, l.pos)
}

func (l *Lexer) newline() Token {
	sp := l.curPos()
	s := l.pos
	l.advance()
	return l.tok(KindNewline, "\n", sp, l.curPos(), s, l.pos)
}

func (l *Lexer) tok(k TokenKind, v string, sp, ep Position, s, e int) Token {
	return Token{
		Kind: k, Value: v, StartPos: sp, EndPos: ep, Start: s, End: e,
		LeadingTrivia: emptyTrivia(), TrailingTrivia: emptyTrivia(),
		LeadingInvalid: emptyTrivia(), TrailingInvalid: emptyTrivia(),
	}
}

func emptyTrivia() []Token { return []Token{} }

// scanning methods ----------------------------------------------------------

func (l *Lexer) singleLineComment() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // /
	l.advance() // /
	cs := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '\n' {
		l.advance()
	}
	return l.tok(KindSingleComment, string(l.src[cs:l.pos]), sp, l.curPos(), s, l.pos)
}

func (l *Lexer) multiLineComment() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // /
	l.advance() // *
	cs := l.pos
	for l.pos < len(l.src) {
		if l.src[l.pos] == '*' && l.peek() == '/' {
			ce := l.pos
			l.advance()
			l.advance()
			return l.tok(KindMultiComment, string(l.src[cs:ce]), sp, l.curPos(), s, l.pos)
		}
		l.advance()
	}
	l.Errors = append(l.Errors, Error{Message: "unterminated multi-line comment", Position: sp})
	return l.tok(KindMultiComment, string(l.src[cs:l.pos]), sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanString() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // opening '

	// multi-line string '''
	if l.pos+1 < len(l.src) && l.src[l.pos] == '\'' && l.src[l.pos+1] == '\'' {
		l.advance()
		l.advance()
		cs := l.pos
		for l.pos < len(l.src) {
			if l.src[l.pos] == '\'' && l.pos+2 < len(l.src) && l.src[l.pos+1] == '\'' && l.src[l.pos+2] == '\'' {
				ce := l.pos
				l.advance()
				l.advance()
				l.advance()
				return l.tok(KindString, string(l.src[cs:ce]), sp, l.curPos(), s, l.pos)
			}
			l.advance()
		}
		l.Errors = append(l.Errors, Error{Message: "unterminated multi-line string", Position: sp})
		return l.tok(KindString, string(l.src[cs:l.pos]), sp, l.curPos(), s, l.pos)
	}

	// single-line string
	cs := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '\'' && l.src[l.pos] != '\n' {
		l.advance()
	}
	v := string(l.src[cs:l.pos])
	if l.pos < len(l.src) && l.src[l.pos] == '\'' {
		l.advance()
	} else {
		l.Errors = append(l.Errors, Error{Message: "unterminated string", Position: sp})
	}
	return l.tok(KindString, v, sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanQuoted() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // "
	cs := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '"' && l.src[l.pos] != '\n' {
		l.advance()
	}
	v := string(l.src[cs:l.pos])
	if l.pos < len(l.src) && l.src[l.pos] == '"' {
		l.advance()
	} else {
		l.Errors = append(l.Errors, Error{Message: "unterminated quoted string", Position: sp})
	}
	return l.tok(KindQuotedString, v, sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanFuncExpr() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // `
	cs := l.pos
	for l.pos < len(l.src) && l.src[l.pos] != '`' {
		l.advance()
	}
	v := string(l.src[cs:l.pos])
	if l.pos < len(l.src) {
		l.advance()
	} else {
		l.Errors = append(l.Errors, Error{Message: "unterminated function expression", Position: sp})
	}
	return l.tok(KindFuncExpr, v, sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanColor() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // #
	for l.pos < len(l.src) && isHex(l.src[l.pos]) {
		l.advance()
	}
	return l.tok(KindColor, string(l.src[s:l.pos]), sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanNumeric() Token {
	s, sp := l.pos, l.curPos()
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.advance()
	}
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		l.advance()
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.advance()
		}
	}
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		l.advance()
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			l.advance()
		}
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
			l.advance()
		}
	}
	return l.tok(KindNumeric, string(l.src[s:l.pos]), sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanHexString() Token {
	s, sp := l.pos, l.curPos()
	l.advance() // skip X/x
	l.advance() // skip opening '
	for l.pos < len(l.src) && l.src[l.pos] != '\'' {
		l.advance()
	}
	if l.pos < len(l.src) {
		l.advance() // skip closing '
	}
	return l.tok(KindNumeric, string(l.src[s:l.pos]), sp, l.curPos(), s, l.pos)
}

func (l *Lexer) scanIdent() Token {
	s, sp := l.pos, l.curPos()
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.advance()
	}
	return l.tok(KindIdentifier, string(l.src[s:l.pos]), sp, l.curPos(), s, l.pos)
}

// character predicates ------------------------------------------------------

func isDigit(c rune) bool      { return c >= '0' && c <= '9' }
func isHex(c rune) bool        { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isIdentStart(c rune) bool { return c == '_' || unicode.IsLetter(c) }
func isIdentPart(c rune) bool  { return isIdentStart(c) || unicode.IsDigit(c) }

// ---------------------------------------------------------------------------
// Trivia attachment
// ---------------------------------------------------------------------------

// attachTrivia splits the raw token stream into significant tokens with
// leading/trailing trivia. Trailing trivia of a token consists of spaces, tabs,
// an optional single-line comment, and at most one newline (all on the same
// line). Everything else is leading trivia of the next significant token.
func attachTrivia(raw []Token) []Token {
	var result []Token
	var buf []Token

	for _, t := range raw {
		if t.IsTrivia() {
			buf = append(buf, t)
			continue
		}
		if len(result) > 0 && len(buf) > 0 {
			split := traiviaSplit(buf)
			result[len(result)-1].TrailingTrivia = buf[:split]
			t.LeadingTrivia = buf[split:]
		} else {
			t.LeadingTrivia = buf
		}
		if t.LeadingTrivia == nil {
			t.LeadingTrivia = emptyTrivia()
		}
		buf = nil
		result = append(result, t)
	}

	// remaining trivia → trailing of last token
	if len(buf) > 0 && len(result) > 0 {
		last := &result[len(result)-1]
		last.TrailingTrivia = append(last.TrailingTrivia, buf...)
	}

	for i := range result {
		if result[i].LeadingTrivia == nil {
			result[i].LeadingTrivia = emptyTrivia()
		}
		if result[i].TrailingTrivia == nil {
			result[i].TrailingTrivia = emptyTrivia()
		}
	}
	return result
}

// traiviaSplit returns the index up to which trivia belongs to the previous
// token (trailing). Trivia after the split belongs to the next token (leading).
func traiviaSplit(buf []Token) int {
	idx := 0
	for i, t := range buf {
		switch t.Kind {
		case KindSpace, KindTab:
			idx = i + 1
		case KindNewline:
			idx = i + 1
			return idx // newline terminates trailing trivia
		case KindSingleComment:
			idx = i + 1 // include EOL comment, continue for the newline
		default:
			return idx
		}
	}
	return idx
}
