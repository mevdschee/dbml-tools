package interpreter

import (
	"strings"
	"testing"

	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

func interpret(src string) (*Database, []Error) {
	l := lexer.New(src)
	tokens := l.Lex()
	p := parser.New(tokens, src)
	prog := p.Parse()
	interp := New()
	db := interp.Interpret(prog)
	return db, interp.Errors
}

func TestInterpretTable(t *testing.T) {
	db, errs := interpret("Table users {\n  id integer\n  name varchar\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(db.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(db.Tables))
	}
	tbl := db.Tables[0]
	if tbl.Name != "users" {
		t.Errorf("expected table name 'users', got %q", tbl.Name)
	}
	if len(tbl.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(tbl.Fields))
	}
	if tbl.Fields[0].Name != "id" || tbl.Fields[0].Type.TypeName != "integer" {
		t.Errorf("field 0: expected 'id integer', got %q %q", tbl.Fields[0].Name, tbl.Fields[0].Type.TypeName)
	}
	if tbl.Fields[1].Name != "name" || tbl.Fields[1].Type.TypeName != "varchar" {
		t.Errorf("field 1: expected 'name varchar', got %q %q", tbl.Fields[1].Name, tbl.Fields[1].Type.TypeName)
	}
}

func TestInterpretPrimaryKey(t *testing.T) {
	db, _ := interpret("Table t {\n  id integer [primary key]\n}\n")
	if !db.Tables[0].Fields[0].PK {
		t.Error("expected pk=true for [primary key]")
	}
}

func TestInterpretPK(t *testing.T) {
	db, _ := interpret("Table t {\n  id integer [pk]\n}\n")
	if !db.Tables[0].Fields[0].PK {
		t.Error("expected pk=true for [pk]")
	}
}

func TestInterpretFieldSettings(t *testing.T) {
	db, _ := interpret("Table t {\n  id integer [pk, not null, unique]\n}\n")
	f := db.Tables[0].Fields[0]
	if !f.PK {
		t.Error("expected pk=true")
	}
	if f.NotNull == nil || !*f.NotNull {
		t.Error("expected not_null=true")
	}
	if !f.Unique {
		t.Error("expected unique=true")
	}
}

func TestInterpretNote(t *testing.T) {
	db, _ := interpret("Table t {\n  body text [note: 'Content of the post']\n}\n")
	f := db.Tables[0].Fields[0]
	if f.Note == nil {
		t.Fatal("expected note on field")
	}
	if f.Note.Value != "Content of the post" {
		t.Errorf("expected note value 'Content of the post', got %q", f.Note.Value)
	}
}

func TestInterpretRef(t *testing.T) {
	db, _ := interpret("Table a {\n  id integer\n}\nTable b {\n  a_id integer\n}\nRef: b.a_id > a.id\n")
	if len(db.Refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(db.Refs))
	}
	ref := db.Refs[0]
	if len(ref.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(ref.Endpoints))
	}

	left := ref.Endpoints[0]
	if left.TableName != "b" || left.FieldNames[0] != "a_id" || left.Relation != "*" {
		t.Errorf("left endpoint: expected b.a_id *, got %s.%v %s", left.TableName, left.FieldNames, left.Relation)
	}

	right := ref.Endpoints[1]
	if right.TableName != "a" || right.FieldNames[0] != "id" || right.Relation != "1" {
		t.Errorf("right endpoint: expected a.id 1, got %s.%v %s", right.TableName, right.FieldNames, right.Relation)
	}
}

func TestInterpretRefOneToMany(t *testing.T) {
	db, _ := interpret("Ref: a.id < b.a_id\n")
	left := db.Refs[0].Endpoints[0]
	right := db.Refs[0].Endpoints[1]
	if left.Relation != "1" {
		t.Errorf("expected left relation '1', got %q", left.Relation)
	}
	if right.Relation != "*" {
		t.Errorf("expected right relation '*', got %q", right.Relation)
	}
}

func TestInterpretRefOneToOne(t *testing.T) {
	db, _ := interpret("Ref: a.id - b.id\n")
	left := db.Refs[0].Endpoints[0]
	right := db.Refs[0].Endpoints[1]
	if left.Relation != "1" || right.Relation != "1" {
		t.Errorf("expected both relations '1', got %q and %q", left.Relation, right.Relation)
	}
}

func TestInterpretRefManyToMany(t *testing.T) {
	db, _ := interpret("Ref: a.id <> b.id\n")
	left := db.Refs[0].Endpoints[0]
	right := db.Refs[0].Endpoints[1]
	if left.Relation != "*" || right.Relation != "*" {
		t.Errorf("expected both relations '*', got %q and %q", left.Relation, right.Relation)
	}
}

func TestInterpretEnum(t *testing.T) {
	db, _ := interpret("Enum status {\n  active\n  inactive\n  pending\n}\n")
	if len(db.Enums) != 1 {
		t.Fatalf("expected 1 enum, got %d", len(db.Enums))
	}
	e := db.Enums[0]
	if e.Name != "status" {
		t.Errorf("expected enum name 'status', got %q", e.Name)
	}
	if len(e.Values) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(e.Values))
	}
	if e.Values[0].Name != "active" {
		t.Errorf("expected first value 'active', got %q", e.Values[0].Name)
	}
}

