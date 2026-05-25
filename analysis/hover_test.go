package analysis

import (
	"strings"
	"testing"
)

func TestHover_TableName(t *testing.T) {
	src := `Table us▮ers {
  id int [pk]
  email varchar(255)
  Note: 'application users'
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover")
	}
	if !strings.Contains(h.Markdown, "Table") || !strings.Contains(h.Markdown, "users") {
		t.Errorf("missing table header: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "id") || !strings.Contains(h.Markdown, "email") {
		t.Errorf("missing column list: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "application users") {
		t.Errorf("missing note: %q", h.Markdown)
	}
	if sub(a.Source, h.Range) != "users" {
		t.Errorf("range = %q, want 'users'", sub(a.Source, h.Range))
	}
}

func TestHover_ColumnName(t *testing.T) {
	src := `Table users {
  id int [pk]
  em▮ail varchar(255) [not null, unique, note: 'login email']
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover")
	}
	if !strings.Contains(h.Markdown, "users.email") {
		t.Errorf("missing qualified column: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "varchar(255)") {
		t.Errorf("missing type: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "not null") {
		t.Errorf("missing not null: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "unique") {
		t.Errorf("missing unique: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "login email") {
		t.Errorf("missing note: %q", h.Markdown)
	}
}

func TestHover_ColumnPrimaryKey(t *testing.T) {
	src := `Table t {
  i▮d int [pk]
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if !strings.Contains(h.Markdown, "primary key") {
		t.Errorf("missing pk: %q", h.Markdown)
	}
}

func TestHover_RefEndpointTable(t *testing.T) {
	src := `Table users { id int }
Table orders { user_id int }
Ref: orders.user_id > us▮ers.id`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover on ref endpoint")
	}
	if !strings.Contains(h.Markdown, "users") {
		t.Errorf("expected hover for users: %q", h.Markdown)
	}
}

func TestHover_RefEndpointColumn(t *testing.T) {
	src := `Table users { id int  email varchar(255) }
Table orders { user_id int }
Ref: orders.user_id > users.i▮d`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover on column endpoint")
	}
	if !strings.Contains(h.Markdown, "users.id") {
		t.Errorf("expected qualified hover: %q", h.Markdown)
	}
}

func TestHover_EnumTypeToken(t *testing.T) {
	src := `Enum order_status {
  pending
  shipped
  cancelled
}
Table orders {
  status orde▮r_status
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover on enum-as-type")
	}
	if !strings.Contains(h.Markdown, "Enum") {
		t.Errorf("expected enum header: %q", h.Markdown)
	}
	if !strings.Contains(h.Markdown, "pending") || !strings.Contains(h.Markdown, "shipped") {
		t.Errorf("missing enum values: %q", h.Markdown)
	}
}

func TestHover_EnumDecl(t *testing.T) {
	src := `Enum order_sta▮tus {
  pending
  shipped
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover")
	}
	if !strings.Contains(h.Markdown, "order_status") {
		t.Errorf("missing enum name: %q", h.Markdown)
	}
}

func TestHover_BuiltinType(t *testing.T) {
	src := `Table t {
  c int▮eger
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("expected hover for builtin type")
	}
	if !strings.Contains(strings.ToLower(h.Markdown), "integer") {
		t.Errorf("expected mention of integer: %q", h.Markdown)
	}
}

func TestHover_AttributeName(t *testing.T) {
	src := `Table t {
  id int [p▮k]
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("expected hover for attribute")
	}
	if !strings.Contains(strings.ToLower(h.Markdown), "primary key") {
		t.Errorf("expected primary key doc: %q", h.Markdown)
	}
}

func TestHover_NotNullAttribute(t *testing.T) {
	src := `Table t {
  email varchar [not n▮ull]
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("expected hover")
	}
	if !strings.Contains(strings.ToLower(h.Markdown), "not null") {
		t.Errorf("missing not null doc: %q", h.Markdown)
	}
}

func TestHover_NoHoverOnWhitespace(t *testing.T) {
	src := `Table t {
   ▮ id int
}`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h != nil {
		t.Errorf("expected nil hover on whitespace, got %v", h)
	}
}

func TestHover_AliasResolvesToTable(t *testing.T) {
	src := `Table users as u {
  id int
}
Table orders { uid int }
Ref: orders.uid > u▮.id`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("nil hover on alias")
	}
	if !strings.Contains(h.Markdown, "users") {
		t.Errorf("alias should hover as users: %q", h.Markdown)
	}
}

func TestHover_TopLevelKeyword(t *testing.T) {
	src := `Ta▮ble t { id int }`
	a, off := analyzeWithCursor(t, src)
	h := a.Hover(off)
	if h == nil {
		t.Fatal("expected hover on 'Table' keyword")
	}
	if !strings.Contains(strings.ToLower(h.Markdown), "table declaration") {
		t.Errorf("expected keyword doc: %q", h.Markdown)
	}
}
