package generator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mevdschee/dbml-tools/interpreter"
)

// pkIndexName returns the name of the table's `[pk]` index, if it has one.
func pkIndexName(tbl interpreter.Table) string {
	for _, idx := range tbl.Indexes {
		if idx.PK && idx.Name != nil {
			return *idx.Name
		}
	}
	return ""
}

// defaultIndexName names an unnamed index the way PostgreSQL does:
// <table>_<column>_..._idx, with "expr" standing in for an expression.
func defaultIndexName(tbl interpreter.Table, idx interpreter.Index) string {
	parts := []string{tbl.Name}
	for _, c := range idx.Columns {
		if c.Type == "expression" {
			parts = append(parts, "expr")
		} else {
			parts = append(parts, c.Value)
		}
	}
	return strings.Join(append(parts, "idx"), "_")
}

// indexNames resolves the SQL name of every non-pk index in db, keyed by
// table key, aligned with that table's Indexes ("" for pk indexes).
//
// MariaDB scopes index names to their table, but PostgreSQL and SQLite share
// one namespace per schema between indexes and tables. A name that is used
// more than once in its scope is prefixed with "<table>_". PostgreSQL names
// and MariaDB names are cut to the length the database takes. In MariaDB a
// column's UNIQUE also takes a key name in the table: the column's name.
func indexNames(db *interpreter.Database, d Dialect) map[string][]string {
	base := map[string][]string{}
	for _, tbl := range db.Tables {
		names := make([]string, len(tbl.Indexes))
		for i, idx := range tbl.Indexes {
			if idx.PK {
				continue
			}
			if idx.Name != nil && *idx.Name != "" {
				names[i] = truncateIdent(*idx.Name, d)
			} else {
				names[i] = truncateIdent(defaultIndexName(tbl, idx), d)
			}
		}
		base[tableKey(tbl)] = names
	}

	// Count every name per scope: per table in MariaDB, where unique columns
	// take a key name, and per schema elsewhere, where tables take one.
	scopeOf := func(tbl interpreter.Table) string {
		if d == MariaDB {
			return tableKey(tbl) + "\x00"
		}
		return qualifiedName(tbl.SchemaName, "") + "\x00"
	}
	used := map[string]int{}
	for _, tbl := range db.Tables {
		if d == MariaDB {
			for _, col := range tbl.Fields {
				if col.Unique && !col.PK {
					used[scopeOf(tbl)+col.Name]++
				}
			}
		} else {
			used[scopeOf(tbl)+tbl.Name]++
		}
		for _, name := range base[tableKey(tbl)] {
			if name != "" {
				used[scopeOf(tbl)+name]++
			}
		}
	}

	resolved := map[string][]string{}
	for _, tbl := range db.Tables {
		names := make([]string, len(tbl.Indexes))
		for i, name := range base[tableKey(tbl)] {
			if name != "" && used[scopeOf(tbl)+name] > 1 {
				name = truncateIdent(tbl.Name+"_"+name, d)
			}
			names[i] = name
		}
		resolved[tableKey(tbl)] = names
	}

	// The prefix does not help two indexes of one table with the same name,
	// and a prefixed name may itself be taken. Number every later use of a
	// name, as PostgreSQL does for its own names (t_a_idx, t_a_idx1).
	taken := map[string]bool{}
	for _, tbl := range db.Tables {
		if d == MariaDB {
			for _, col := range tbl.Fields {
				if col.Unique && !col.PK {
					taken[scopeOf(tbl)+col.Name] = true
				}
			}
		} else {
			taken[scopeOf(tbl)+tbl.Name] = true
		}
	}
	for _, tbl := range db.Tables {
		names := resolved[tableKey(tbl)]
		for i, name := range names {
			if name == "" {
				continue
			}
			for n := 1; taken[scopeOf(tbl)+names[i]]; n++ {
				names[i] = withSuffix(name, strconv.Itoa(n), d)
			}
			taken[scopeOf(tbl)+names[i]] = true
		}
	}
	return resolved
}

// indexRef renders a resolved index name for CREATE/DROP INDEX. PostgreSQL
// and SQLite qualify the index with the table's schema.
func indexRef(tbl interpreter.Table, name string, d Dialect) string {
	if d == MariaDB {
		return quoteIdent(name, d)
	}
	return schemaRef(tbl.SchemaName, name, d)
}

