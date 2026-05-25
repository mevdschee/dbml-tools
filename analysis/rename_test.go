package analysis

import (
	"sort"
	"strings"
	"testing"
)

// applyEdits returns src after applying edits. Edits are in any order.
func applyEdits(src string, edits []TextEdit) string {
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].Range.Start > edits[j].Range.Start
	})
	runes := []rune(src)
	for _, e := range edits {
		before := string(runes[:e.Range.Start])
		after := string(runes[e.Range.End:])
		runes = []rune(before + e.NewText + after)
	}
	return string(runes)
}

// ---------------------------------------------------------------------------
// prepareRename
// ---------------------------------------------------------------------------

func TestPrepareRename_Table(t *testing.T) {
	src := `Table us▮ers { id int }`
	a, off := analyzeWithCursor(t, src)
	pr, err := a.PrepareRename(off)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if sub(a.Source, pr.Range) != "users" {
		t.Errorf("range covers %q", sub(a.Source, pr.Range))
	}
	if pr.Placeholder != "users" {
		t.Errorf("placeholder = %q", pr.Placeholder)
	}
}

func TestPrepareRename_OnRefUseSite(t *testing.T) {
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > us▮ers.id`
	a, off := analyzeWithCursor(t, src)
	pr, err := a.PrepareRename(off)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if sub(a.Source, pr.Range) != "users" {
		t.Errorf("range covers %q", sub(a.Source, pr.Range))
	}
}

func TestPrepareRename_OnKeyword_Fails(t *testing.T) {
	src := `Ta▮ble t { id int }`
	a, off := analyzeWithCursor(t, src)
	_, err := a.PrepareRename(off)
	if err == nil {
		t.Error("expected error renaming a keyword")
	}
}

func TestPrepareRename_OnBuiltinType_Fails(t *testing.T) {
	src := `Table t {
  id i▮nt
}`
	a, off := analyzeWithCursor(t, src)
	_, err := a.PrepareRename(off)
	if err == nil {
		t.Error("expected error renaming builtin type")
	}
}

func TestPrepareRename_OnUnresolvedRef_Fails(t *testing.T) {
	src := `Table o { id int }
Ref: o.id > unkno▮wn.id`
	a, off := analyzeWithCursor(t, src)
	_, err := a.PrepareRename(off)
	if err == nil {
		t.Error("expected error renaming unresolved")
	}
}

// ---------------------------------------------------------------------------
// rename
// ---------------------------------------------------------------------------

func TestRename_Table_UpdatesAllUses(t *testing.T) {
	src := `Table us▮ers { id int }
Table orders { uid int }
Ref: orders.uid > users.id
TableGroup g {
  users
}`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "people")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	for _, want := range []string{"Table people {", "orders.uid > people.id", "  people\n"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in result:\n%s", want, result)
		}
	}
	if strings.Contains(result, "users") {
		t.Errorf("'users' still present:\n%s", result)
	}
}

func TestRename_Column_UpdatesAllRefs(t *testing.T) {
	src := `Table users { i▮d int }
Table orders { uid int }
Ref: orders.uid > users.id`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "user_id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "Table users { user_id int }") {
		t.Errorf("decl not renamed:\n%s", result)
	}
	if !strings.Contains(result, "users.user_id") {
		t.Errorf("ref not renamed:\n%s", result)
	}
}

func TestRename_Column_DoesNotAffectOtherTables(t *testing.T) {
	src := `Table t1 { i▮d int }
Table t2 { id int }`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "k")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "Table t1 { k int }") {
		t.Errorf("t1.id not renamed:\n%s", result)
	}
	if !strings.Contains(result, "Table t2 { id int }") {
		t.Errorf("t2.id was incorrectly renamed:\n%s", result)
	}
}

func TestRename_Enum_UpdatesTypeReferences(t *testing.T) {
	src := `Enum order_sta▮tus { pending shipped }
Table orders {
  status order_status
}
Table archives {
  status order_status
}`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "OrderState")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "Enum OrderState {") {
		t.Errorf("decl not renamed:\n%s", result)
	}
	count := strings.Count(result, "OrderState")
	if count != 3 {
		t.Errorf("expected 3 occurrences of OrderState, got %d:\n%s", count, result)
	}
}

func TestRename_RefThroughAlias(t *testing.T) {
	src := `Table users as u { i▮d int }
Table o { uid int }
Ref: o.uid > u.id`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "uid")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "u.uid") {
		t.Errorf("alias-side ref not renamed:\n%s", result)
	}
}

func TestRename_TableGroup(t *testing.T) {
	src := `Table u { id int }
TableGroup com▮merce {
  u
}`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "shop")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "TableGroup shop") {
		t.Errorf("group not renamed:\n%s", result)
	}
}

func TestRename_InvalidIdentifier_Errors(t *testing.T) {
	src := `Table us▮ers { id int }`
	a, off := analyzeWithCursor(t, src)
	if _, err := a.Rename(off, ""); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := a.Rename(off, "9bad"); err == nil {
		t.Error("expected error for digit-prefix name")
	}
	if _, err := a.Rename(off, "has space"); err == nil {
		t.Error("expected error for whitespace in name")
	}
}

func TestRename_QuotedIfNeeded(t *testing.T) {
	// Renaming to a name that needs quoting wraps it.
	src := `Table us▮ers { id int }`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, `weird-name`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, `"weird-name"`) {
		t.Errorf("expected quoted form:\n%s", result)
	}
}

func TestRename_InlineRef(t *testing.T) {
	src := `Table users { i▮d int }
Table orders { uid int [ref: > users.id] }`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "user_id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "users.user_id]") {
		t.Errorf("inline ref not renamed:\n%s", result)
	}
}

func TestRename_FromUseSite(t *testing.T) {
	// Rename when cursor is on a ref use, not the declaration.
	src := `Table users { id int }
Table orders { uid int }
Ref: orders.uid > us▮ers.id`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "people")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	result := applyEdits(a.Source, edits)
	if !strings.Contains(result, "Table people {") {
		t.Errorf("decl not renamed from use site:\n%s", result)
	}
}

func TestRename_EditsAreNonOverlapping(t *testing.T) {
	src := `Table us▮ers { id int }
Ref: users.id > users.id`
	a, off := analyzeWithCursor(t, src)
	edits, err := a.Rename(off, "p")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Verify no overlapping ranges.
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Range.Start < edits[j].Range.Start })
	for i := 1; i < len(edits); i++ {
		if edits[i].Range.Start < edits[i-1].Range.End {
			t.Errorf("overlapping edits: %v and %v", edits[i-1], edits[i])
		}
	}
}
