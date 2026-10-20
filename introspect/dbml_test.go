package introspect

import (
	"database/sql"
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

func TestSQLiteIndexKeys(t *testing.T) {
	cases := map[string][]string{
		`CREATE INDEX "i" ON "t" ("a", "b")`:                        {`"a"`, `"b"`},
		`CREATE INDEX "i" ON "t" ("org_id", (lower(name)))`:         {`"org_id"`, `lower(name)`},
		`CREATE UNIQUE INDEX "x(y" ON "t,u" ((a + b) * 2 DESC, c)`:  {`(a + b) * 2`, `c`},
		`CREATE INDEX i ON t (substr(a, 1, 2)) WHERE b IS NOT NULL`: {`substr(a, 1, 2)`},
		`CREATE INDEX "i" ON "t" ((lower(name)) ASC, ("a"))`:        {`lower(name)`, `"a"`},
	}
	for stmt, want := range cases {
		got := sqliteIndexKeys(stmt)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("%s:\n  got  %q\n  want %q", stmt, got, want)
		}
	}
}

func TestGeneratedDBMLKeepsMixedIndexKeys(t *testing.T) {
	idx := &Index{Name: "i", IsUnique: true, Type: "btree"}
	idx.addKey("org_id", false)
	idx.addKey("lower(name::text)", true)
	schema := &DBSchema{Tables: []*Table{{
		Name:    "t",
		Columns: []*Column{{Name: "org_id", Type: "int"}, {Name: "name", Type: "text"}},
		Indexes: []*Index{idx},
	}}}
	dbml := GenerateDBML(schema, "PostgreSQL", false)
	want := "(\"org_id\", `lower(name::text)`) [unique, name: \"i\"]"
	if !strings.Contains(dbml, want) {
		t.Errorf("expected %q in:\n%s", want, dbml)
	}
}

func TestPgColumnTypeKeepsTimePrecision(t *testing.T) {
	null := sql.NullInt64{}
	prec := func(n int64) sql.NullInt64 { return sql.NullInt64{Int64: n, Valid: true} }
	for _, tc := range []struct {
		dataType  string
		precision sql.NullInt64
		want      string
	}{
		{"timestamp with time zone", prec(3), "timestamptz(3)"},
		{"timestamp with time zone", prec(6), "timestamptz"},
		{"timestamp without time zone", prec(0), "timestamp(0)"},
		{"time with time zone", prec(2), "timetz(2)"},
		{"integer", null, "integer"},
	} {
		if got := pgColumnType(tc.dataType, null, null, null, tc.precision, "pg_catalog", ""); got != tc.want {
			t.Errorf("%s with precision %v: got %q, want %q", tc.dataType, tc.precision, got, tc.want)
		}
	}
}
