package interpreter

import (
	"reflect"
	"strings"
	"testing"
)

func TestInterpretTopLevelRecords(t *testing.T) {
	// Records may come before the table it fills.
	src := `Records users(id, username, role) {
  0, 'Alice', 'admin'
  1, 'Bob', 'moderator'
}

Table users {
  id integer [primary key]
  username varchar
  role varchar
}
`
	db, errs := interpret(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	rec := db.Tables[0].Records
	if rec == nil {
		t.Fatal("expected records on users")
	}
	if want := []string{"id", "username", "role"}; !reflect.DeepEqual(rec.Columns, want) {
		t.Errorf("columns: got %v, want %v", rec.Columns, want)
	}
	want := [][]string{{"0", "Alice", "admin"}, {"1", "Bob", "moderator"}}
	if !reflect.DeepEqual(rec.Rows, want) {
		t.Errorf("rows: got %v, want %v", rec.Rows, want)
	}
}

func TestInterpretTopLevelRecordsMatchesSchemaAndAlias(t *testing.T) {
	src := `Table a.users { id int }
Table b.users as U { id int }
Records b.users(id) {
  1
}
Records a.users(id) {
  2
}
`
	db, errs := interpret(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got := db.Tables[0].Records.Rows[0][0]; got != "2" {
		t.Errorf("a.users: got %q", got)
	}
	if got := db.Tables[1].Records.Rows[0][0]; got != "1" {
		t.Errorf("b.users: got %q", got)
	}

	db, errs = interpret("Table users as U { id int }\nRecords U(id) {\n  1\n}\n")
	if len(errs) > 0 || db.Tables[0].Records == nil {
		t.Errorf("records via alias not attached: %v", errs)
	}
}

func TestInterpretRecordsWithoutColumnList(t *testing.T) {
	src := `Enum status {
  active
}
Table t {
  records {
    1, -5, status.active, , null
  }
  a int
  b int
  c status
  d int
  e int
}
`
	db, errs := interpret(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	rec := db.Tables[0].Records
	if want := []string{"a", "b", "c", "d", "e"}; !reflect.DeepEqual(rec.Columns, want) {
		t.Errorf("columns: got %v, want %v", rec.Columns, want)
	}
	if want := []string{"1", "-5", "active", "null", "null"}; !reflect.DeepEqual(rec.Rows[0], want) {
		t.Errorf("row: got %v, want %v", rec.Rows[0], want)
	}
}

func TestInterpretRecordsErrors(t *testing.T) {
	cases := map[string]string{
		"unknown table":   "Records nope(id) {\n  1\n}\n",
		"already has":     "Table t {\n  id int\n  records (id) {\n    1\n  }\n}\nRecords t(id) {\n  2\n}\n",
		"has 2 values":    "Table t { id int }\nRecords t(id) {\n  1, 2\n}\n",
		"has 1 values, e": "Table t {\n  a int\n  b int\n  records {\n    1\n  }\n}\n",
	}
	for want, src := range cases {
		_, errs := interpret(src)
		found := false
		for _, e := range errs {
			if strings.Contains(e.Message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected an error containing %q, got %v", want, errs)
		}
	}
}
