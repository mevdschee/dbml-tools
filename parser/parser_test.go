package parser

import (
	"testing"

	"github.com/mevdschee/dbml-tools/lexer"
)

func parse(src string) (*ProgramNode, []Error) {
	l := lexer.New(src)
	tokens := l.Lex()
	p := New(tokens, src)
	prog := p.Parse()
	return prog, p.Errors
}

func TestParseTable(t *testing.T) {
	prog, errs := parse("Table users {\n  id integer\n  name varchar\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(prog.Body))
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.Type.Value != "Table" {
		t.Errorf("expected type 'Table', got %q", decl.Type.Value)
	}

	name := decl.Name.(*PrimaryExprNode).Expr.(*VariableNode)
	if name.Token.Value != "users" {
		t.Errorf("expected name 'users', got %q", name.Token.Value)
	}

	block := decl.Body.(*BlockExprNode)
	if len(block.Body) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(block.Body))
	}

	// First field: id integer
	field1 := block.Body[0].(*FuncAppNode)
	callee1 := field1.Callee.(*PrimaryExprNode).Expr.(*VariableNode)
	if callee1.Token.Value != "id" {
		t.Errorf("field 1 name: expected 'id', got %q", callee1.Token.Value)
	}
	if len(field1.Args) != 1 {
		t.Fatalf("field 1: expected 1 arg, got %d", len(field1.Args))
	}
}

func TestParseTableWithSettings(t *testing.T) {
	prog, errs := parse("Table users {\n  id integer [primary key]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	block := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	field := block.Body[0].(*FuncAppNode)

	if len(field.Args) != 2 {
		t.Fatalf("expected 2 args (type + settings), got %d", len(field.Args))
	}

	// Second arg should be a list expression
	list := field.Args[1].(*ListExprNode)
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 setting, got %d", len(list.Items))
	}

	attr := list.Items[0].(*AttributeNode)
	stream := attr.Name.(*IdentStreamNode)
	if len(stream.Tokens) != 2 {
		t.Fatalf("expected 2 tokens in identifier stream, got %d", len(stream.Tokens))
	}
	if stream.Tokens[0].Value != "primary" || stream.Tokens[1].Value != "key" {
		t.Errorf("expected 'primary key', got %q %q", stream.Tokens[0].Value, stream.Tokens[1].Value)
	}
}

