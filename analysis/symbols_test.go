package analysis

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SymbolIndex
// ---------------------------------------------------------------------------

func TestSymbolIndex_Table(t *testing.T) {
	a := Analyze(`Table users {
  id int [pk]
  email varchar(255) [not null, unique]
}`)
	tbl := a.Symbols.Tables["users"]
	if tbl == nil {
		t.Fatalf("table 'users' not indexed")
	}
	if tbl.Kind != SymTable {
		t.Errorf("kind = %v, want SymTable", tbl.Kind)
	}
	if got := sub(a.Source, tbl.NameRange); got != "users" {
		t.Errorf("NameRange covers %q, want %q", got, "users")
	}
	if len(tbl.Columns) != 2 {
		t.Fatalf("columns: got %d, want 2", len(tbl.Columns))
	}
}

func TestSymbolIndex_TableCaseInsensitive(t *testing.T) {
	a := Analyze("Table Users {\n  id int\n}")
	if a.Symbols.ResolveTable("USERS") == nil {
		t.Error("case-insensitive lookup failed")
	}
	if a.Symbols.ResolveTable("Users").Name != "Users" {
		t.Error("original casing not preserved")
	}
}

func TestSymbolIndex_Columns(t *testing.T) {
	a := Analyze(`Table users {
  id int [pk]
  email varchar(255) [not null, unique]
  note_field text [note: 'a column']
}`)
	col := a.Symbols.ResolveColumn("users", "email")
	if col == nil {
		t.Fatalf("column not found")
	}
	if col.Parent == nil || col.Parent.Name != "users" {
		t.Errorf("parent not set")
	}
	if col.TypeName != "varchar" {
		t.Errorf("type = %q, want varchar", col.TypeName)
	}
	if len(col.TypeArgs) != 1 || col.TypeArgs[0] != "255" {
		t.Errorf("type args = %v, want [255]", col.TypeArgs)
	}
	if !col.Unique || !col.NotNull {
		t.Errorf("unique=%v notnull=%v, want both true", col.Unique, col.NotNull)
	}
	if col.Qualified != "users.email" {
		t.Errorf("qualified = %q", col.Qualified)
	}
	if sub(a.Source, col.NameRange) != "email" {
		t.Errorf("NameRange = %q", sub(a.Source, col.NameRange))
	}
}

func TestSymbolIndex_ColumnNote(t *testing.T) {
	a := Analyze("Table t {\n  c int [note: 'hello']\n}")
	col := a.Symbols.ResolveColumn("t", "c")
	if col.Note != "hello" {
		t.Errorf("note = %q, want 'hello'", col.Note)
	}
}

func TestSymbolIndex_TableNote(t *testing.T) {
	a := Analyze("Table t {\n  c int\n  Note: 'table doc'\n}")
	tbl := a.Symbols.Tables["t"]
	if tbl.Note != "table doc" {
		t.Errorf("table note = %q", tbl.Note)
	}
}

func TestSymbolIndex_Alias(t *testing.T) {
	a := Analyze("Table users as u {\n  id int\n}")
	if a.Symbols.Aliases["u"] == nil {
		t.Fatal("alias 'u' not indexed")
	}
	if a.Symbols.Aliases["u"].Parent.Name != "users" {
		t.Error("alias does not point to underlying table")
	}
	if a.Symbols.ResolveTable("u").Name != "users" {
		t.Error("ResolveTable does not follow alias")
	}
}

func TestSymbolIndex_Enum(t *testing.T) {
	a := Analyze(`Enum order_status {
  pending
  shipped
  cancelled
}`)
	e := a.Symbols.Enums["order_status"]
	if e == nil {
		t.Fatal("enum not indexed")
	}
	if len(e.EnumValues) != 3 {
		t.Fatalf("values: got %d", len(e.EnumValues))
	}
	if e.EnumValues[1].Name != "shipped" {
		t.Errorf("value[1] = %q", e.EnumValues[1].Name)
	}
	if v := a.Symbols.EnumValues["order_status"]["pending"]; v == nil {
		t.Error("enum value 'pending' not indexed")
	}
}

func TestSymbolIndex_TableGroup(t *testing.T) {
	a := Analyze(`Table users { id int }
Table orders { id int }
TableGroup commerce {
  users
  orders
}`)
	tg := a.Symbols.TableGroups["commerce"]
	if tg == nil {
		t.Fatal("table group not indexed")
	}
	if tg.Kind != SymTableGroup {
		t.Errorf("kind = %v", tg.Kind)
	}
}

func TestSymbolIndex_RefName(t *testing.T) {
	a := Analyze(`Table u { id int }
Table o { uid int }
Ref my_fk: o.uid > u.id`)
	r := a.Symbols.RefNames["my_fk"]
	if r == nil {
		t.Fatal("ref name not indexed")
	}
}

func TestSymbolIndex_QuotedNames(t *testing.T) {
	a := Analyze("Table \"weird name\" {\n  id int\n}")
	if a.Symbols.ResolveTable("weird name") == nil {
		t.Error("quoted table name not indexed")
	}
}

func TestSymbolIndex_DefRangeCoversFullDecl(t *testing.T) {
	src := "Table users {\n  id int\n}"
	a := Analyze(src)
	tbl := a.Symbols.Tables["users"]
	if !strings.HasPrefix(sub(a.Source, tbl.DefRange), "Table users") {
		t.Errorf("DefRange does not start with 'Table users': %q", sub(a.Source, tbl.DefRange))
	}
	if !strings.HasSuffix(sub(a.Source, tbl.DefRange), "}") {
		t.Errorf("DefRange does not end with }: %q", sub(a.Source, tbl.DefRange))
	}
}

func TestSymbolIndex_SkipsPseudoFields(t *testing.T) {
	a := Analyze(`Table t {
  id int [pk]
  Note: 'doc'
  indexes {
    id [unique]
  }
}`)
	tbl := a.Symbols.Tables["t"]
	for _, c := range tbl.Columns {
		if strings.EqualFold(c.Name, "note") || strings.EqualFold(c.Name, "indexes") {
			t.Errorf("pseudo-field %q leaked into columns", c.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// SpanIndex
// ---------------------------------------------------------------------------

func TestSpanIndex_InnermostAtTableName(t *testing.T) {
	src := "Table users {\n  id int\n}"
	a := Analyze(src)
	// offset of 'u' in 'users'
	off := strings.Index(src, "users")
	chain := a.Spans.Innermost(off)
	if len(chain) == 0 {
		t.Fatal("no chain at table name")
	}
	leaf := a.Spans.Leaf(off)
	if leaf == nil {
		t.Fatal("Leaf returned nil")
	}
}

func TestSpanIndex_InnermostAtColumnType(t *testing.T) {
	src := "Table t {\n  id integer\n}"
	a := Analyze(src)
	off := strings.Index(src, "integer")
	leaf := a.Spans.Leaf(off)
	if leaf == nil {
		t.Fatal("no leaf at column type")
	}
}

func TestSpanIndex_OutsideAnyNode(t *testing.T) {
	src := "Table t {\n  id int\n}\n"
	a := Analyze(src)
	// trailing newline is past last token
	chain := a.Spans.Innermost(len([]rune(src)))
	_ = chain // may legitimately be empty
}
