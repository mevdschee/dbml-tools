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
