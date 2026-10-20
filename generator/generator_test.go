package generator

import (
	"os"
	"strings"
	"testing"

	"github.com/mevdschee/dbml-tools/interpreter"
	"github.com/mevdschee/dbml-tools/lexer"
	"github.com/mevdschee/dbml-tools/parser"
)

func parseDBML(src string) *interpreter.Database {
	l := lexer.New(src)
	tokens := l.Lex()
	p := parser.New(tokens, src)
	prog := p.Parse()
	interp := interpreter.New()
	return interp.Interpret(prog)
}

func loadBlogDBML(t *testing.T) *interpreter.Database {
	t.Helper()
	src, err := os.ReadFile("../testdata/blog.dbml")
	if err != nil {
		t.Fatalf("failed to read blog.dbml: %v", err)
	}
	return parseDBML(string(src))
}

func TestDumpBlogGeneric(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, Generic)

	t.Run("all_tables_present", func(t *testing.T) {
		tables := []string{"abc_posts", "barcodes", "categories", "comments",
			"countries", "events", "invisibles", "kunsthåndværk", "nopk",
			"post_tags", "products", "tags", "users"}
		for _, tbl := range tables {
			if !strings.Contains(got, `CREATE TABLE "`+tbl+`"`) {
				t.Errorf("missing table %s", tbl)
			}
		}
	})
	t.Run("serial_type", func(t *testing.T) {
		if !strings.Contains(got, `"abc_id" serial PRIMARY KEY`) {
			t.Error("expected serial PRIMARY KEY for abc_id")
		}
	})
	t.Run("bigserial_type", func(t *testing.T) {
		if !strings.Contains(got, `"id" bigserial PRIMARY KEY`) {
			t.Error("expected bigserial PRIMARY KEY for comments.id")
		}
	})
	t.Run("insert_statements", func(t *testing.T) {
		if !strings.Contains(got, `INSERT INTO "abc_posts"`) {
			t.Error("expected INSERT INTO for abc_posts")
		}
		if !strings.Contains(got, `'blog started'`) {
			t.Error("expected data values in INSERT")
		}
	})
	t.Run("hex_values", func(t *testing.T) {
		if !strings.Contains(got, "X'00ff01'") {
			t.Error("expected hex value X'00ff01' in barcodes INSERT")
		}
	})
	t.Run("null_values", func(t *testing.T) {
		if !strings.Contains(got, "NULL)") {
			t.Error("expected NULL values in INSERT")
		}
	})
	t.Run("fk_constraints", func(t *testing.T) {
		if !strings.Contains(got, `ALTER TABLE "abc_posts" ADD CONSTRAINT "fk_abc_posts_abc_category_id"`) {
			t.Error("expected FK constraint for abc_posts")
		}
	})
	t.Run("double_quote_identifiers", func(t *testing.T) {
		if strings.Contains(got, "`") {
			t.Error("Generic should use double-quote identifiers, not backticks")
		}
	})
}

func TestDumpBlogPostgres(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, Postgres)

	t.Run("serial_promotion", func(t *testing.T) {
		if !strings.Contains(got, `"id" SERIAL PRIMARY KEY`) {
			t.Errorf("expected SERIAL PRIMARY KEY for int [pk, increment] in Postgres")
		}
	})
	t.Run("bigserial_promotion", func(t *testing.T) {
		if !strings.Contains(got, `"id" BIGSERIAL PRIMARY KEY`) {
			t.Errorf("expected BIGSERIAL PRIMARY KEY for bigint [pk, increment] in Postgres")
		}
	})
	t.Run("fk_constraints", func(t *testing.T) {
		if !strings.Contains(got, `ALTER TABLE "abc_posts" ADD CONSTRAINT "fk_abc_posts_abc_user_id" FOREIGN KEY ("abc_user_id") REFERENCES "users" ("id");`) {
			t.Error("expected FK constraint for abc_posts.abc_user_id")
		}
	})
	t.Run("table_comment", func(t *testing.T) {
		if !strings.Contains(got, `COMMENT ON TABLE "categories" IS 'The post categories of the blog system.';`) {
			t.Error("expected COMMENT ON TABLE for categories")
		}
	})
	t.Run("column_comment", func(t *testing.T) {
		if !strings.Contains(got, `COMMENT ON COLUMN "categories"."id" IS 'The identifier of the category.';`) {
			t.Error("expected COMMENT ON COLUMN for categories.id")
		}
	})
	t.Run("unicode_table", func(t *testing.T) {
		if !strings.Contains(got, `CREATE TABLE "kunsthåndværk"`) {
			t.Error("expected unicode table name")
		}
	})
	t.Run("unicode_fk", func(t *testing.T) {
		if !strings.Contains(got, `ALTER TABLE "kunsthåndværk" ADD CONSTRAINT "fk_kunsthåndværk_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id");`) {
			t.Error("expected FK constraint for kunsthåndværk.user_id")
		}
	})
	t.Run("no_autoincrement_keyword", func(t *testing.T) {
		if strings.Contains(got, "AUTOINCREMENT") || strings.Contains(got, "AUTO_INCREMENT") {
			t.Error("Postgres should not contain AUTOINCREMENT or AUTO_INCREMENT")
		}
	})
	t.Run("nopk_no_primary_key", func(t *testing.T) {
		nopkIdx := strings.Index(got, `CREATE TABLE "nopk"`)
		if nopkIdx < 0 {
			t.Fatal("nopk table not found")
		}
		nopkSQL := got[nopkIdx : nopkIdx+strings.Index(got[nopkIdx:], ");")+2]
		if strings.Contains(nopkSQL, "PRIMARY KEY") {
			t.Error("nopk table should not have PRIMARY KEY")
		}
	})
}