func TestInterpretTableGroup(t *testing.T) {
	db, _ := interpret("TableGroup core {\n  users\n  posts\n}\n")
	if len(db.TableGroups) != 1 {
		t.Fatalf("expected 1 table group, got %d", len(db.TableGroups))
	}
	tg := db.TableGroups[0]
	if tg.Name != "core" {
		t.Errorf("expected name 'core', got %q", tg.Name)
	}
	if len(tg.Tables) != 2 {
		t.Fatalf("expected 2 tables in group, got %d", len(tg.Tables))
	}
}

func TestInterpretProject(t *testing.T) {
	db, _ := interpret("Project myapp {\n  database_type: 'PostgreSQL'\n}\n")
	proj, ok := db.Project.(map[string]interface{})
	if !ok {
		t.Fatal("expected project to be a map")
	}
	if proj["name"] != "myapp" {
		t.Errorf("expected project name 'myapp', got %v", proj["name"])
	}
}

func TestInterpretSampleDBML(t *testing.T) {
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

	db, errs := interpret(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	// 2 tables
	if len(db.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(db.Tables))
	}

	// users: 4 fields
	if len(db.Tables[0].Fields) != 4 {
		t.Errorf("users: expected 4 fields, got %d", len(db.Tables[0].Fields))
	}

	// posts: 6 fields
	if len(db.Tables[1].Fields) != 6 {
		t.Errorf("posts: expected 6 fields, got %d", len(db.Tables[1].Fields))
	}

	// users.id should be pk
	if !db.Tables[0].Fields[0].PK {
		t.Error("users.id should be primary key")
	}

	// posts.body should have a note
	body := db.Tables[1].Fields[2]
	if body.Note == nil || body.Note.Value != "Content of the post" {
		t.Error("posts.body should have note 'Content of the post'")
	}

	// 1 ref
	if len(db.Refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(db.Refs))
	}

	ref := db.Refs[0]
	if ref.Endpoints[0].TableName != "posts" || ref.Endpoints[0].Relation != "*" {
		t.Error("ref left endpoint should be posts.*")
	}
	if ref.Endpoints[1].TableName != "users" || ref.Endpoints[1].Relation != "1" {
		t.Error("ref right endpoint should be users.1")
	}

	// Token positions
	if db.Tables[0].Token.Start.Offset != 64 {
		t.Errorf("users table start offset: expected 64, got %d", db.Tables[0].Token.Start.Offset)
	}
	if db.Tables[0].Fields[0].Token.Start.Offset != 80 {
		t.Errorf("users.id start offset: expected 80, got %d", db.Tables[0].Fields[0].Token.Start.Offset)
	}
}

func TestMissingColumnType(t *testing.T) {
	// "titlevarchar" is a single identifier – no space between name and type
	db, errs := interpret("Table posts {\n  titlevarchar\n}\n")
	if len(errs) == 0 {
		t.Fatal("expected an error for column missing a type, got none")
	}
	found := false
	for _, e := range errs {
		if e.Message == `column "titlevarchar" is missing a type` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing-type error, got: %v", errs)
	}
	// The field is still returned (with empty type), table parsing continues
	if len(db.Tables) != 1 {
		t.Errorf("expected 1 table even on error, got %d", len(db.Tables))
	}
}

func TestInterpretUnknownDeclaration(t *testing.T) {
	// "Tableposts" is a single identifier – the missing space makes it an
	// unknown keyword.  Both the parser and interpreter should report errors.
	_, errs := interpret("Tableposts {\n  id integer\n}\n")
	if len(errs) == 0 {
		t.Fatal("expected interpreter error for unknown declaration 'Tableposts'")
	}
}

func TestInterpretEmpty(t *testing.T) {
	db, _ := interpret("")
	if len(db.Tables) != 0 {
		t.Error("expected no tables")
	}
	if len(db.Refs) != 0 {
		t.Error("expected no refs")
	}
}

func TestInterpretMultipleRefs(t *testing.T) {
	src := "Ref: a.x > b.y\nRef: c.x > d.y\n"
	db, _ := interpret(src)
	if len(db.Refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(db.Refs))
	}
}

