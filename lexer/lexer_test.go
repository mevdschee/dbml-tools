package lexer

import (
	"fmt"
	"testing"
)

func TestSimpleTable(t *testing.T) {
	src := "Table users {\n  id integer\n}\n"
	l := New(src)
	tokens := l.Lex()

	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}

	// Table, users, {, id, integer, }, EOF
	expected := []struct {
		kind  TokenKind
		value string
	}{
		{KindIdentifier, "Table"},
		{KindIdentifier, "users"},
		{KindLBrace, "{"},
		{KindIdentifier, "id"},
		{KindIdentifier, "integer"},
		{KindRBrace, "}"},
		{KindEOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, exp := range expected {
		if tokens[i].Kind != exp.kind {
			t.Errorf("token %d: expected kind %s, got %s", i, exp.kind, tokens[i].Kind)
		}
		if tokens[i].Value != exp.value {
			t.Errorf("token %d: expected value %q, got %q", i, exp.value, tokens[i].Value)
		}
	}
}

func TestStringLiteral(t *testing.T) {
	src := "[note: 'hello world']"
	l := New(src)
	tokens := l.Lex()

	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}

	// [, note, :, 'hello world', ], EOF
	if len(tokens) != 6 {
		t.Fatalf("expected 6 tokens, got %d", len(tokens))
	}
	if tokens[3].Kind != KindString || tokens[3].Value != "hello world" {
		t.Errorf("expected string 'hello world', got %s %q", tokens[3].Kind, tokens[3].Value)
	}
}

