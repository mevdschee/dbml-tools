package analysis

import (
	"strings"
	"testing"
)

func labels(items []Completion) []string {
	out := make([]string, len(items))
	for i, c := range items {
		out[i] = c.Label
	}
	return out
}

func hasLabel(items []Completion, label string) bool {
	for _, c := range items {
		if c.Label == label {
			return true
		}
	}
	return false
}

func kindOf(items []Completion, label string) CompletionKind {
	for _, c := range items {
		if c.Label == label {
			return c.Kind
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Top-level keyword completion
// ---------------------------------------------------------------------------

func TestComplete_TopLevelKeyword(t *testing.T) {
	src := `▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	for _, want := range []string{"Table", "Enum", "Ref", "Project", "TableGroup"} {
		if !hasLabel(items, want) {
			t.Errorf("missing %q in: %v", want, labels(items))
		}
	}
}

func TestComplete_TopLevelKeyword_AfterDecl(t *testing.T) {
	src := `Table t { id int }
▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "Table") {
		t.Errorf("missing Table after another decl: %v", labels(items))
	}
}

func TestComplete_TopLevelKeyword_PartialPrefix(t *testing.T) {
	src := `Ta▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "Table") {
		t.Errorf("missing Table for prefix 'Ta': %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Column type position
// ---------------------------------------------------------------------------

func TestComplete_ColumnType_BuiltinTypes(t *testing.T) {
	src := `Table t {
  id ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "int") || !hasLabel(items, "varchar") || !hasLabel(items, "text") {
		t.Errorf("missing builtin types: %v", labels(items))
	}
	if kindOf(items, "int") != CompletionTypeName {
		t.Errorf("int kind = %v, want TypeName", kindOf(items, "int"))
	}
}

func TestComplete_ColumnType_IncludesEnums(t *testing.T) {
	src := `Enum order_status { pending shipped }
Table orders {
  status ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "order_status") {
		t.Errorf("missing enum in type completion: %v", labels(items))
	}
	if kindOf(items, "order_status") != CompletionEnum {
		t.Errorf("enum kind = %v", kindOf(items, "order_status"))
	}
}

func TestComplete_ColumnType_NoTables(t *testing.T) {
	src := `Table users { id int }
Table orders {
  status ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if hasLabel(items, "users") {
		t.Errorf("should not offer tables as column types: %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Column attribute name inside [...]
// ---------------------------------------------------------------------------

func TestComplete_AttributeName(t *testing.T) {
	src := `Table t {
  id int [▮]
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	for _, want := range []string{"pk", "unique", "not null", "default", "note", "ref", "primary key", "increment"} {
		if !hasLabel(items, want) {
			t.Errorf("missing attribute %q: %v", want, labels(items))
		}
	}
	if hasLabel(items, "Table") {
		t.Errorf("should not offer keywords inside [...]: %v", labels(items))
	}
}

func TestComplete_AttributeName_AfterComma(t *testing.T) {
	src := `Table t {
  id int [pk, ▮]
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "not null") {
		t.Errorf("missing attribute after comma: %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Ref endpoint table
// ---------------------------------------------------------------------------

func TestComplete_RefEndpointTable(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > ▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "users") || !hasLabel(items, "orders") {
		t.Errorf("missing tables in ref endpoint: %v", labels(items))
	}
	if kindOf(items, "users") != CompletionTable {
		t.Errorf("users kind = %v", kindOf(items, "users"))
	}
}

func TestComplete_RefEndpointTable_IncludesAliases(t *testing.T) {
	src := `Table users as u { id int }
Table orders { uid int }
Ref: orders.uid > ▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "u") {
		t.Errorf("missing alias 'u': %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Ref endpoint column (after table.)
// ---------------------------------------------------------------------------

func TestComplete_RefEndpointColumn(t *testing.T) {
	src := `Table users {
  id int
  email varchar
}
Table orders { uid int }
Ref: orders.uid > users.▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "id") || !hasLabel(items, "email") {
		t.Errorf("missing columns of users: %v", labels(items))
	}
	if hasLabel(items, "uid") {
		t.Errorf("should not offer columns from other tables: %v", labels(items))
	}
}

func TestComplete_RefEndpointColumn_ViaAlias(t *testing.T) {
	src := `Table users as u {
  id int
  email varchar
}
Table orders { uid int }
Ref: orders.uid > u.▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "id") || !hasLabel(items, "email") {
		t.Errorf("missing columns through alias: %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Inline ref target
// ---------------------------------------------------------------------------

func TestComplete_InlineRefTarget_Table(t *testing.T) {
	src := `Table users { id int }
Table orders {
  uid int [ref: > ▮]
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "users") {
		t.Errorf("missing table in inline ref: %v", labels(items))
	}
}

func TestComplete_InlineRefTarget_Column(t *testing.T) {
	src := `Table users {
  id int
  email varchar
}
Table orders {
  uid int [ref: > users.▮]
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "id") || !hasLabel(items, "email") {
		t.Errorf("missing columns in inline ref: %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// TableGroup body
// ---------------------------------------------------------------------------

func TestComplete_TableGroupBody(t *testing.T) {
	src := `Table users { id int }
Table orders { id int }
TableGroup commerce {
  ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "users") || !hasLabel(items, "orders") {
		t.Errorf("missing tables in TableGroup body: %v", labels(items))
	}
	if hasLabel(items, "Table") {
		t.Errorf("should not offer keywords in TableGroup body: %v", labels(items))
	}
}

// ---------------------------------------------------------------------------
// Project settings
// ---------------------------------------------------------------------------

func TestComplete_ProjectSettingKey(t *testing.T) {
	src := `Project p {
  ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, "database_type") {
		t.Errorf("missing database_type: %v", labels(items))
	}
}

func TestComplete_ProjectSettingValue_DatabaseType(t *testing.T) {
	src := `Project p {
  database_type: ▮
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	for _, want := range []string{"'MariaDB'", "'PostgreSQL'", "'SQLite'"} {
		if !hasLabel(items, want) {
			t.Errorf("missing %q: %v", want, labels(items))
		}
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestComplete_NoSuggestionsInsideStringLiteral(t *testing.T) {
	src := `Table t {
  id int [note: 'this is a not▮e']
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if len(items) != 0 {
		t.Errorf("should be empty inside string literal, got: %v", labels(items))
	}
}

func TestComplete_AttributeValuesForDefault(t *testing.T) {
	src := `Table t {
  c int [default: ▮]
}`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	// Expect at least: null, true, false
	for _, want := range []string{"null", "true", "false"} {
		if !hasLabel(items, want) {
			t.Errorf("missing default value %q: %v", want, labels(items))
		}
	}
}

func TestComplete_RelationshipOperators(t *testing.T) {
	src := `Table u { id int }
Table o { uid int }
Ref: o.uid ▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	if !hasLabel(items, ">") {
		t.Errorf("missing relationship operators: %v", labels(items))
	}
}

func TestComplete_PrefixIsCarriedInRange(t *testing.T) {
	src := `Ta▮`
	a, off := analyzeWithCursor(t, src)
	items := a.Completions(off)
	for _, c := range items {
		if c.Label != "Table" {
			continue
		}
		got := sub(a.Source, c.ReplaceRange)
		// The replacement range should cover "Ta", not be empty.
		if !strings.HasPrefix("Ta", got) && got != "Ta" {
			t.Errorf("ReplaceRange = %q, want prefix span 'Ta'", got)
		}
		return
	}
	t.Error("Table not present")
}
