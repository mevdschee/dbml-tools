package parser

import "testing"

func TestParseTopLevelRecords(t *testing.T) {
	for _, kw := range []string{"Records", "records"} {
		src := kw + " users(id, username, role) {\n  0, 'Alice', 'admin'\n  1, 'Bob', 'moderator'\n}\n"
		prog, errs := parse(src)
		if len(errs) > 0 {
			t.Fatalf("%s: unexpected errors: %v", kw, errs)
		}
		if len(prog.Body) != 1 {
			t.Fatalf("%s: expected 1 declaration, got %d", kw, len(prog.Body))
		}
		decl := prog.Body[0].(*ElementDeclNode)
		if decl.Columns == nil || len(decl.Columns.Items) != 3 {
			t.Fatalf("%s: expected 3 columns, got %+v", kw, decl.Columns)
		}
		blk := decl.Body.(*BlockExprNode)
		if len(blk.Body) != 2 {
			t.Fatalf("%s: expected 2 rows, got %d", kw, len(blk.Body))
		}
		for _, item := range blk.Body {
			row, ok := item.(*RecordRowNode)
			if !ok || len(row.Values) != 3 {
				t.Errorf("%s: expected a row of 3 values, got %T %+v", kw, item, item)
			}
		}
	}
}

func TestParseTopLevelRecordsSchemaQualified(t *testing.T) {
	prog, errs := parse("Records s.users(id) {\n  1\n}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	decl := prog.Body[0].(*ElementDeclNode)
	if _, ok := decl.Name.(*InfixExprNode); !ok {
		t.Errorf("expected schema.table name, got %T", decl.Name)
	}
}

func TestParseTopLevelRecordsRequiresColumns(t *testing.T) {
	_, errs := parse("Records users {\n  1\n}\n")
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}

func TestParseRecordsValues(t *testing.T) {
	src := "Table t {\n  a int\n  records {\n    -5, status.active, , null, `now()`, 1.5\n    1, 'x',\n  }\n}\n"
	prog, errs := parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	blk := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	rows := blk.Body[len(blk.Body)-1].(*BlockExprNode).Body
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	row := rows[0].(*RecordRowNode)
	if len(row.Values) != 6 {
		t.Fatalf("expected 6 values, got %d", len(row.Values))
	}
	if lit, ok := row.Values[0].(*PrimaryExprNode).Expr.(*LiteralNode); !ok || lit.Token.Value != "-5" {
		t.Errorf("expected literal -5, got %+v", row.Values[0])
	}
	if _, ok := row.Values[1].(*InfixExprNode); !ok {
		t.Errorf("expected enum constant, got %T", row.Values[1])
	}
	if row.Values[2] != nil {
		t.Errorf("expected empty field, got %+v", row.Values[2])
	}
	// A trailing comma leaves an empty last field.
	row = rows[1].(*RecordRowNode)
	if len(row.Values) != 3 || row.Values[2] != nil {
		t.Errorf("expected 3 values ending in an empty field, got %+v", row.Values)
	}
}

func TestParseRecordsWithColumnsAndSettings(t *testing.T) {
	src := "Table t {\n  a int\n  records (a) [example] {\n    1\n  }\n}\n"
	prog, errs := parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	blk := prog.Body[0].(*ElementDeclNode).Body.(*BlockExprNode)
	rows := blk.Body[len(blk.Body)-1].(*BlockExprNode).Body
	if _, ok := rows[0].(*RecordRowNode); !ok {
		t.Errorf("expected a record row, got %T", rows[0])
	}
}

func TestParseRecordsRowError(t *testing.T) {
	_, errs := parse("Records t(a) {\n  1 2\n}\n")
	if len(errs) == 0 {
		t.Fatal("expected an error for a missing comma")
	}
}
