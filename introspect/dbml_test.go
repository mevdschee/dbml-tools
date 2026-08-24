package introspect

import (
	"strings"
	"testing"

	"github.com/mevdschee/dbml-tools/generator"
	"github.com/mevdschee/dbml-tools/interpreter"
	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

// roundTripSchema mirrors the shape that broke the todbml -> check -> tosql
// round trip: an anonymous Project block plus FKs with referential actions.
func roundTripSchema() *DBSchema {
	col := func(name, typ string, pk bool) *Column {
		return &Column{Name: name, Type: typ, IsPrimaryKey: pk, IsNullable: !pk}
	}
	return &DBSchema{
		Tables: []*Table{
			{Name: "mailbox", Columns: []*Column{col("id", "bigint(20)", true)}},
			{Name: "customer", Columns: []*Column{
				col("id", "bigint(20)", true),
				col("default_mailbox_id", "bigint(20)", false),
				col("owner_id", "bigint(20)", false),
			}},
		},
		FKs: []*ForeignKey{
			{
				TableName: "customer", Columns: []string{"default_mailbox_id"},
				RefTable: "mailbox", RefColumns: []string{"id"},
				OnDelete: "SET NULL", OnUpdate: "CASCADE",
			},
			{
				TableName: "customer", Columns: []string{"owner_id"},
				RefTable: "mailbox", RefColumns: []string{"id"},
				OnDelete: "NO ACTION",
			},
			{
				// Multi-column FKs are emitted as table.(col, col) endpoints.
				TableName: "customer", Columns: []string{"id", "owner_id"},
				RefTable: "mailbox", RefColumns: []string{"id", "tenant_id"},
				OnDelete: "CASCADE",
			},
		},
	}
}

func TestGeneratedDBMLParses(t *testing.T) {
	dbml := GenerateDBML(roundTripSchema(), "MariaDB", false)

	l := lexer.New(dbml)
	p := parser.New(l.Lex(), dbml)
	prog := p.Parse()
	if len(p.Errors) > 0 {
		t.Fatalf("generated DBML does not parse: %v\n%s", p.Errors, dbml)
	}

	interp := interpreter.New()
	db := interp.Interpret(prog)
	if len(interp.Errors) > 0 {
		t.Fatalf("generated DBML does not interpret: %v\n%s", interp.Errors, dbml)
	}
	if len(db.Refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(db.Refs))
	}
}

func TestGeneratedDBMLKeepsRefActions(t *testing.T) {
	dbml := GenerateDBML(roundTripSchema(), "MariaDB", false)

	l := lexer.New(dbml)
	p := parser.New(l.Lex(), dbml)
	db := interpreter.New().Interpret(p.Parse())
	sql := generator.Dump(db, generator.MariaDB)

	want := []string{
		"FOREIGN KEY (`default_mailbox_id`) REFERENCES `mailbox` (`id`) ON DELETE SET NULL ON UPDATE CASCADE;",
		"FOREIGN KEY (`id`, `owner_id`) REFERENCES `mailbox` (`id`, `tenant_id`) ON DELETE CASCADE;",
	}
	for _, w := range want {
		if !strings.Contains(sql, w) {
			t.Errorf("lost in round trip; missing %q in:\n%s", w, sql)
		}
	}
}