func TestParseTableWithNote(t *testing.T) {
	prog, errs := parse("Table users {\n  body text [note: 'Content']\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	block := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	field := block.Body[0].(*FuncAppNode)

	list := field.Args[1].(*ListExprNode)
	attr := list.Items[0].(*AttributeNode)

	// name should be "note"
	noteName := attr.Name.(*PrimaryExprNode).Expr.(*VariableNode)
	if noteName.Token.Value != "note" {
		t.Errorf("expected attribute name 'note', got %q", noteName.Token.Value)
	}

	// colon should be present
	if attr.ColonTok == nil {
		t.Fatal("expected colon in attribute")
	}

	// value should be string literal 'Content'
	lit := attr.Value.(*PrimaryExprNode).Expr.(*LiteralNode)
	if lit.Token.Value != "Content" {
		t.Errorf("expected note value 'Content', got %q", lit.Token.Value)
	}
}

func TestParseRef(t *testing.T) {
	prog, errs := parse("Ref: posts.user_id > users.id\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(prog.Body))
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.Type.Value != "Ref" {
		t.Errorf("expected Ref, got %q", decl.Type.Value)
	}
	if decl.Colon == nil {
		t.Fatal("expected colon in Ref declaration")
	}

	// Body should be an infix expression with >
	infix := decl.Body.(*InfixExprNode)
	if infix.Op.Value != ">" {
		t.Errorf("expected > operator, got %q", infix.Op.Value)
	}

	// Left: posts.user_id
	leftInfix := infix.Left.(*InfixExprNode)
	if leftInfix.Op.Value != "." {
		t.Errorf("expected . operator on left, got %q", leftInfix.Op.Value)
	}

	// Right: users.id
	rightInfix := infix.Right.(*InfixExprNode)
	if rightInfix.Op.Value != "." {
		t.Errorf("expected . operator on right, got %q", rightInfix.Op.Value)
	}
}

func TestParseEnum(t *testing.T) {
	prog, errs := parse("Enum status {\n  active\n  inactive\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.Type.Value != "Enum" {
		t.Errorf("expected Enum, got %q", decl.Type.Value)
	}

	block := decl.Body.(*BlockExprNode)
	if len(block.Body) != 2 {
		t.Fatalf("expected 2 enum values, got %d", len(block.Body))
	}
}

func TestParseAlias(t *testing.T) {
	prog, errs := parse("Table users as U {\n  id integer\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.As == nil {
		t.Fatal("expected 'as' keyword")
	}
	if decl.As.Value != "as" {
		t.Errorf("expected 'as', got %q", decl.As.Value)
	}
	alias := decl.Alias.(*PrimaryExprNode).Expr.(*VariableNode)
	if alias.Token.Value != "U" {
		t.Errorf("expected alias 'U', got %q", alias.Token.Value)
	}
}

func TestParseMultipleDeclarations(t *testing.T) {
	src := `Table users {
  id integer
}

Table posts {
  id integer
}

Ref: posts.user_id > users.id
`
	prog, errs := parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 declarations, got %d", len(prog.Body))
	}
}

func TestParseProject(t *testing.T) {
	prog, errs := parse("Project myapp {\n  database_type: 'PostgreSQL'\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.Type.Value != "Project" {
		t.Errorf("expected Project, got %q", decl.Type.Value)
	}
}

func TestParseTableGroup(t *testing.T) {
	prog, errs := parse("TableGroup core {\n  users\n  posts\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	decl := prog.Body[0].(*ElementDeclNode)
	if decl.Type.Value != "TableGroup" {
		t.Errorf("expected TableGroup, got %q", decl.Type.Value)
	}

	block := decl.Body.(*BlockExprNode)
	if len(block.Body) != 2 {
		t.Fatalf("expected 2 table refs, got %d", len(block.Body))
	}
}

func TestParseSampleDBML(t *testing.T) {
	src := `// Welcome to DBML Playground!
// Try editing this DBML schema

Table users {
  id integer [primary key]
  username varchar
  role varchar
  created_at timestamp
}

Table posts {
  id integer [primary key]
  title varchar
  body text [note: 'Content of the post']
  user_id integer
  status varchar
  created_at timestamp
}

Ref: posts.user_id > users.id // many-to-one`

	prog, errs := parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}

	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 declarations (2 tables + 1 ref), got %d", len(prog.Body))
	}

	// First table: users with 4 fields
	users := prog.Body[0].(*ElementDeclNode)
	usersBlock := users.Body.(*BlockExprNode)
	if len(usersBlock.Body) != 4 {
		t.Errorf("users: expected 4 fields, got %d", len(usersBlock.Body))
	}

	// Second table: posts with 6 fields
	posts := prog.Body[1].(*ElementDeclNode)
	postsBlock := posts.Body.(*BlockExprNode)
	if len(postsBlock.Body) != 6 {
		t.Errorf("posts: expected 6 fields, got %d", len(postsBlock.Body))
	}

	// Ref declaration
	ref := prog.Body[2].(*ElementDeclNode)
	if ref.Colon == nil {
		t.Error("ref: expected colon")
	}
	if _, ok := ref.Body.(*InfixExprNode); !ok {
		t.Errorf("ref body: expected InfixExprNode, got %T", ref.Body)
	}
}