// createIndexSQL renders a CREATE INDEX statement for a non-pk index.
func createIndexSQL(tbl interpreter.Table, idx interpreter.Index, name string, d Dialect) string {
	parts := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		if c.Type == "expression" {
			if d == MariaDB {
				return fmt.Sprintf("-- MariaDB: expression index %s on %s is not supported",
					quoteIdent(name, d), tableRef(tbl, d))
			}
			parts[i] = "(" + c.Value + ")"
		} else {
			parts[i] = quoteIdent(c.Value, d)
		}
	}

	unique := ""
	if idx.Unique {
		unique = "UNIQUE "
	}

	// SQLite takes the schema on the index name, never on the table.
	on := tableRef(tbl, d)
	if d == SQLite {
		on = quoteIdent(tbl.Name, d)
	}

	using, suffix := "", ""
	if idx.Type != nil && *idx.Type != "" {
		switch d {
		case Postgres, Generic:
			using = " USING " + *idx.Type
		case MariaDB:
			if t := strings.ToUpper(*idx.Type); t == "BTREE" || t == "HASH" {
				suffix = " USING " + t
			}
		}
		// SQLite has a single index type, so the type is dropped.
	}

	return fmt.Sprintf("CREATE %sINDEX %s ON %s%s (%s)%s;",
		unique, indexRef(tbl, name, d), on, using, strings.Join(parts, ", "), suffix)
}

// dropIndexSQL renders a DROP INDEX statement.
func dropIndexSQL(tbl interpreter.Table, name string, d Dialect) string {
	if d == MariaDB {
		return fmt.Sprintf("DROP INDEX %s ON %s;", quoteIdent(name, d), tableRef(tbl, d))
	}
	return fmt.Sprintf("DROP INDEX %s;", indexRef(tbl, name, d))
}

// tableIndexes returns the CREATE INDEX statements for a table's non-pk indexes.
func tableIndexes(tbl interpreter.Table, names []string, d Dialect) []string {
	var stmts []string
	for i, idx := range tbl.Indexes {
		if !idx.PK {
			stmts = append(stmts, createIndexSQL(tbl, idx, names[i], d))
		}
	}
	return stmts
}

// indexSignature describes what an index does, independent of its name, so
// two indexes with the same name can be compared across schemas.
func indexSignature(idx interpreter.Index, d Dialect) string {
	parts := make([]string, len(idx.Columns))
	for i, c := range idx.Columns {
		if c.Type == "expression" {
			parts[i] = "`" + normalizeIndexExpr(c.Value) + "`"
		} else {
			parts[i] = c.Value
		}
	}
	typ := "btree"
	if idx.Type != nil && *idx.Type != "" && d != SQLite {
		typ = strings.ToLower(*idx.Type)
	}
	return fmt.Sprintf("unique=%t type=%s (%s)", idx.Unique, typ, strings.Join(parts, ", "))
}

// pgCast matches a PostgreSQL type cast such as ::text or ::character varying(20).
var pgCast = regexp.MustCompile(`::\s*(character varying|double precision|bit varying|` +
	`(timestamp|time) with(out)? time zone|"[^"]+"|[a-z_][a-z0-9_]*)(\(\d+(,\s*\d+)?\))?(\[\])?`)