func TestMultiLineString(t *testing.T) {
	src := "'''line1\nline2'''"
	l := New(src)
	tokens := l.Lex()

	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}

	if len(tokens) != 2 { // string + EOF
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != KindString || tokens[0].Value != "line1\nline2" {
		t.Errorf("expected multi-line string, got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestOperators(t *testing.T) {
	src := "a.b > c.d"
	l := New(src)
	tokens := l.Lex()

	// a, ., b, >, c, ., d, EOF
	expected := []struct {
		kind  TokenKind
		value string
	}{
		{KindIdentifier, "a"},
		{KindOp, "."},
		{KindIdentifier, "b"},
		{KindOp, ">"},
		{KindIdentifier, "c"},
		{KindOp, "."},
		{KindIdentifier, "d"},
		{KindEOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}
	for i, exp := range expected {
		if tokens[i].Kind != exp.kind || tokens[i].Value != exp.value {
			t.Errorf("token %d: expected %s %q, got %s %q", i, exp.kind, exp.value, tokens[i].Kind, tokens[i].Value)
		}
	}
}

func TestManyToManyOp(t *testing.T) {
	src := "a <> b"
	l := New(src)
	tokens := l.Lex()

	if tokens[1].Kind != KindOp || tokens[1].Value != "<>" {
		t.Errorf("expected <> operator, got %s %q", tokens[1].Kind, tokens[1].Value)
	}
}

func TestComments(t *testing.T) {
	src := "// comment\nTable users {}\n"
	l := New(src)
	tokens := l.Lex()

	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}

	// Table, users, {, }, EOF
	if len(tokens) != 5 {
		t.Fatalf("expected 5 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != KindIdentifier || tokens[0].Value != "Table" {
		t.Errorf("first token should be Table, got %s %q", tokens[0].Kind, tokens[0].Value)
	}

	// comment should be in leading trivia of "Table"
	if len(tokens[0].LeadingTrivia) == 0 {
		t.Error("expected leading trivia on Table token")
	}
	foundComment := false
	for _, tv := range tokens[0].LeadingTrivia {
		if tv.Kind == KindSingleComment {
			foundComment = true
			if tv.Value != " comment" {
				t.Errorf("expected comment value ' comment', got %q", tv.Value)
			}
		}
	}
	if !foundComment {
		t.Error("expected single-line comment in leading trivia")
	}
}

func TestTriviaAttachment(t *testing.T) {
	src := "Table users {\n  id integer\n}\n"
	l := New(src)
	tokens := l.Lex()

	// "Table" trailing trivia should have a space
	if len(tokens[0].TrailingTrivia) != 1 || tokens[0].TrailingTrivia[0].Kind != KindSpace {
		t.Errorf("Table trailing trivia: expected [space], got %v", tokens[0].TrailingTrivia)
	}

	// "{" trailing trivia should have a newline
	if len(tokens[2].TrailingTrivia) != 1 || tokens[2].TrailingTrivia[0].Kind != KindNewline {
		t.Errorf("'{' trailing trivia: expected [newline], got %v", tokens[2].TrailingTrivia)
	}

	// "id" leading trivia should have spaces (indentation)
	if len(tokens[3].LeadingTrivia) != 2 {
		t.Errorf("'id' leading trivia: expected 2 spaces, got %d items", len(tokens[3].LeadingTrivia))
	}
}

func TestPositions(t *testing.T) {
	src := "Table users {\n  id integer\n}\n"
	l := New(src)
	tokens := l.Lex()

	// "Table" at line 0, col 0
	if tokens[0].StartPos.Line != 0 || tokens[0].StartPos.Column != 0 {
		t.Errorf("Table position: expected 0:0, got %d:%d", tokens[0].StartPos.Line, tokens[0].StartPos.Column)
	}

	// "id" at line 1, col 2
	if tokens[3].StartPos.Line != 1 || tokens[3].StartPos.Column != 2 {
		t.Errorf("id position: expected 1:2, got %d:%d", tokens[3].StartPos.Line, tokens[3].StartPos.Column)
	}
}

func TestSimpleTokenConversion(t *testing.T) {
	src := "Table x {}\n"
	l := New(src)
	tokens := l.Lex()
	simple := ToSimpleTokens(tokens)

	// "Table" → line 1, column 1 (1-based)
	if simple[0].Position.Line != 1 || simple[0].Position.Column != 1 {
		t.Errorf("simple position: expected 1:1, got %d:%d", simple[0].Position.Line, simple[0].Position.Column)
	}
}

func TestQuotedString(t *testing.T) {
	src := `"my-table"`
	l := New(src)
	tokens := l.Lex()

	if tokens[0].Kind != KindQuotedString || tokens[0].Value != "my-table" {
		t.Errorf("expected quoted string 'my-table', got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestFuncExpression(t *testing.T) {
	src := "`now()`"
	l := New(src)
	tokens := l.Lex()

	if tokens[0].Kind != KindFuncExpr || tokens[0].Value != "now()" {
		t.Errorf("expected func expr 'now()', got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestColorLiteral(t *testing.T) {
	src := "#FF0000"
	l := New(src)
	tokens := l.Lex()

	if tokens[0].Kind != KindColor || tokens[0].Value != "#FF0000" {
		t.Errorf("expected color '#FF0000', got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestNumericLiteral(t *testing.T) {
	src := "42 3.14"
	l := New(src)
	tokens := l.Lex()

	if tokens[0].Kind != KindNumeric || tokens[0].Value != "42" {
		t.Errorf("expected numeric '42', got %s %q", tokens[0].Kind, tokens[0].Value)
	}
	if tokens[1].Kind != KindNumeric || tokens[1].Value != "3.14" {
		t.Errorf("expected numeric '3.14', got %s %q", tokens[1].Kind, tokens[1].Value)
	}
}

func TestUnterminatedString(t *testing.T) {
	src := "'unterminated"
	l := New(src)
	_ = l.Lex()

	if len(l.Errors) == 0 {
		t.Error("expected unterminated string error")
	}
}

func TestMultiLineComment(t *testing.T) {
	src := "/* block\ncomment */ x"
	l := New(src)
	tokens := l.Lex()

	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}

	// x, EOF
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Value != "x" {
		t.Errorf("expected 'x', got %q", tokens[0].Value)
	}
}

func TestTildeOperator(t *testing.T) {
	src := "~timestamps"
	l := New(src)
	tokens := l.Lex()

	if tokens[0].Kind != KindOp || tokens[0].Value != "~" {
		t.Errorf("expected ~ operator, got %s %q", tokens[0].Kind, tokens[0].Value)
	}
	if tokens[1].Kind != KindIdentifier || tokens[1].Value != "timestamps" {
		t.Errorf("expected identifier 'timestamps', got %s %q", tokens[1].Kind, tokens[1].Value)
	}
}

func TestEmptyInput(t *testing.T) {
	l := New("")
	tokens := l.Lex()

	if len(tokens) != 1 || tokens[0].Kind != KindEOF {
		t.Errorf("expected single EOF token, got %d tokens", len(tokens))
	}
}

func TestUnexpectedCharacters(t *testing.T) {
	cases := []struct {
		src  string
		char string
		line int
		col  int
	}{
		{"@", "@", 0, 0},
		{"$", "$", 0, 0},
		{"^", "^", 0, 0},
		{"?", "?", 0, 0},
		{`\`, `\`, 0, 0},
		{"id@x", "@", 0, 2},      // position after valid identifier
		{"id\n@", "@", 1, 0},     // position on second line
	}

	for _, tc := range cases {
		l := New(tc.src)
		_ = l.Lex()

		if len(l.Errors) == 0 {
			t.Errorf("src %q: expected at least one error", tc.src)
			continue
		}
		e := l.Errors[0]
		if e.Position.Line != tc.line || e.Position.Column != tc.col {
			t.Errorf("src %q: expected error at %d:%d, got %d:%d",
				tc.src, tc.line, tc.col, e.Position.Line, e.Position.Column)
		}
		want := "unexpected character " + fmt.Sprintf("%q", tc.char)
		if e.Message != want {
			t.Errorf("src %q: expected message %q, got %q", tc.src, want, e.Message)
		}
	}
}

func TestAllUnexpectedCharsReported(t *testing.T) {
	// Three unexpected chars — all three must be reported.
	l := New("@$^")
	_ = l.Lex()

	if len(l.Errors) != 3 {
		t.Errorf("expected 3 errors for '@$^', got %d", len(l.Errors))
	}
}
