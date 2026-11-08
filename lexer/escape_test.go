package lexer

import "testing"

func TestStringEscapes(t *testing.T) {
	l := New(`'it\'s' 'a\\' 'C:\path'`)
	tokens := l.Lex()
	if len(l.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", l.Errors)
	}
	want := []string{"it's", `a\`, `C:\path`}
	for i, w := range want {
		if tokens[i].Kind != KindString || tokens[i].Value != w {
			t.Errorf("token %d: got %s %q, want %q", i, tokens[i].Kind, tokens[i].Value, w)
		}
	}
}