func TestInterpretRefActions(t *testing.T) {
	tests := []struct {
		name           string
		src            string
		delete, update string
	}{
		{"unquoted", "Ref: a.b_id > b.id [delete: set null, update: cascade]\n", "set null", "cascade"},
		{"quoted", "Ref: a.b_id > b.id [delete: \"set null\"]\n", "set null", ""},
		{"mixed case", "Ref: a.b_id > b.id [delete: CASCADE]\n", "cascade", ""},
		{"none", "Ref: a.b_id > b.id\n", "", ""},
		{"named ref", "Ref fk_a_b: a.b_id > b.id [update: restrict]\n", "", "restrict"},
		{"block form", "Ref {\n  a.b_id > b.id [delete: set default]\n}\n", "set default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, errs := interpret(tt.src)
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if len(db.Refs) != 1 {
				t.Fatalf("expected 1 ref, got %d", len(db.Refs))
			}
			ref := db.Refs[0]
			if got := derefOr(ref.OnDelete); got != tt.delete {
				t.Errorf("onDelete: expected %q, got %q", tt.delete, got)
			}
			if got := derefOr(ref.OnUpdate); got != tt.update {
				t.Errorf("onUpdate: expected %q, got %q", tt.update, got)
			}
		})
	}
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestInterpretBlockRefs(t *testing.T) {
	db, errs := interpret("Ref {\n  a.b_id > b.id\n  a.c_id > c.id\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(db.Refs) != 2 {
		t.Fatalf("expected 2 refs from block body, got %d", len(db.Refs))
	}
	for i, want := range []string{"b_id", "c_id"} {
		if got := db.Refs[i].Endpoints[0].FieldNames[0]; got != want {
			t.Errorf("ref %d: expected field %q, got %q", i, want, got)
		}
	}
}

func TestInterpretCompositeRef(t *testing.T) {
	for _, src := range []string{
		"Ref: a.(x, y) > b.(p, q)\n",
		"Ref {\n  a.(x, y) > b.(p, q)\n}\n",
	} {
		db, errs := interpret(src)
		if len(errs) > 0 {
			t.Fatalf("interpret(%q): unexpected errors: %v", src, errs)
		}
		if len(db.Refs) != 1 {
			t.Fatalf("interpret(%q): expected 1 ref, got %d", src, len(db.Refs))
		}
		eps := db.Refs[0].Endpoints
		if got := strings.Join(eps[0].FieldNames, ","); got != "x,y" {
			t.Errorf("left fields: expected \"x,y\", got %q", got)
		}
		if got := strings.Join(eps[1].FieldNames, ","); got != "p,q" {
			t.Errorf("right fields: expected \"p,q\", got %q", got)
		}
	}
}

func TestInterpretSchemaQualifiedCompositeRef(t *testing.T) {
	db, errs := interpret("Ref: s.a.(x, y) > s.b.(p, q)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ep := db.Refs[0].Endpoints[0]
	if ep.SchemaName == nil || *ep.SchemaName != "s" {
		t.Errorf("expected schema \"s\", got %v", ep.SchemaName)
	}
	if ep.TableName != "a" {
		t.Errorf("expected table \"a\", got %q", ep.TableName)
	}
	if got := strings.Join(ep.FieldNames, ","); got != "x,y" {
		t.Errorf("expected fields \"x,y\", got %q", got)
	}
}

func TestInterpretSchemaQualifiedTable(t *testing.T) {
	db, errs := interpret("Table my_schema.my_table {\n  id int [pk]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	tbl := db.Tables[0]
	if tbl.SchemaName == nil || *tbl.SchemaName != "my_schema" {
		t.Errorf("expected schema \"my_schema\", got %v", tbl.SchemaName)
	}
	if tbl.Name != "my_table" {
		t.Errorf("expected table \"my_table\", got %q", tbl.Name)
	}
}

func TestInterpretQuotedSchemaQualifiedTable(t *testing.T) {
	db, errs := interpret("Table \"my schema\".\"my table\" {\n  id int [pk]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	tbl := db.Tables[0]
	if tbl.SchemaName == nil || *tbl.SchemaName != "my schema" {
		t.Errorf("expected schema \"my schema\", got %v", tbl.SchemaName)
	}
	if tbl.Name != "my table" {
		t.Errorf("expected table \"my table\", got %q", tbl.Name)
	}
}

func TestInterpretDottedTableNameIsNotSplit(t *testing.T) {
	db, errs := interpret("Table \"dotted.name\" {\n  id int [pk]\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	tbl := db.Tables[0]
	if tbl.SchemaName != nil {
		t.Errorf("expected no schema, got %q", *tbl.SchemaName)
	}
	if tbl.Name != "dotted.name" {
		t.Errorf("expected table \"dotted.name\", got %q", tbl.Name)
	}
}

func TestInterpretSchemaQualifiedEnumAndType(t *testing.T) {
	src := "Enum my_schema.status {\n  active\n}\n" +
		"Table t {\n  state my_schema.status [not null]\n}\n"
	db, errs := interpret(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	e := db.Enums[0]
	if e.SchemaName == nil || *e.SchemaName != "my_schema" {
		t.Errorf("expected enum schema \"my_schema\", got %v", e.SchemaName)
	}
	if e.Name != "status" {
		t.Errorf("expected enum name \"status\", got %q", e.Name)
	}
	col := db.Tables[0].Fields[0]
	if col.Type.SchemaName == nil || *col.Type.SchemaName != "my_schema" {
		t.Errorf("expected type schema \"my_schema\", got %v", col.Type.SchemaName)
	}
	if col.Type.TypeName != "status" {
		t.Errorf("expected type name \"status\", got %q", col.Type.TypeName)
	}
	if col.NotNull == nil || !*col.NotNull {
		t.Error("expected [not null] setting to survive the qualified type")
	}
}