func TestDumpBlogMariaDB(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, MariaDB)

	t.Run("backtick_quoting", func(t *testing.T) {
		if !strings.Contains(got, "CREATE TABLE `categories`") {
			t.Error("expected backtick quoting for MariaDB")
		}
	})
	t.Run("auto_increment", func(t *testing.T) {
		if !strings.Contains(got, "int AUTO_INCREMENT PRIMARY KEY") {
			t.Errorf("expected AUTO_INCREMENT for int [pk, increment] in MariaDB")
		}
	})
	t.Run("bigint_auto_increment", func(t *testing.T) {
		if !strings.Contains(got, "bigint AUTO_INCREMENT PRIMARY KEY") {
			t.Errorf("expected bigint AUTO_INCREMENT for bigint [pk, increment] in MariaDB")
		}
	})
	t.Run("inline_comment", func(t *testing.T) {
		if !strings.Contains(got, "COMMENT 'The identifier of the category.'") {
			t.Error("expected inline COMMENT for MariaDB column")
		}
	})
	t.Run("table_comment", func(t *testing.T) {
		if !strings.Contains(got, "COMMENT = 'The post categories of the blog system.'") {
			t.Error("expected table COMMENT for MariaDB")
		}
	})
	t.Run("fk_constraints", func(t *testing.T) {
		if !strings.Contains(got, "ALTER TABLE `abc_posts` ADD CONSTRAINT `fk_abc_posts_abc_user_id` FOREIGN KEY (`abc_user_id`) REFERENCES `users` (`id`);") {
			t.Error("expected FK constraint with backtick quoting")
		}
	})
	t.Run("unicode_column", func(t *testing.T) {
		if !strings.Contains(got, "`Umlauts ä_ö_ü-COUNT`") {
			t.Error("expected unicode column name with backtick quoting")
		}
	})
	t.Run("no_serial", func(t *testing.T) {
		if strings.Contains(got, "SERIAL") || strings.Contains(got, "BIGSERIAL") {
			t.Error("MariaDB should not contain SERIAL/BIGSERIAL")
		}
	})
	t.Run("no_comment_on", func(t *testing.T) {
		if strings.Contains(got, "COMMENT ON") {
			t.Error("MariaDB should not use COMMENT ON statements")
		}
	})
	t.Run("all_tables_present", func(t *testing.T) {
		tables := []string{"categories", "users", "abc_posts", "comments", "tags",
			"post_tags", "countries", "events", "products", "barcodes",
			"kunsthåndværk", "invisibles", "nopk"}
		for _, tbl := range tables {
			if !strings.Contains(got, "`"+tbl+"`") {
				t.Errorf("expected table %s in MariaDB output", tbl)
			}
		}
	})
}