func TestToJSON(t *testing.T) {
	prog, errs := parse("Table users {\n  id integer\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	j := prog.ToJSON()
	m, ok := j.(map[string]interface{})
	if !ok {
		t.Fatal("expected map from ToJSON")
	}
	if m["kind"] != "<program>" {
		t.Errorf("expected kind <program>, got %v", m["kind"])
	}
	body := m["body"].([]interface{})
	if len(body) != 1 {
		t.Fatalf("expected 1 body element, got %d", len(body))
	}
	decl := body[0].(map[string]interface{})
	if decl["kind"] != "<element-declaration>" {
		t.Errorf("expected kind <element-declaration>, got %v", decl["kind"])
	}
}

func TestParseError(t *testing.T) {
	// Starting with brace is not a valid declaration keyword.
	_, errs := parse("{ bad }")
	if len(errs) == 0 {
		t.Error("expected parse error for '{ bad }'")
	}
}

func TestParseUnknownKeyword(t *testing.T) {
	_, errs := parse("Tableposts {\n  id integer\n}\n")
	if len(errs) == 0 {
		t.Fatal("expected error for unknown keyword 'Tableposts'")
	}
	found := false
	for _, e := range errs {
		if e.Message == `unknown declaration keyword "Tableposts"; expected Table, Enum, Ref, Note, Project, TableGroup, or TablePartial` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown-keyword error, got: %v", errs)
	}
}

func TestParseTableRequiresName(t *testing.T) {
	_, errs := parse("Table {\n  id integer\n}\n")
	if len(errs) == 0 {
		t.Fatal("expected error for Table without name")
	}
	found := false
	for _, e := range errs {
		if e.Message == "Table declaration requires a name" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-name error, got: %v", errs)
	}
}

func TestParseTableRequiresBody(t *testing.T) {
	_, errs := parse("Table users\n")
	if len(errs) == 0 {
		t.Fatal("expected error for Table without body")
	}
	found := false
	for _, e := range errs {
		if e.Message == "Table declaration requires a body" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-body error, got: %v", errs)
	}
}

func TestParseRefRequiresBody(t *testing.T) {
	_, errs := parse("Ref myref\n")
	if len(errs) == 0 {
		t.Fatal("expected error for Ref without body")
	}
	found := false
	for _, e := range errs {
		if e.Message == "Ref declaration requires a body" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-body error, got: %v", errs)
	}
}

func TestParseCommaSettings(t *testing.T) {
	prog, errs := parse("Table t {\n  id integer [pk, not null, unique]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	block := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	field := block.Body[0].(*FuncAppNode)
	list := field.Args[1].(*ListExprNode)

	if len(list.Items) != 3 {
		t.Fatalf("expected 3 settings, got %d", len(list.Items))
	}
}

func TestParseAnonymousProject(t *testing.T) {
	_, errs := parse("Project {\n  database_type: 'MariaDB'\n}\n")
	if len(errs) > 0 {
		t.Fatalf("anonymous Project should be accepted: %v", errs)
	}
}

func TestParseNamedProject(t *testing.T) {
	prog, errs := parse("Project webmail {\n  database_type: 'MariaDB'\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	decl := prog.Body[0].(*ElementDeclNode)
	if got := decl.Name.(*PrimaryExprNode).Expr.(*VariableNode).Token.Value; got != "webmail" {
		t.Errorf("expected name 'webmail', got %q", got)
	}
}

func TestParseMultiWordRefAction(t *testing.T) {
	for _, src := range []string{
		"Ref: a.b_id > b.id [delete: set null]\n",
		"Ref: a.b_id > b.id [delete: no action, update: set default]\n",
		"Ref: a.b_id > b.id [delete: \"set null\"]\n",
	} {
		if _, errs := parse(src); len(errs) > 0 {
			t.Errorf("parse(%q): unexpected errors: %v", src, errs)
		}
	}
}

func TestParseInlineRefKeepsDotValue(t *testing.T) {
	// Gathering multi-word values must not swallow the dotted target of a ref.
	_, errs := parse("Table a {\n  b_id integer [ref: > b.id, not null]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

// refExpr digs the relationship tree out of a Ref declaration body, which is
// either a bare expression (colon form) or one line of a block.
func refExpr(t *testing.T, prog *ProgramNode) *InfixExprNode {
	t.Helper()
	decl := prog.Body[0].(*ElementDeclNode)
	body := decl.Body
	if blk, ok := body.(*BlockExprNode); ok {
		if len(blk.Body) != 1 {
			t.Fatalf("expected 1 ref line, got %d", len(blk.Body))
		}
		body = blk.Body[0]
	}
	if fa, ok := body.(*FuncAppNode); ok {
		body = fa.Callee // settings wrapper
	}
	infix, ok := body.(*InfixExprNode)
	if !ok {
		t.Fatalf("expected InfixExprNode, got %T", body)
	}
	return infix
}

func TestParseBlockRefBuildsTree(t *testing.T) {
	prog, errs := parse("Ref {\n  a.b_id > b.id\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := refExpr(t, prog).Op.Value; got != ">" {
		t.Errorf("expected relation '>', got %q", got)
	}
}

func TestParseBlockRefWithSettings(t *testing.T) {
	prog, errs := parse("Ref {\n  a.b_id > b.id [delete: set null]\n  a.c_id > c.id\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	blk := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	if len(blk.Body) != 2 {
		t.Fatalf("expected 2 ref lines, got %d", len(blk.Body))
	}
	if _, ok := blk.Body[0].(*FuncAppNode); !ok {
		t.Errorf("expected settings-carrying FuncAppNode, got %T", blk.Body[0])
	}
}

func TestParseCompositeRefEndpoint(t *testing.T) {
	for _, src := range []string{
		"Ref: a.(x, y) > b.(p, q)\n",
		"Ref {\n  a.(x, y) > b.(p, q)\n}\n",
	} {
		prog, errs := parse(src)
		if len(errs) > 0 {
			t.Fatalf("parse(%q): unexpected errors: %v", src, errs)
		}
		infix := refExpr(t, prog)
		for _, side := range []Node{infix.Left, infix.Right} {
			ep := side.(*InfixExprNode)
			tup, ok := ep.Right.(*TupleExprNode)
			if !ok {
				t.Fatalf("expected TupleExprNode endpoint, got %T", ep.Right)
			}
			if len(tup.Items) != 2 {
				t.Errorf("expected 2 columns, got %d", len(tup.Items))
			}
		}
	}
}

func TestParseTableBlockStillFuncApp(t *testing.T) {
	// Only Ref blocks change; table bodies keep the function-application form.
	prog, errs := parse("Table t {\n  id integer [pk]\n  indexes {\n    (id) [name: 'i']\n  }\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	blk := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	if _, ok := blk.Body[0].(*FuncAppNode); !ok {
		t.Errorf("expected column as FuncAppNode, got %T", blk.Body[0])
	}
}