// normalizeIndexExpr reduces an index expression to a form for comparison
// only. A database hands back its own rendering of an expression: PostgreSQL
// turns lower(email) into lower((email)::text). Casts, parentheses, quotes and
// whitespace are dropped, so both compare as equal.
func normalizeIndexExpr(expr string) string {
	e := pgCast.ReplaceAllString(strings.ToLower(expr), "")
	return strings.Map(func(r rune) rune {
		switch r {
		case '(', ')', '"', ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, e)
}

// diffIndexes compares the non-pk indexes of two versions of a table by
// resolved name. It returns the drops, to run before column changes, and the
// creates, to run after them. An index named after a table in removed (by
// schema-qualified name) is returned in late, to run after that table is
// dropped.
func diffIndexes(oldTbl, newTbl interpreter.Table, oldNames, newNames []string,
	removed map[string]bool, d Dialect) (drops, creates, late []string) {
	oldSig := map[string]string{}
	for i, idx := range oldTbl.Indexes {
		if !idx.PK {
			oldSig[oldNames[i]] = indexSignature(idx, d)
		}
	}
	newSig := map[string]string{}
	for i, idx := range newTbl.Indexes {
		if !idx.PK {
			newSig[newNames[i]] = indexSignature(idx, d)
		}
	}

	for i, idx := range oldTbl.Indexes {
		if idx.PK {
			continue
		}
		name := oldNames[i]
		if sig, ok := newSig[name]; !ok || sig != oldSig[name] {
			drops = append(drops, dropIndexSQL(oldTbl, name, d))
		}
	}
	for i, idx := range newTbl.Indexes {
		if idx.PK {
			continue
		}
		name := newNames[i]
		if sig, ok := oldSig[name]; !ok || sig != newSig[name] {
			stmt := createIndexSQL(newTbl, idx, name, d)
			if removed[qualifiedName(newTbl.SchemaName, name)] {
				late = append(late, stmt)
			} else {
				creates = append(creates, stmt)
			}
		}
	}
	return drops, creates, late
}

// pkColumns returns the primary key columns of a table in column order.
func pkColumns(tbl interpreter.Table) []string {
	var cols []string
	for _, col := range tbl.Fields {
		if col.PK {
			cols = append(cols, col.Name)
		}
	}
	return cols
}

// pkConstraintName returns the name of a table's primary key constraint:
// its pk index name, or <table>_pkey, the name PostgreSQL gives it.
func pkConstraintName(tbl interpreter.Table, d Dialect) string {
	if name := pkIndexName(tbl); name != "" {
		return truncateIdent(name, d)
	}
	return truncateIdent(tbl.Name+"_pkey", d)
}

// diffPK compares the primary keys of two versions of a table. It returns the
// statement dropping the old key, to run before column changes, and the one
// adding the new key, to run after them.
func diffPK(oldTbl, newTbl interpreter.Table, d Dialect) (drops, adds []string) {
	oldCols, newCols := pkColumns(oldTbl), pkColumns(newTbl)
	tblRef := tableRef(newTbl, d)
	if strings.Join(oldCols, "\x00") == strings.Join(newCols, "\x00") {
		// Only PostgreSQL keeps a name of choice that it reports back.
		oldName, newName := pkConstraintName(oldTbl, d), pkConstraintName(newTbl, d)
		if d != Postgres || len(newCols) == 0 || oldName == newName {
			return nil, nil
		}
		return nil, []string{fmt.Sprintf("ALTER TABLE %s RENAME CONSTRAINT %s TO %s;",
			tblRef, quoteIdent(oldName, d), quoteIdent(newName, d))}
	}
	if d == SQLite {
		return nil, []string{fmt.Sprintf(
			"-- SQLite: cannot change the primary key of %s (requires table rebuild)",
			tableKey(newTbl))}
	}

	if len(oldCols) > 0 {
		switch d {
		case MariaDB:
			drops = append(drops, fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY;", tblRef))
		default:
			drops = append(drops, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;",
				tableRef(oldTbl, d), quoteIdent(pkConstraintName(oldTbl, d), d)))
		}
	}

	if len(newCols) > 0 {
		quoted := make([]string, len(newCols))
		for i, c := range newCols {
			quoted[i] = quoteIdent(c, d)
		}
		constraint := ""
		if name := pkIndexName(newTbl); name != "" && d != MariaDB {
			constraint = "CONSTRAINT " + quoteIdent(name, d) + " "
		}
		adds = append(adds, fmt.Sprintf("ALTER TABLE %s ADD %sPRIMARY KEY (%s);",
			tblRef, constraint, strings.Join(quoted, ", ")))
	}
	return drops, adds
}

// withSuffix appends suffix to name, cutting name short so the suffix
// survives truncateIdent.
func withSuffix(name, suffix string, d Dialect) string {
	r := []rune(name)
	for truncateIdent(string(r)+suffix, d) != string(r)+suffix {
		r = r[:len(r)-1]
	}
	return string(r) + suffix
}

// truncateIdent cuts a name to the longest identifier the dialect takes, so
// the name tosql emits loads, and is the name the database reports back.
// PostgreSQL silently cuts names to 63 bytes (NAMEDATALEN - 1); MariaDB
// rejects names over 64 characters.
func truncateIdent(name string, d Dialect) string {
	switch d {
	case Postgres:
		if len(name) <= 63 {
			return name
		}
		// Cut in bytes, but not inside a multi-byte character.
		n := 63
		for n > 0 && !utf8.RuneStart(name[n]) {
			n--
		}
		return name[:n]
	case MariaDB:
		if r := []rune(name); len(r) > 64 {
			return string(r[:64])
		}
	}
	return name
}
