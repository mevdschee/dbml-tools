package analysis

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Definition
// ---------------------------------------------------------------------------

func TestDefinition_TableRef(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > us▮ers.id`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal("no definition")
	}
	if sub(a.Source, *def) != "users" {
		t.Errorf("def range covers %q", sub(a.Source, *def))
	}
}

func TestDefinition_ColumnRef(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > users.i▮d`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal("no definition")
	}
	if sub(a.Source, *def) != "id" {
		t.Errorf("def covers %q", sub(a.Source, *def))
	}
}

func TestDefinition_FromInlineRef(t *testing.T) {
	src := `Table users { id int }
Table orders {
  uid int [ref: > us▮ers.id]
}`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal("no definition from inline ref")
	}
}

func TestDefinition_EnumAsType(t *testing.T) {
	src := `Enum order_status { pending shipped }
Table orders {
  status order_sta▮tus
}`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal("no definition for enum-as-type")
	}
	if sub(a.Source, *def) != "order_status" {
		t.Errorf("def covers %q", sub(a.Source, *def))
	}
}

func TestDefinition_OnDeclarationItself(t *testing.T) {
	// Cursor on a declaration: definition is the declaration's own NameRange.
	src := `Table us▮ers { id int }`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal("expected definition on decl itself")
	}
	if sub(a.Source, *def) != "users" {
		t.Errorf("got %q", sub(a.Source, *def))
	}
}

func TestDefinition_UnresolvedRef(t *testing.T) {
	src := `Table o { id int }
Ref: o.id > nonexis▮tent.id`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def != nil {
		t.Errorf("expected nil definition for unresolved, got %v", def)
	}
}

func TestDefinition_AliasUseGoesToTableDecl(t *testing.T) {
	src := `Table users as u { id int }
Table o { uid int }
Ref: o.uid > u▮.id`
	a, off := analyzeWithCursor(t, src)
	def := a.Definition(off)
	if def == nil {
		t.Fatal()
	}
	if sub(a.Source, *def) != "users" {
		t.Errorf("alias should resolve to underlying table; got %q", sub(a.Source, *def))
	}
}

// ---------------------------------------------------------------------------
// References (find all uses)
// ---------------------------------------------------------------------------

func TestReferences_Table(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > users.id
TableGroup g {
  users
}`
	a := Analyze(src)
	tbl := a.Symbols.Tables["users"]
	refs := a.ReferencesOf(tbl, false)
	// Should find the use in Ref + the use in TableGroup
	if len(refs) != 2 {
		t.Errorf("got %d refs, want 2", len(refs))
	}
}

func TestReferences_TableIncludingDecl(t *testing.T) {
	src := `Table users { id int }
Ref: users.id > users.id`
	a := Analyze(src)
	tbl := a.Symbols.Tables["users"]
	refs := a.ReferencesOf(tbl, true)
	// 1 decl + 2 use sites
	if len(refs) != 3 {
		t.Errorf("got %d refs, want 3 (incl decl)", len(refs))
	}
}

func TestReferences_Column(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > users.id`
	a := Analyze(src)
	col := a.Symbols.ResolveColumn("users", "id")
	refs := a.ReferencesOf(col, false)
	if len(refs) != 1 {
		t.Errorf("got %d refs, want 1", len(refs))
	}
}

func TestReferences_Enum(t *testing.T) {
	src := `Enum status { a b }
Table t1 { c status }
Table t2 { c status }`
	a := Analyze(src)
	e := a.Symbols.Enums["status"]
	refs := a.ReferencesOf(e, false)
	if len(refs) != 2 {
		t.Errorf("enum refs: got %d, want 2", len(refs))
	}
}

func TestReferences_ResolvesAlias(t *testing.T) {
	src := `Table users as u { id int }
Table o1 { uid int }
Table o2 { uid int }
Ref: o1.uid > users.id
Ref: o2.uid > u.id`
	a := Analyze(src)
	tbl := a.Symbols.Tables["users"]
	refs := a.ReferencesOf(tbl, false)
	if len(refs) != 2 {
		t.Errorf("alias + direct uses: got %d, want 2", len(refs))
	}
	// Check both 'users' and 'u' are among the matched site texts.
	var texts []string
	for _, r := range refs {
		texts = append(texts, sub(a.Source, r))
	}
	joined := strings.Join(texts, ",")
	if !strings.Contains(joined, "users") || !strings.Contains(joined, "u") {
		t.Errorf("expected both 'users' and 'u' uses, got %v", texts)
	}
}

func TestReferences_NilSymbol(t *testing.T) {
	a := Analyze("Table t { id int }")
	if refs := a.ReferencesOf(nil, false); refs != nil {
		t.Errorf("expected nil for nil symbol, got %v", refs)
	}
}
