package generator

import (
	"strings"
	"testing"
)

func TestDumpSkipsExampleRecords(t *testing.T) {
	src := "Table a {\n  id int\n  records [example] {\n    1\n  }\n}\n" +
		"Table b {\n  id int\n}\nRecords b(id) [example] {\n  2\n}\n" +
		"Table c {\n  id int\n  records (id) {\n    3\n  }\n}\n"
	for _, d := range []Dialect{Postgres, SQLite} {
		got := Dump(parseDBML(src), d)
		if strings.Contains(got, `INSERT INTO "a"`) || strings.Contains(got, `INSERT INTO "b"`) {
			t.Errorf("example records must not be inserted:\n%s", got)
		}
		if !strings.Contains(got, `INSERT INTO "c"`) {
			t.Errorf("expected records of c to be inserted:\n%s", got)
		}
	}
	// With only example records, SQLite needs no insert transaction.
	got := Dump(parseDBML("Table a {\n  id int\n  records [example] {\n    1\n  }\n}\n"), SQLite)
	if strings.Contains(got, "BEGIN;") {
		t.Errorf("unexpected insert transaction:\n%s", got)
	}
}
