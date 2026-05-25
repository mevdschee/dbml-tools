package analysis

import (
	"strings"
	"testing"
)

func findRef(a *Analysis, kind RefSiteKind, text string) *ResolvedRef {
	for i := range a.Refs {
		if a.Refs[i].Kind == kind && a.Refs[i].SourceText == text {
			return &a.Refs[i]
		}
	}
	return nil
}

func countRefs(a *Analysis, kind RefSiteKind) int {
	n := 0
	for _, r := range a.Refs {
		if r.Kind == kind {
			n++
		}
	}
	return n
}

func TestResolveRefs_ColonForm(t *testing.T) {
	src := `Table users { id int }
Table orders { user_id int }
Ref: orders.user_id > users.id`
	a := Analyze(src)
	if got := countRefs(a, RefSiteTable); got != 2 {
		t.Errorf("table refs: got %d, want 2", got)
	}
	if got := countRefs(a, RefSiteColumn); got != 2 {
		t.Errorf("column refs: got %d, want 2", got)
	}
	if r := findRef(a, RefSiteColumn, "user_id"); r == nil || r.Target == nil {
		t.Fatalf("user_id column not resolved")
	} else if r.Target.Qualified != "orders.user_id" {
		t.Errorf("resolved to %q", r.Target.Qualified)
	}
	if r := findRef(a, RefSiteColumn, "id"); r == nil || r.Target == nil {
		t.Fatalf("id column not resolved")
	} else if r.Target.Qualified != "users.id" {
		t.Errorf("resolved to %q", r.Target.Qualified)
	}
}

func TestResolveRefs_BlockForm(t *testing.T) {
	src := `Table u { id int }
Table p { id int }
Table o { u_id int  p_id int }
Ref {
  o.u_id > u.id
  o.p_id > p.id
}`
	a := Analyze(src)
	if got := countRefs(a, RefSiteColumn); got != 4 {
		t.Errorf("column refs in block: got %d, want 4", got)
	}
}

func TestResolveRefs_UnresolvedTable(t *testing.T) {
	src := `Table o { user_id int }
Ref: o.user_id > nonexistent.id`
	a := Analyze(src)
	r := findRef(a, RefSiteTable, "nonexistent")
	if r == nil {
		t.Fatal("missing table ref site")
	}
	if r.Target != nil {
		t.Errorf("expected unresolved, got target %q", r.Target.Qualified)
	}
}

func TestResolveRefs_UnresolvedColumn(t *testing.T) {
	src := `Table u { id int }
Table o { user_id int }
Ref: o.user_id > u.nonexistent_col`
	a := Analyze(src)
	for _, r := range a.Refs {
		if r.Kind == RefSiteColumn && r.SourceText == "nonexistent_col" {
			if r.Target != nil {
				t.Error("expected unresolved column")
			}
			return
		}
	}
	t.Error("missing column ref site")
}

func TestResolveRefs_ViaAlias(t *testing.T) {
	src := `Table users as u { id int }
Table orders { user_id int }
Ref: orders.user_id > u.id`
	a := Analyze(src)
	r := findRef(a, RefSiteTable, "u")
	if r == nil || r.Target == nil {
		t.Fatal("alias ref not resolved")
	}
	if r.Target.Name != "users" {
		t.Errorf("alias resolved to %q, want 'users'", r.Target.Name)
	}
	rc := findRef(a, RefSiteColumn, "id")
	if rc == nil || rc.Target == nil || rc.Target.Qualified != "users.id" {
		t.Errorf("column via alias not resolved correctly: %#v", rc)
	}
}

func TestResolveRefs_CompositeTuple(t *testing.T) {
	// Composite tuple refs (a.(x, y) > b.(x, y)) are in the DBML grammar
	// but not currently supported by the parser. When/if the parser learns
	// to handle them, this test should be unskipped.
	t.Skip("parser does not yet support composite tuple ref endpoints")
}

func TestResolveRefs_InlineRef(t *testing.T) {
	src := `Table users { id int }
Table orders {
  user_id int [ref: > users.id]
}`
	a := Analyze(src)
	r := findRef(a, RefSiteTable, "users")
	if r == nil || r.Target == nil {
		t.Fatal("inline ref table not resolved")
	}
	rc := findRef(a, RefSiteColumn, "id")
	if rc == nil || rc.Target == nil {
		t.Fatal("inline ref column not resolved")
	}
}

func TestResolveRefs_EnumAsType(t *testing.T) {
	src := `Enum order_status { pending shipped }
Table orders {
  status order_status
}`
	a := Analyze(src)
	r := findRef(a, RefSiteEnumType, "order_status")
	if r == nil {
		t.Fatal("enum-type ref not collected")
	}
	if r.Target == nil || r.Target.Name != "order_status" {
		t.Errorf("enum-type ref unresolved")
	}
}

func TestResolveRefs_EnumAsType_NoFalsePositiveForBuiltin(t *testing.T) {
	src := `Table t { c int }`
	a := Analyze(src)
	for _, r := range a.Refs {
		if r.Kind == RefSiteEnumType {
			t.Errorf("unexpected enum-type ref: %#v", r)
		}
	}
}

func TestResolveRefs_TableGroup(t *testing.T) {
	src := `Table users { id int }
Table orders { id int }
TableGroup commerce {
  users
  orders
}`
	a := Analyze(src)
	if got := countRefs(a, RefSiteTableInGroup); got != 2 {
		t.Errorf("table-group refs: got %d, want 2", got)
	}
	for _, r := range a.Refs {
		if r.Kind == RefSiteTableInGroup && r.Target == nil {
			t.Errorf("table-group ref unresolved: %q", r.SourceText)
		}
	}
}

func TestResolveRefs_SiteRangeIsIdentifierOnly(t *testing.T) {
	src := `Table users { id int }
Table orders { user_id int }
Ref: orders.user_id > users.id`
	a := Analyze(src)
	r := findRef(a, RefSiteColumn, "user_id")
	if r == nil {
		t.Fatal()
	}
	got := sub(a.Source, r.SiteRange)
	if got != "user_id" {
		t.Errorf("site range covers %q, want exactly 'user_id'", got)
	}
}

func TestRefAt(t *testing.T) {
	src := `Table users { id int }
Table orders { user_id int }
Ref: orders.user_id > users.id`
	a := Analyze(src)
	// offset at the 'u' of "user_id" in the Ref line
	idx := strings.LastIndex(src, "user_id")
	r := a.RefAt(idx)
	if r == nil {
		t.Fatal("RefAt returned nil")
	}
	if r.SourceText != "user_id" {
		t.Errorf("got %q", r.SourceText)
	}
}
