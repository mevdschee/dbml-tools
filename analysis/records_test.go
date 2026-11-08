package analysis

import "testing"

func TestResolveRefs_Records(t *testing.T) {
	src := `Table s.users {
  id int
  name varchar
  records (id) {
    1
  }
}
Records s.users(id, name) {
  1, 'Bob'
}`
	a := Analyze(src)
	if r := findRef(a, RefSiteTable, "users"); r == nil || r.Target == nil {
		t.Fatal("records table not resolved")
	}
	if got := countRefs(a, RefSiteColumn); got != 3 {
		t.Errorf("column refs: got %d, want 3", got)
	}
	if r := findRef(a, RefSiteColumn, "name"); r == nil || r.Target == nil || r.Target.Qualified != "users.name" {
		t.Errorf("records column not resolved: %+v", r)
	}
}
