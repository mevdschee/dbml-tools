package generator

import (
	"strings"
	"testing"
)

func TestDumpRecordsExpressions(t *testing.T) {
	src := "Table t {\n  id int\n  created timestamp\n  label varchar\n" +
		"  records {\n    1, `now()`, '`now()`'\n  }\n}\n"
	got := Dump(parseDBML(src), Postgres)
	if !strings.Contains(got, "('1', now(), '`now()`');") {
		t.Errorf("expected the expression as written and the string quoted:\n%s", got)
	}
}
