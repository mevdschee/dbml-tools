package generator

import (
	"dbml-tools/interpreter"
	"dbml-tools/lexer"
	"dbml-tools/parser"
	"os"
	"strings"
	"testing"
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
	expected := `CREATE TABLE "categories" (
    "id" int PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "icon" binary
);

CREATE TABLE "users" (
    "id" int PRIMARY KEY,
    "username" varchar(255) NOT NULL,
    "password" varchar(255) NOT NULL,
    "api_key" varchar(255),
    "location" geometry
);

CREATE TABLE "abc_posts" (
    "abc_id" int PRIMARY KEY,
    "abc_user_id" int NOT NULL,
    "abc_category_id" int NOT NULL,
    "abc_content" varchar(255) NOT NULL
);

CREATE TABLE "comments" (
    "id" bigint PRIMARY KEY,
    "post_id" int NOT NULL,
    "message" varchar(255) NOT NULL,
    "category_id" int NOT NULL
);

CREATE TABLE "tags" (
    "id" int PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "is_important" bool NOT NULL
);

CREATE TABLE "post_tags" (
    "id" int PRIMARY KEY,
    "post_id" int NOT NULL,
    "tag_id" int NOT NULL
);

CREATE TABLE "countries" (
    "id" int PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "shape" geometry NOT NULL
);

CREATE TABLE "events" (
    "id" int PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "datetime" timestamp,
    "visitors" bigint
);

CREATE TABLE "products" (
    "id" int PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "price" decimal(10, 2) NOT NULL,
    "properties" json NOT NULL,
    "created_at" timestamp NOT NULL,
    "deleted_at" timestamp
);

CREATE TABLE "barcodes" (
    "id" int PRIMARY KEY,
    "product_id" int NOT NULL,
    "hex" varchar(255) NOT NULL,
    "bin" binary NOT NULL,
    "ip_address" varchar(15)
);

CREATE TABLE "kunsthåndværk" (
    "id" varchar(36) PRIMARY KEY,
    "Umlauts ä_ö_ü-COUNT" int NOT NULL UNIQUE,
    "user_id" int NOT NULL,
    "invisible" varchar(36),
    "invisible_id" varchar(36)
);

CREATE TABLE "invisibles" (
    "id" varchar(36) PRIMARY KEY
);

CREATE TABLE "nopk" (
    "id" varchar(36) NOT NULL
);

ALTER TABLE "abc_posts" ADD CONSTRAINT "fk_abc_posts_abc_user_id" FOREIGN KEY ("abc_user_id") REFERENCES "users" ("id");
ALTER TABLE "abc_posts" ADD CONSTRAINT "fk_abc_posts_abc_category_id" FOREIGN KEY ("abc_category_id") REFERENCES "categories" ("id");
ALTER TABLE "comments" ADD CONSTRAINT "fk_comments_post_id" FOREIGN KEY ("post_id") REFERENCES "abc_posts" ("abc_id");
ALTER TABLE "comments" ADD CONSTRAINT "fk_comments_category_id" FOREIGN KEY ("category_id") REFERENCES "categories" ("id");
ALTER TABLE "post_tags" ADD CONSTRAINT "fk_post_tags_post_id" FOREIGN KEY ("post_id") REFERENCES "abc_posts" ("abc_id");
ALTER TABLE "post_tags" ADD CONSTRAINT "fk_post_tags_tag_id" FOREIGN KEY ("tag_id") REFERENCES "tags" ("id");
ALTER TABLE "barcodes" ADD CONSTRAINT "fk_barcodes_product_id" FOREIGN KEY ("product_id") REFERENCES "products" ("id");
ALTER TABLE "kunsthåndværk" ADD CONSTRAINT "fk_kunsthåndværk_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("id");
ALTER TABLE "kunsthåndværk" ADD CONSTRAINT "fk_kunsthåndværk_invisible_id" FOREIGN KEY ("invisible_id") REFERENCES "invisibles" ("id");

`
	if got != expected {
		t.Errorf("Generic dump mismatch:\n--- got ---\n%s\n--- expected ---\n%s", got, expected)
	}
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

func TestDumpBlogMySQL(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, MySQL)

	t.Run("backtick_quoting", func(t *testing.T) {
		if !strings.Contains(got, "CREATE TABLE `categories`") {
			t.Error("expected backtick quoting for MySQL")
		}
	})
	t.Run("auto_increment", func(t *testing.T) {
		if !strings.Contains(got, "int AUTO_INCREMENT PRIMARY KEY") {
			t.Errorf("expected AUTO_INCREMENT for int [pk, increment] in MySQL")
		}
	})
	t.Run("bigint_auto_increment", func(t *testing.T) {
		if !strings.Contains(got, "bigint AUTO_INCREMENT PRIMARY KEY") {
			t.Errorf("expected bigint AUTO_INCREMENT for bigint [pk, increment] in MySQL")
		}
	})
	t.Run("inline_comment", func(t *testing.T) {
		if !strings.Contains(got, "COMMENT 'The identifier of the category.'") {
			t.Error("expected inline COMMENT for MySQL column")
		}
	})
	t.Run("table_comment", func(t *testing.T) {
		if !strings.Contains(got, "COMMENT = 'The post categories of the blog system.'") {
			t.Error("expected table COMMENT for MySQL")
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
			t.Error("MySQL should not contain SERIAL/BIGSERIAL")
		}
	})
	t.Run("no_comment_on", func(t *testing.T) {
		if strings.Contains(got, "COMMENT ON") {
			t.Error("MySQL should not use COMMENT ON statements")
		}
	})
	t.Run("all_tables_present", func(t *testing.T) {
		tables := []string{"categories", "users", "abc_posts", "comments", "tags",
			"post_tags", "countries", "events", "products", "barcodes",
			"kunsthåndværk", "invisibles", "nopk"}
		for _, tbl := range tables {
			if !strings.Contains(got, "`"+tbl+"`") {
				t.Errorf("expected table %s in MySQL output", tbl)
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
	t.Run("no_auto_increment_mysql", func(t *testing.T) {
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
	// database_type: 'normalized' maps to Generic
	if d != Generic {
		t.Errorf("expected Generic dialect for 'normalized', got %d", d)
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

func TestDumpMySQLColumnCommentRoundtrip(t *testing.T) {
	db := loadBlogDBML(t)
	got := Dump(db, MySQL)

	// Verify all three categories column comments are present as inline COMMENT
	comments := map[string]string{
		"id":   "The identifier of the category.",
		"name": "The name of the category.",
		"icon": "A small image representing the category.",
	}
	for col, note := range comments {
		expected := "`" + col + "` "
		if !strings.Contains(got, expected) {
			t.Fatalf("column %s not found in MySQL output", col)
		}
		expectedComment := "COMMENT '" + note + "'"
		if !strings.Contains(got, expectedComment) {
			t.Errorf("expected MySQL inline comment for column %s: %s", col, expectedComment)
		}
	}

	// Verify the column comment appears on the same line as the column definition
	// Extract only the categories table block (ends at next CREATE TABLE or end)
	catIdx := strings.Index(got, "CREATE TABLE `categories`")
	if catIdx < 0 {
		t.Fatal("categories table not found")
	}
	nextCreate := strings.Index(got[catIdx+1:], "CREATE TABLE")
	var catSQL string
	if nextCreate >= 0 {
		catSQL = got[catIdx : catIdx+1+nextCreate]
	} else {
		catSQL = got[catIdx:]
	}

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
		t.Error("MySQL should use inline COMMENT, not COMMENT ON")
	}
}