func TestDumpBlogSQLite(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, SQLite)

	t.Run("autoincrement", func(t *testing.T) {
		if !strings.Contains(got, "PRIMARY KEY AUTOINCREMENT") {
			t.Error("expected AUTOINCREMENT for SQLite")
		}
	})
	t.Run("autoincrement_requires_integer", func(t *testing.T) {
		// SQLite rejects AUTOINCREMENT unless the column type is exactly INTEGER.
		for _, line := range strings.Split(got, "\n") {
			if strings.Contains(line, "AUTOINCREMENT") && !strings.Contains(line, " INTEGER PRIMARY KEY AUTOINCREMENT") {
				t.Errorf("AUTOINCREMENT on non-INTEGER column: %s", strings.TrimSpace(line))
			}
		}
		if !strings.Contains(got, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`) {
			t.Error("expected bigint/int increment pk to become INTEGER PRIMARY KEY AUTOINCREMENT")
		}
	})
	t.Run("double_quote_identifiers", func(t *testing.T) {
		if !strings.Contains(got, `CREATE TABLE "categories"`) {
			t.Error("expected double-quote identifiers for SQLite")
		}
	})
	t.Run("no_fk_alter_table", func(t *testing.T) {
		if strings.Contains(got, "ALTER TABLE") {
			t.Error("SQLite should not use ALTER TABLE for FK constraints")
		}
	})
	t.Run("no_auto_increment_maria_db", func(t *testing.T) {
		if strings.Contains(got, "AUTO_INCREMENT") {
			t.Error("SQLite should not contain AUTO_INCREMENT")
		}
	})
	t.Run("no_serial", func(t *testing.T) {
		if strings.Contains(got, "SERIAL") || strings.Contains(got, "BIGSERIAL") {
			t.Error("SQLite should not contain SERIAL/BIGSERIAL")
		}
	})
	t.Run("no_comments", func(t *testing.T) {
		if strings.Contains(got, "COMMENT") {
			t.Error("SQLite should not contain any COMMENT statements")
		}
	})
	t.Run("varchar_pk_no_autoincrement", func(t *testing.T) {
		// kunsthåndværk has varchar pk - should not get AUTOINCREMENT
		khIdx := strings.Index(got, `CREATE TABLE "kunsthåndværk"`)
		if khIdx < 0 {
			t.Fatal("kunsthåndværk table not found")
		}
		khSQL := got[khIdx : khIdx+strings.Index(got[khIdx:], ");")+2]
		if strings.Contains(khSQL, "AUTOINCREMENT") {
			t.Error("varchar pk should not have AUTOINCREMENT")
		}
		if !strings.Contains(khSQL, `"id" varchar(36) PRIMARY KEY`) {
			t.Error("expected varchar(36) PRIMARY KEY without AUTOINCREMENT")
		}
	})
	t.Run("nopk_table", func(t *testing.T) {
		nopkIdx := strings.Index(got, `CREATE TABLE "nopk"`)
		if nopkIdx < 0 {
			t.Fatal("nopk table not found")
		}
		nopkSQL := got[nopkIdx : nopkIdx+strings.Index(got[nopkIdx:], ");")+2]
		if strings.Contains(nopkSQL, "PRIMARY KEY") {
			t.Error("nopk table should not have PRIMARY KEY")
		}
	})
	t.Run("all_tables_present", func(t *testing.T) {
		tables := []string{"categories", "users", "abc_posts", "comments", "tags",
			"post_tags", "countries", "events", "products", "barcodes",
			"kunsthåndværk", "invisibles", "nopk"}
		for _, tbl := range tables {
			if !strings.Contains(got, `"`+tbl+`"`) {
				t.Errorf("expected table %s in SQLite output", tbl)
			}
		}
	})
}

func TestDumpBlogDialectFromDatabase(t *testing.T) {
	db := loadBlogDBML(t)
	d := DialectFromDatabase(db)
	// database_type: 'MariaDB' maps to MariaDB
	if d != MariaDB {
		t.Errorf("expected MariaDB dialect, got %d", d)
	}
}

func TestDumpBlogTableCount(t *testing.T) {
	db := loadBlogDBML(t)
	if len(db.Tables) != 13 {
		t.Errorf("expected 13 tables, got %d", len(db.Tables))
	}
}

func TestDumpBlogRefCount(t *testing.T) {
	db := loadBlogDBML(t)
	if len(db.Refs) != 9 {
		t.Errorf("expected 9 refs, got %d", len(db.Refs))
	}
}

func TestDumpMariaDBColumnCommentRoundtrip(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, MariaDB)

	// Verify all three categories column comments are present as inline COMMENT
	comments := map[string]string{
		"id":   "The identifier of the category.",
		"name": "The name of the category.",
		"icon": "A small image representing the category.",
	}
	for col, note := range comments {
		expected := "`" + col + "` "
		if !strings.Contains(got, expected) {
			t.Fatalf("column %s not found in MariaDB output", col)
		}
		expectedComment := "COMMENT '" + note + "'"
		if !strings.Contains(got, expectedComment) {
			t.Errorf("expected MariaDB inline comment for column %s: %s", col, expectedComment)
		}
	}

	// Verify the column comment appears on the same line as the column definition
	// Extract only the categories table block (ends at next CREATE TABLE or end)
	catIdx := strings.Index(got, "CREATE TABLE `categories`")
	if catIdx < 0 {
		t.Fatal("categories table not found")
	}
	// Find end of CREATE TABLE statement (before INSERT or next CREATE)
	nextInsert := strings.Index(got[catIdx:], "\nINSERT ")
	nextCreate := strings.Index(got[catIdx+1:], "CREATE TABLE")
	endPos := len(got)
	if nextInsert >= 0 {
		endPos = catIdx + nextInsert
	}
	if nextCreate >= 0 && catIdx+1+nextCreate < endPos {
		endPos = catIdx + 1 + nextCreate
	}
	catSQL := got[catIdx:endPos]

	// Each column line should contain both the column name and its comment
	lines := strings.Split(catSQL, "\n")
	for _, line := range lines {
		for col, note := range comments {
			if strings.Contains(line, "`"+col+"`") {
				if !strings.Contains(line, "COMMENT '"+note+"'") {
					t.Errorf("column %s comment not on same line as definition:\n%s", col, line)
				}
			}
		}
	}

	// Verify comments do NOT appear as COMMENT ON (Postgres style)
	if strings.Contains(got, "COMMENT ON") {
		t.Error("MariaDB should use inline COMMENT, not COMMENT ON")
	}
}

func TestForeignKeyActions(t *testing.T) {
	src := `Project "p" {
  database_type: 'MariaDB'
}

Table a {
  id bigint [pk]
  b_id bigint
  c_id bigint
  d_id bigint
}

Table b { id bigint [pk] }
Table c { id bigint [pk] }
Table d { id bigint [pk] }

Ref: a.b_id > b.id [delete: set null, update: cascade]
Ref: a.c_id > c.id [delete: cascade]
Ref: a.d_id > d.id [delete: no action]
`
	got := Dump(parseDBML(src), MariaDB)

	want := []string{
		"FOREIGN KEY (`b_id`) REFERENCES `b` (`id`) ON DELETE SET NULL ON UPDATE CASCADE;",
		"FOREIGN KEY (`c_id`) REFERENCES `c` (`id`) ON DELETE CASCADE;",
		// NO ACTION is the SQL default and is left implicit.
		"FOREIGN KEY (`d_id`) REFERENCES `d` (`id`);",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestCompositeAndBlockForeignKeys(t *testing.T) {
	src := `Table a {
  x int
  y int
}

Table b {
  p int
  q int
}

Ref {
  b.(p, q) > a.(x, y) [delete: cascade]
  b.p > a.x
}
`
	got := Dump(parseDBML(src), Postgres)

	want := []string{
		`FOREIGN KEY ("p", "q") REFERENCES "a" ("x", "y") ON DELETE CASCADE;`,
		`FOREIGN KEY ("p") REFERENCES "a" ("x");`,
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

const schemaQualifiedDBML = `
Enum my_schema.status {
  active
  inactive
}

Table my_schema.users {
  id int [pk, increment]
  state my_schema.status
  name varchar [note: 'the name']
}

Table my_schema.posts {
  id int [pk]
  user_id int
}

Ref: my_schema.posts.user_id > my_schema.users.id
`

func TestDumpSchemaQualifiedPostgres(t *testing.T) {
	got := Dump(parseDBML(schemaQualifiedDBML), Postgres)

	checks := []string{
		`CREATE TYPE "my_schema"."status" AS ENUM (`,
		`CREATE TABLE "my_schema"."users" (`,
		`"state" "my_schema"."status"`,
		`ALTER TABLE "my_schema"."posts" ADD CONSTRAINT "fk_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "my_schema"."users" ("id");`,
		`COMMENT ON COLUMN "my_schema"."users"."name" IS 'the name';`,
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"my_schema.users"`) {
		t.Errorf("schema and table must be quoted separately:\n%s", got)
	}
}

func TestDumpSchemaQualifiedMariaDB(t *testing.T) {
	got := Dump(parseDBML(schemaQualifiedDBML), MariaDB)

	if !strings.Contains(got, "CREATE TABLE `my_schema`.`users` (") {
		t.Errorf("expected separately quoted schema and table:\n%s", got)
	}
	if strings.Contains(got, "`my_schema.users`") {
		t.Errorf("schema and table must be quoted separately:\n%s", got)
	}
}

func TestDumpQuotedDottedTableName(t *testing.T) {
	got := Dump(parseDBML("Table \"dotted.name\" {\n  id int [pk]\n}\n"), Postgres)
	if !strings.Contains(got, `CREATE TABLE "dotted.name" (`) {
		t.Errorf("a quoted name containing a dot is a single identifier:\n%s", got)
	}
}

func TestDumpSchemaQualifiedRecords(t *testing.T) {
	src := "Table my_schema.users {\n  id int [pk]\n  records (\"id\") {\n    '1'\n  }\n}\n"
	got := Dump(parseDBML(src), Postgres)
	if !strings.Contains(got, `INSERT INTO "my_schema"."users"`) {
		t.Errorf("expected schema-qualified INSERT:\n%s", got)
	}
}

func TestMigrateSchemaQualified(t *testing.T) {
	oldDB := parseDBML("Table s1.t {\n  id int [pk]\n  a int\n}\nTable s2.t {\n  id int [pk]\n}\n")
	newDB := parseDBML("Table s1.t {\n  id int [pk]\n  a bigint\n}\nTable s3.t {\n  id int [pk]\n}\n")
	got := Migrate(oldDB, newDB, Postgres)

	checks := []string{
		`CREATE TABLE "s3"."t" (`,
		`ALTER TABLE "s1"."t" ALTER COLUMN "a" TYPE bigint;`,
		`DROP TABLE "s2"."t";`,
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
}

func TestSQLiteInlineForeignKeys(t *testing.T) {
	src := `Table s.users {
  id int [pk, increment]
}

Table s.posts {
  id int [pk, increment]
  user_id int
  parent_id int
  records ("id", "user_id", "parent_id") {
    '2', '1', '1'
    '1', '1', null
  }
}

Table s.post_tags {
  post_id int
  tag int
}

Ref: s.posts.user_id > s.users.id [delete: cascade]
Ref: s.posts.parent_id > s.posts.id
Ref {
  s.post_tags.post_id > s.posts.id
}
`
	got := Dump(parseDBML(src), SQLite)

	postsIdx := strings.Index(got, `CREATE TABLE "s"."posts" (`)
	if postsIdx < 0 {
		t.Fatalf("posts table not found:\n%s", got)
	}
	postsSQL := got[postsIdx : postsIdx+strings.Index(got[postsIdx:], ");")]
	// SQLite does not accept a schema-qualified parent table in REFERENCES.
	for _, want := range []string{
		`CONSTRAINT "fk_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE`,
		`CONSTRAINT "fk_posts_parent_id" FOREIGN KEY ("parent_id") REFERENCES "posts" ("id")`,
	} {
		if !strings.Contains(postsSQL, want) {
			t.Errorf("expected %q inside CREATE TABLE posts:\n%s", want, got)
		}
	}
	if strings.Contains(postsSQL, "fk_post_tags") {
		t.Errorf("post_tags FK must not be declared on posts:\n%s", got)
	}
	if !strings.Contains(got, `CONSTRAINT "fk_post_tags_post_id" FOREIGN KEY ("post_id") REFERENCES "posts" ("id")`) {
		t.Errorf("expected FK from a Ref block:\n%s", got)
	}
	if strings.Contains(got, "ALTER TABLE") {
		t.Errorf("SQLite must not use ALTER TABLE for FKs:\n%s", got)
	}

	// Records follow all tables, with FK checks deferred to COMMIT, so the
	// script loads with PRAGMA foreign_keys = ON regardless of row order.
	begin := strings.Index(got, "BEGIN;\nPRAGMA defer_foreign_keys = ON;\n")
	insert := strings.Index(got, `INSERT INTO "s"."posts"`)
	commit := strings.Index(got, "COMMIT;")
	lastCreate := strings.LastIndex(got, "CREATE TABLE")
	if begin < 0 || commit < 0 || !(lastCreate < begin && begin < insert && insert < commit) {
		t.Errorf("expected records in a deferred-FK transaction after all tables:\n%s", got)
	}
}

func TestMigrateSQLiteNewTableForeignKeys(t *testing.T) {
	oldDB := parseDBML("Table users {\n  id int [pk]\n}\n")
	newDB := parseDBML("Table users {\n  id int [pk]\n}\nTable posts {\n  id int [pk]\n  user_id int\n}\nRef: posts.user_id > users.id\n")
	got := Migrate(oldDB, newDB, SQLite)

	want := `CONSTRAINT "fk_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in new table:\n%s", want, got)
	}
}

func TestInlineRefForeignKeys(t *testing.T) {
	src := "Table users {\n  id int [pk]\n}\nTable posts {\n  id int [pk]\n  user_id int [ref: > users.id]\n}\n"

	pg := Dump(parseDBML(src), Postgres)
	if !strings.Contains(pg, `ALTER TABLE "posts" ADD CONSTRAINT "fk_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id");`) {
		t.Errorf("expected FK from inline ref:\n%s", pg)
	}
	lite := Dump(parseDBML(src), SQLite)
	if !strings.Contains(lite, `CONSTRAINT "fk_posts_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id")`) {
		t.Errorf("expected inline FK from inline ref:\n%s", lite)
	}
}

const indexesDBML = `
Table users {
  id int [pk, increment]
  email varchar(255) [unique]
  org_id int
  name varchar(100)

  indexes {
    email [name: "email"]
    org_id [name: "org"]
    name [type: hash]
    (org_id, ` + "`lower(name)`" + `) [unique, name: "org_name"]
  }
}

Table memberships {
  user_id int
  group_id int
  role varchar(20)

  indexes {
    (user_id, group_id) [pk, name: "memberships_pk"]
    role [name: "org"]
  }
}

Table org {
  id int [pk]
  indexes {
    id [name: "users"]
  }
}
`

func TestDumpIndexesPostgres(t *testing.T) {
	got := Dump(parseDBML(indexesDBML), Postgres)

	for _, want := range []string{
		`CREATE INDEX "email" ON "users" ("email");`,
		// "org" is used by two tables and is a table name, "users" is a
		// table name: PostgreSQL names share one namespace per schema.
		`CREATE INDEX "users_org" ON "users" ("org_id");`,
		`CREATE INDEX "memberships_org" ON "memberships" ("role");`,
		`CREATE INDEX "org_users" ON "org" ("id");`,
		`CREATE INDEX "users_name_idx" ON "users" USING hash ("name");`,
		`CREATE UNIQUE INDEX "org_name" ON "users" ("org_id", (lower(name)));`,
		`CONSTRAINT "memberships_pk" PRIMARY KEY ("user_id", "group_id")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `INDEX "memberships_pk"`) {
		t.Errorf("a pk index is the PRIMARY KEY, not a CREATE INDEX:\n%s", got)
	}
}

func TestDumpIndexesSQLite(t *testing.T) {
	got := Dump(parseDBML(indexesDBML), SQLite)

	for _, want := range []string{
		`CREATE INDEX "users_org" ON "users" ("org_id");`,
		// SQLite has one index type, so type: hash is dropped.
		`CREATE INDEX "users_name_idx" ON "users" ("name");`,
		`CREATE UNIQUE INDEX "org_name" ON "users" ("org_id", (lower(name)));`,
		`CONSTRAINT "memberships_pk" PRIMARY KEY ("user_id", "group_id")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}

	// SQLite puts the schema on the index name, not on the table.
	got = Dump(parseDBML("Table s.t {\n  a int\n  indexes {\n    a [name: 'i']\n  }\n}\n"), SQLite)
	if !strings.Contains(got, `CREATE INDEX "s"."i" ON "t" ("a");`) {
		t.Errorf("expected schema-qualified index name:\n%s", got)
	}
}

func TestDumpIndexesMariaDB(t *testing.T) {
	got := Dump(parseDBML(indexesDBML), MariaDB)

	for _, want := range []string{
		// Index names are scoped to the table, so "org" needs no prefix ...
		"CREATE INDEX `org` ON `users` (`org_id`);",
		"CREATE INDEX `org` ON `memberships` (`role`);",
		"CREATE INDEX `users` ON `org` (`id`);",
		// ... but users.email UNIQUE already takes the key name "email".
		"CREATE INDEX `users_email` ON `users` (`email`);",
		"CREATE INDEX `users_name_idx` ON `users` (`name`) USING HASH;",
		"-- MariaDB: expression index `org_name` on `users` is not supported",
		"PRIMARY KEY (`user_id`, `group_id`)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "CONSTRAINT `memberships_pk`") {
		t.Errorf("MariaDB always names the primary key PRIMARY:\n%s", got)
	}
}

func TestMigrateIndexes(t *testing.T) {
	oldDB := parseDBML(`Table t {
  id int [pk]
  a int
  b int
  gone int
  indexes {
    a [name: "a"]
    b [name: "b"]
    gone [name: "gone"]
  }
}`)
	newDB := parseDBML(`Table t {
  id int [pk]
  a int
  b int
  c int
  indexes {
    a [name: "a"]
    b [unique, name: "b"]
    c
  }
}`)
	got := Migrate(oldDB, newDB, Postgres)

	want := `-- Modified table: t
DROP INDEX "b";
DROP INDEX "gone";
ALTER TABLE "t" ADD COLUMN "c" int;
ALTER TABLE "t" DROP COLUMN "gone";
CREATE UNIQUE INDEX "b" ON "t" ("b");
CREATE INDEX "t_c_idx" ON "t" ("c");
`
	if !strings.Contains(got, want) {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}

	if got := Migrate(newDB, newDB, Postgres); got != "" {
		t.Errorf("identical schemas must not differ:\n%s", got)
	}
}

func TestMigrateIndexesCompareResolvedNames(t *testing.T) {
	// The DBML names both indexes "org"; the database holds them under their
	// prefixed names, as tosql created them. That is not a difference.
	dbml := parseDBML("Table a {\n  x int\n  indexes {\n    x [name: 'org']\n  }\n}\n" +
		"Table b {\n  x int\n  indexes {\n    x [name: 'org']\n  }\n}\n")
	live := parseDBML("Table a {\n  x int\n  indexes {\n    x [name: 'a_org']\n  }\n}\n" +
		"Table b {\n  x int\n  indexes {\n    x [name: 'b_org']\n  }\n}\n")
	if got := Migrate(live, dbml, Postgres); got != "" {
		t.Errorf("prefixed names must match:\n%s", got)
	}
}

func TestMigrateIndexExpressionsAsTheDatabaseReportsThem(t *testing.T) {
	dbml := parseDBML("Table t {\n  e varchar\n  indexes {\n    `lower(e)` [name: 'i']\n  }\n}\n")
	live := parseDBML("Table t {\n  e varchar\n  indexes {\n    `lower((e)::text)` [name: 'i']\n  }\n}\n")
	if got := Migrate(live, dbml, Postgres); got != "" {
		t.Errorf("PostgreSQL's rendering of the same expression must match:\n%s", got)
	}
}

func TestMigrateSQLiteRoundTripTypes(t *testing.T) {
	// SQLite reports the INTEGER that tosql used for an auto-increment key,
	// and upper-cases built-in type names.
	dbml := parseDBML("Table t {\n  id bigint [pk, increment]\n  body text\n}\n")
	live := parseDBML("Table t {\n  id INTEGER [pk, increment]\n  body TEXT\n}\n")
	if got := Migrate(live, dbml, SQLite); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
}

func TestMigrateTypeAliases(t *testing.T) {
	// The database reports a type under its own name: PostgreSQL reads int
	// back as integer, MariaDB adds a display width and stores boolean as
	// tinyint(1).
	pgDBML := parseDBML("Table t {\n  a int\n  b bool\n  c varchar(20)\n  d timestamptz\n  e decimal(10,2)\n}\n")
	pgLive := parseDBML("Table t {\n  a integer\n  b boolean\n  c \"character varying\"(20)\n  d \"timestamp with time zone\"\n  e numeric(10,2)\n}\n")
	if got := Migrate(pgLive, pgDBML, Postgres); got != "" {
		t.Errorf("expected no PostgreSQL changes:\n%s", got)
	}

	myDBML := parseDBML("Table t {\n  id bigint [pk, increment]\n  a integer [not null]\n  b boolean\n  d tinyint\n}\n")
	myLive := parseDBML("Table t {\n  id bigint(20) [pk, increment]\n  a int(11) [not null]\n  b tinyint(1)\n  d tinyint(4)\n}\n")
	if got := Migrate(myLive, myDBML, MariaDB); got != "" {
		t.Errorf("expected no MariaDB changes:\n%s", got)
	}

	// Real changes are still found.
	changed := parseDBML("Table t {\n  a bigint\n  b smallint\n  c varchar(30)\n  d \"timestamp without time zone\"\n  e numeric(10,2)\n}\n")
	got := Migrate(pgLive, changed, Postgres)
	for _, col := range []string{`"a"`, `"b"`, `"c"`, `"d"`} {
		if !strings.Contains(got, "ALTER COLUMN "+col+" TYPE") {
			t.Errorf("expected a type change for %s:\n%s", col, got)
		}
	}
	if strings.Contains(got, `"e"`) {
		t.Errorf("unexpected change for e:\n%s", got)
	}
	if got := Migrate(myLive, parseDBML("Table t {\n  id bigint(20) [pk, increment]\n  a int(11) [not null]\n  b tinyint(4)\n  d tinyint(4)\n}\n"), MariaDB); !strings.Contains(got, "MODIFY COLUMN `b` tinyint(4)") {
		t.Errorf("tinyint(1) to tinyint(4) is a change:\n%s", got)
	}
}

func TestMigratePrimaryKey(t *testing.T) {
	oldDB := parseDBML("Table t {\n  a int [pk]\n  b int [not null]\n}\n")
	newDB := parseDBML("Table t {\n  a int\n  b int\n  indexes {\n    (a, b) [pk, name: 't_ab']\n  }\n}\n")

	want := `-- Modified table: t
ALTER TABLE "t" DROP CONSTRAINT "t_pkey";
ALTER TABLE "t" ADD CONSTRAINT "t_ab" PRIMARY KEY ("a", "b");
`
	if got := Migrate(oldDB, newDB, Postgres); !strings.Contains(got, want) {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}
	if got := Migrate(newDB, oldDB, Postgres); !strings.Contains(got, `DROP CONSTRAINT "t_ab";`) ||
		!strings.Contains(got, `ADD PRIMARY KEY ("a");`) {
		t.Errorf("expected the named key dropped and a added:\n%s", got)
	}

	want = "ALTER TABLE `t` DROP PRIMARY KEY;\nALTER TABLE `t` ADD PRIMARY KEY (`a`, `b`);\n"
	if got := Migrate(oldDB, newDB, MariaDB); !strings.Contains(got, want) {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}
	if got := Migrate(oldDB, newDB, SQLite); !strings.Contains(got, "-- SQLite: cannot change the primary key") {
		t.Errorf("expected a SQLite comment:\n%s", got)
	}
	// The same key declared on the column or as a pk index is no change.
	if got := Migrate(oldDB, parseDBML("Table t {\n  a int\n  b int [not null]\n  indexes {\n    a [pk]\n  }\n}\n"), Postgres); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
}

func TestPostgresIndexNamesAreTruncated(t *testing.T) {
	long := strings.Repeat("x", 70)
	db := parseDBML("Table t {\n  a int\n  indexes {\n    a [name: '" + long + "']\n  }\n}\n")
	if got := Dump(db, Postgres); !strings.Contains(got, `"`+long[:63]+`"`) || strings.Contains(got, long[:64]) {
		t.Errorf("expected the name cut to 63 bytes:\n%s", got)
	}
	live := parseDBML("Table t {\n  a int\n  indexes {\n    a [name: '" + long[:63] + "']\n  }\n}\n")
	if got := Migrate(live, db, Postgres); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
	if got := Dump(db, SQLite); !strings.Contains(got, long) {
		t.Errorf("SQLite keeps long names:\n%s", got)
	}
}

func TestMariaDBIndexNamesAreTruncated(t *testing.T) {
	long := strings.Repeat("é", 70)
	db := parseDBML("Table t {\n  a int\n  indexes {\n    a [name: '" + long + "']\n  }\n}\n")
	if got := Dump(db, MariaDB); !strings.Contains(got, "`"+strings.Repeat("é", 64)+"`") {
		t.Errorf("expected the name cut to 64 characters:\n%s", got)
	}
}

func TestIndexNamesStayUniqueAfterPrefixing(t *testing.T) {
	db := parseDBML("Table t {\n  a int\n  indexes {\n    a\n    a [type: hash]\n  }\n}\n")
	got := Dump(db, Postgres)
	if !strings.Contains(got, `CREATE INDEX "t_t_a_idx" ON "t" ("a");`) ||
		!strings.Contains(got, `CREATE INDEX "t_t_a_idx1" ON "t" USING hash ("a");`) {
		t.Errorf("expected two distinct names:\n%s", got)
	}
	// Both indexes take part in a migration.
	if got := Migrate(parseDBML("Table t {\n  a int\n}\n"), db, Postgres); strings.Count(got, "CREATE INDEX") != 2 {
		t.Errorf("expected both indexes created:\n%s", got)
	}
}

func TestNamedSingleColumnPrimaryKey(t *testing.T) {
	db := parseDBML("Table t {\n  id int\n  indexes {\n    id [pk, name: 'my_pk']\n  }\n}\n")
	if got := Dump(db, Postgres); !strings.Contains(got, `CONSTRAINT "my_pk" PRIMARY KEY ("id")`) {
		t.Errorf("expected the named key kept:\n%s", got)
	}
	if got := Dump(db, MariaDB); !strings.Contains(got, "`id` int PRIMARY KEY") {
		t.Errorf("MariaDB names the key PRIMARY, so it stays inline:\n%s", got)
	}
	if got := Dump(db, SQLite); !strings.Contains(got, `"id" int PRIMARY KEY`) {
		t.Errorf("SQLite keeps a single-column key inline:\n%s", got)
	}
}

func TestMigrateNewTableTakesDroppedIndexName(t *testing.T) {
	oldDB := parseDBML("Table a {\n  x int\n  indexes {\n    x [name: 'foo']\n  }\n}\n")
	newDB := parseDBML("Table a {\n  x int\n  indexes {\n    x [name: 'foo']\n  }\n}\nTable foo {\n  y int\n}\n")
	got := Migrate(oldDB, newDB, Postgres)
	drop, create := strings.Index(got, `DROP INDEX "foo";`), strings.Index(got, `CREATE TABLE "foo"`)
	if drop < 0 || create < 0 || drop > create {
		t.Errorf("expected the index dropped before the table is created:\n%s", got)
	}

	// The other way round: a new index takes the name of a removed table.
	got = Migrate(newDB, oldDB, Postgres)
	drop, create = strings.Index(got, `DROP TABLE "foo";`), strings.Index(got, `CREATE INDEX "foo"`)
	if drop < 0 || create < 0 || drop > create {
		t.Errorf("expected the table dropped before the index is created:\n%s", got)
	}
}

func TestMigrateDropsColumnBeforeItsTable(t *testing.T) {
	// The dropped column holds the foreign key to the removed table.
	oldDB := parseDBML("Table u {\n  id int [pk]\n}\nTable p {\n  id int [pk]\n  u_id int [ref: > u.id]\n}\n")
	newDB := parseDBML("Table p {\n  id int [pk]\n}\n")
	got := Migrate(oldDB, newDB, Postgres)
	if strings.Index(got, `DROP COLUMN "u_id"`) > strings.Index(got, `DROP TABLE "u"`) {
		t.Errorf("expected the column dropped first:\n%s", got)
	}
}

func TestMigrateTypeArgumentsInsideName(t *testing.T) {
	dbml := parseDBML("Table t {\n  a timestamptz(3)\n  b \"time(0) without time zone\"\n}\n")
	live := parseDBML("Table t {\n  a \"timestamp(3) with time zone\"\n  b \"time(0)\"\n}\n")
	if got := Migrate(live, dbml, Postgres); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
}

func TestMigrateRenamedPrimaryKey(t *testing.T) {
	oldDB := parseDBML("Table t {\n  a int\n  b int\n  indexes {\n    (a, b) [pk, name: 't_ab']\n  }\n}\n")
	newDB := parseDBML("Table t {\n  a int\n  b int\n  indexes {\n    (a, b) [pk, name: 't_pk']\n  }\n}\n")
	want := `ALTER TABLE "t" RENAME CONSTRAINT "t_ab" TO "t_pk";`
	if got := Migrate(oldDB, newDB, Postgres); !strings.Contains(got, want) {
		t.Errorf("expected %s, got:\n%s", want, got)
	}
	// An unnamed key is named <table>_pkey.
	live := parseDBML("Table t {\n  a int\n  b int\n  indexes {\n    (a, b) [pk, name: 't_pkey']\n  }\n}\n")
	unnamed := parseDBML("Table t {\n  a int\n  b int\n  indexes {\n    (a, b) [pk]\n  }\n}\n")
	if got := Migrate(live, unnamed, Postgres); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
	// MariaDB always names it PRIMARY.
	if got := Migrate(oldDB, newDB, MariaDB); got != "" {
		t.Errorf("expected no MariaDB changes:\n%s", got)
	}
}

func TestDumpTypeSuffix(t *testing.T) {
	db := parseDBML("Table t {\n  id bigint(20) unsigned [pk, increment]\n  a int(5) unsigned zerofill [not null]\n  b decimal(10,2) unsigned\n}\n")
	got := Dump(db, MariaDB)
	for _, want := range []string{
		"`id` bigint(20) unsigned AUTO_INCREMENT PRIMARY KEY",
		"`a` int(5) unsigned zerofill NOT NULL",
		"`b` decimal(10, 2) unsigned",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	// Signed and unsigned are different types.
	signed := parseDBML("Table t {\n  id bigint [pk, increment]\n  a int [not null]\n  b decimal(10,2)\n}\n")
	if got := Migrate(signed, db, MariaDB); strings.Count(got, "MODIFY COLUMN") != 3 {
		t.Errorf("expected three changed columns:\n%s", got)
	}
	if got := Migrate(db, db, MariaDB); got != "" {
		t.Errorf("expected no changes:\n%s", got)
	}
}
