package generator

import (
	"fmt"
	"strings"

	"dbml-tools/interpreter"
)

// Dialect represents the target SQL dialect.
type Dialect int

const (
	// Generic is a passthrough dialect: types are emitted as-is without normalization or validation.
	Generic Dialect = iota
	Postgres
	MySQL
	SQLite
)

// ParseDialect parses a dialect name into a Dialect constant.
// An empty string or "normalized" returns Generic (passthrough mode).
func ParseDialect(s string) (Dialect, error) {
	switch strings.ToLower(s) {
	case "", "generic", "normalized":
		return Generic, nil
	case "postgres", "postgresql", "pg":
		return Postgres, nil
	case "mysql", "mariadb":
		return MySQL, nil
	case "sqlite":
		return SQLite, nil
	default:
		return Generic, fmt.Errorf("unknown dialect %q; supported: postgres, mysql, sqlite", s)
	}
}

func quoteIdent(name string, d Dialect) string {
	if d == MySQL {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	// Generic, Postgres, SQLite all use ANSI double-quote identifiers.
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func tableRef(tbl interpreter.Table, d Dialect) string {
	if tbl.SchemaName != nil && *tbl.SchemaName != "" && d == Postgres {
		return quoteIdent(*tbl.SchemaName, d) + "." + quoteIdent(tbl.Name, d)
	}
	return quoteIdent(tbl.Name, d)
}

// typeArgs extracts string args from a column type's Args field.
func typeArgs(col interpreter.Column) []string {
	if col.Type.Args == nil {
		return nil
	}
	switch v := col.Type.Args.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, len(v))
		for i, a := range v {
			result[i] = fmt.Sprintf("%v", a)
		}
		return result
	}
	return nil
}

// findEnum looks up an enum by type name.
func findEnum(typeName string, db *interpreter.Database) *interpreter.Enum {
	lower := strings.ToLower(typeName)
	for i := range db.Enums {
		if strings.ToLower(db.Enums[i].Name) == lower {
			return &db.Enums[i]
		}
	}
	return nil
}

// mapType maps a DBML type name + args to a SQL/DBML type string.
// Returns (sqlType, isSerial) where isSerial indicates auto-increment semantics.
//
// Generic mode normalises type names to canonical DBML equivalents
// (e.g. "integer" → "int", "character varying" → "varchar"). Unrecognised
// types are copied as-is to support custom/vendor-specific types.
//
// Dialect-specific modes (Postgres, MySQL, SQLite) pass types through unchanged;
// only SQL syntax (identifier quoting, enum expansion, auto-increment keywords,
// etc.) is affected by the dialect.
func mapType(typeName string, args []string, d Dialect, db *interpreter.Database) (string, bool) {
	argStr := ""
	if len(args) > 0 {
		argStr = "(" + strings.Join(args, ", ") + ")"
	}

	if d != Generic {
		// Dialect-specific: pass type through as-is.
		// Enum types still need expansion because not all dialects support named types.
		if e := findEnum(typeName, db); e != nil {
			switch d {
			case MySQL:
				vals := make([]string, len(e.Values))
				for i, v := range e.Values {
					vals[i] = "'" + strings.ReplaceAll(v.Name, "'", "''") + "'"
				}
				return "ENUM(" + strings.Join(vals, ", ") + ")", false
			case SQLite:
				return "TEXT", false
			default: // Postgres: reference the named type
				return quoteIdent(e.Name, Postgres), false
			}
		}
		// Detect serial semantics so renderColumn can suppress redundant NOT NULL / increment.
		switch strings.ToLower(typeName) {
		case "serial":
			return typeName, true
		case "bigserial":
			return typeName, true
		}
		return typeName + argStr, false
	}

	// Generic mode: normalise to canonical DBML type names.
	lower := strings.ToLower(typeName)

	// Enum type reference: return the enum's canonical name.
	if e := findEnum(typeName, db); e != nil {
		return e.Name, false
	}

	switch lower {
	case "int", "integer", "mediumint":
		return "int", false
	case "tinyint":
		if argStr == "(1)" {
			return "bool", false
		}
		return "int", false
	case "bigint":
		return "bigint", false
	case "smallint":
		return "smallint", false
	case "float", "real":
		return "float", false
	case "double", "double precision":
		return "double", false
	case "decimal", "numeric":
		return "decimal" + argStr, false
	case "varchar", "character varying":
		return "varchar" + argStr, false
	case "text", "tinytext", "mediumtext", "longtext":
		return "text", false
	case "char", "character":
		return "char" + argStr, false
	case "date":
		return "date", false
	case "time":
		return "time", false
	case "datetime":
		return "datetime", false
	case "timestamp", "timestamp without time zone", "timestamp with time zone":
		return "timestamp", false
	case "bool", "boolean":
		return "bool", false
	case "blob", "tinyblob", "mediumblob", "longblob", "binary", "bytea":
		return "binary", false
	case "varbinary":
		return "varbinary" + argStr, false
	case "uuid":
		return "uuid", false
	case "json":
		return "json", false
	case "jsonb":
		return "jsonb", false
	case "serial":
		return "serial", true
	case "bigserial":
		return "bigserial", true
	}

	// Unknown/custom type: copy as-is to support vendor-specific types.
	return typeName + argStr, false
}

// renderDefault formats a column default value for SQL output.
func renderDefault(def *interpreter.Default, d Dialect) string {
	if def == nil {
		return ""
	}
	v := fmt.Sprintf("%v", def.Value)
	switch def.Type {
	case "number":
		return v
	case "boolean":
		if strings.EqualFold(v, "true") {
			if d == MySQL || d == SQLite {
				return "1"
			}
			return "TRUE"
		}
		if strings.EqualFold(v, "false") {
			if d == MySQL || d == SQLite {
				return "0"
			}
			return "FALSE"
		}
		return v
	case "expression":
		return v
	default: // string
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
}

// renderColumn renders a single column definition (without leading indent).
// inlinePK: whether to emit PRIMARY KEY inline (only for single-PK tables).
func renderColumn(col interpreter.Column, inlinePK bool, d Dialect, db *interpreter.Database) string {
	args := typeArgs(col)
	sqlType, isSerialType := mapType(col.Type.TypeName, args, d, db)
	isAutoInc := isSerialType || (col.Increment != nil && *col.Increment)

	// For Postgres with [increment]: promote to SERIAL/BIGSERIAL.
	if d == Postgres && !isSerialType && isAutoInc {
		if strings.ToLower(col.Type.TypeName) == "bigint" {
			sqlType = "BIGSERIAL"
		} else {
			sqlType = "SERIAL"
		}
		isSerialType = true
	}

	// For MySQL with [increment]: append AUTO_INCREMENT keyword.
	if d == MySQL && !isSerialType && isAutoInc {
		sqlType += " AUTO_INCREMENT"
	}
	// Generic and SQLite: no auto-increment keyword; the type string is emitted as-is.

	parts := []string{quoteIdent(col.Name, d), sqlType}

	// NOT NULL (pk implies not null, skip to avoid redundancy)
	if col.NotNull != nil && *col.NotNull && !col.PK {
		parts = append(parts, "NOT NULL")
	}

	// UNIQUE (skip if pk, since pk already implies unique)
	if col.Unique && !col.PK {
		parts = append(parts, "UNIQUE")
	}

	// DEFAULT
	if col.DBDefault != nil {
		parts = append(parts, "DEFAULT "+renderDefault(col.DBDefault, d))
	}

	// Inline PRIMARY KEY
	if col.PK && inlinePK {
		if d == SQLite && isAutoInc {
			parts = append(parts, "PRIMARY KEY AUTOINCREMENT")
		} else {
			parts = append(parts, "PRIMARY KEY")
		}
	}

	// MySQL: inline column comment
	if d == MySQL && col.Note != nil && col.Note.Value != "" {
		parts = append(parts, "COMMENT '"+strings.ReplaceAll(col.Note.Value, "'", "''")+"'")
	}

	return strings.Join(parts, " ")
}

// createEnumTypePostgres generates a PostgreSQL CREATE TYPE ... AS ENUM statement.
func createEnumTypePostgres(e interpreter.Enum) string {
	vals := make([]string, len(e.Values))
	for i, v := range e.Values {
		vals[i] = "    '" + strings.ReplaceAll(v.Name, "'", "''") + "'"
	}
	return fmt.Sprintf("CREATE TYPE %s AS ENUM (\n%s\n);",
		quoteIdent(e.Name, Postgres),
		strings.Join(vals, ",\n"),
	)
}

// createTableSQL generates a CREATE TABLE statement.
func createTableSQL(tbl interpreter.Table, db *interpreter.Database, d Dialect) string {
	var sb strings.Builder

	pkCount := 0
	for _, col := range tbl.Fields {
		if col.PK {
			pkCount++
		}
	}

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", tableRef(tbl, d)))

	var lines []string
	for _, col := range tbl.Fields {
		lines = append(lines, "    "+renderColumn(col, pkCount == 1, d, db))
	}

	// Composite PRIMARY KEY constraint
	if pkCount > 1 {
		var pkCols []string
		for _, col := range tbl.Fields {
			if col.PK {
				pkCols = append(pkCols, quoteIdent(col.Name, d))
			}
		}
		lines = append(lines, "    PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}

	sb.WriteString(strings.Join(lines, ",\n"))
	if d == MySQL && tbl.Note != nil && tbl.Note.Value != "" {
		sb.WriteString("\n) COMMENT = '" + strings.ReplaceAll(tbl.Note.Value, "'", "''") + "';\n")
	} else {
		sb.WriteString("\n);\n")
	}
	return sb.String()
}

// fkConstraints generates ALTER TABLE ADD CONSTRAINT FOREIGN KEY statements from Ref declarations.
func fkConstraints(db *interpreter.Database, d Dialect) []string {
	var result []string
	for _, ref := range db.Refs {
		if len(ref.Endpoints) != 2 {
			continue
		}
		ep0, ep1 := ref.Endpoints[0], ref.Endpoints[1]

		var child, parent interpreter.RefEndpoint
		switch {
		case ep0.Relation == "*" && ep1.Relation == "1":
			child, parent = ep0, ep1
		case ep0.Relation == "1" && ep1.Relation == "*":
			child, parent = ep1, ep0
		case ep0.Relation == "1" && ep1.Relation == "1":
			// one-to-one: first endpoint holds the FK
			child, parent = ep0, ep1
		default:
			continue // skip many-to-many (junction table required)
		}

		if len(child.FieldNames) == 0 || len(parent.FieldNames) == 0 {
			continue
		}

		constraintName := fmt.Sprintf("fk_%s_%s",
			child.TableName,
			strings.Join(child.FieldNames, "_"),
		)

		childCols := make([]string, len(child.FieldNames))
		for i, f := range child.FieldNames {
			childCols[i] = quoteIdent(f, d)
		}
		parentCols := make([]string, len(parent.FieldNames))
		for i, f := range parent.FieldNames {
			parentCols[i] = quoteIdent(f, d)
		}

		childTable := quoteIdent(child.TableName, d)
		if child.SchemaName != nil && *child.SchemaName != "" && d == Postgres {
			childTable = quoteIdent(*child.SchemaName, d) + "." + childTable
		}
		parentTable := quoteIdent(parent.TableName, d)
		if parent.SchemaName != nil && *parent.SchemaName != "" && d == Postgres {
			parentTable = quoteIdent(*parent.SchemaName, d) + "." + parentTable
		}

		result = append(result, fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s);",
			childTable,
			quoteIdent(constraintName, d),
			strings.Join(childCols, ", "),
			parentTable,
			strings.Join(parentCols, ", "),
		))
	}
	return result
}

// commentStatements generates COMMENT ON TABLE/COLUMN statements for PostgreSQL.
func commentStatements(db *interpreter.Database, d Dialect) []string {
	if d != Postgres {
		return nil
	}
	var stmts []string
	for _, tbl := range db.Tables {
		ref := tableRef(tbl, d)
		if tbl.Note != nil && tbl.Note.Value != "" {
			stmts = append(stmts, fmt.Sprintf("COMMENT ON TABLE %s IS '%s';",
				ref, strings.ReplaceAll(tbl.Note.Value, "'", "''")))
		}
		for _, col := range tbl.Fields {
			if col.Note != nil && col.Note.Value != "" {
				stmts = append(stmts, fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s';",
					ref, quoteIdent(col.Name, d),
					strings.ReplaceAll(col.Note.Value, "'", "''")))
			}
		}
	}
	return stmts
}

// Dump generates a full CREATE TABLE script for the given schema and dialect.
func Dump(db *interpreter.Database, d Dialect) string {
	var sb strings.Builder

	// PostgreSQL: emit CREATE TYPE for enums first
	if d == Postgres && len(db.Enums) > 0 {
		for _, e := range db.Enums {
			sb.WriteString(createEnumTypePostgres(e))
			sb.WriteString("\n\n")
		}
	}

	for _, tbl := range db.Tables {
		sb.WriteString(createTableSQL(tbl, db, d))
		sb.WriteString("\n")
	}

	// Foreign key constraints (SQLite has very limited FK support via PRAGMA)
	if d != SQLite {
		fks := fkConstraints(db, d)
		for _, fk := range fks {
			sb.WriteString(fk)
			sb.WriteString("\n")
		}
		if len(fks) > 0 {
			sb.WriteString("\n")
		}
	}

	// Comment statements (PostgreSQL only)
	comments := commentStatements(db, d)
	for _, c := range comments {
		sb.WriteString(c)
		sb.WriteString("\n")
	}
	if len(comments) > 0 {
		sb.WriteString("\n")
	}

	return sb.String()
}

// Migrate generates ALTER TABLE statements to migrate oldDB to newDB.
// dryRun=true adds a header comment.
func Migrate(oldDB, newDB *interpreter.Database, d Dialect, dryRun bool) string {
	var sb strings.Builder

	if dryRun {
		sb.WriteString("-- DRY RUN: review statements before executing\n\n")
	}

	oldTables := tableMap(oldDB)
	newTables := tableMap(newDB)
	hasOutput := false

	// New enum types (PostgreSQL only)
	if d == Postgres {
		oldEnums := enumMap(oldDB)
		for _, e := range newDB.Enums {
			if _, exists := oldEnums[e.Name]; !exists {
				hasOutput = true
				sb.WriteString("-- New enum type\n")
				sb.WriteString(createEnumTypePostgres(e))
				sb.WriteString("\n\n")
			}
		}
	}

	// New tables
	for _, tbl := range newDB.Tables {
		if _, exists := oldTables[tbl.Name]; !exists {
			hasOutput = true
			sb.WriteString(fmt.Sprintf("-- New table: %s\n", tbl.Name))
			sb.WriteString(createTableSQL(tbl, newDB, d))
			sb.WriteString("\n")
		}
	}

	// Modified tables (same name, different columns)
	for _, newTbl := range newDB.Tables {
		oldTbl, exists := oldTables[newTbl.Name]
		if !exists {
			continue
		}
		stmts := diffTable(oldTbl, newTbl, oldDB, newDB, d)
		if len(stmts) > 0 {
			hasOutput = true
			sb.WriteString(fmt.Sprintf("-- Modified table: %s\n", newTbl.Name))
			for _, stmt := range stmts {
				sb.WriteString(stmt + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Removed tables
	for _, tbl := range oldDB.Tables {
		if _, exists := newTables[tbl.Name]; !exists {
			hasOutput = true
			sb.WriteString(fmt.Sprintf("-- Removed table: %s\n", tbl.Name))
			sb.WriteString(fmt.Sprintf("DROP TABLE %s;\n\n", quoteIdent(tbl.Name, d)))
		}
	}

	// Removed enum types (PostgreSQL only)
	if d == Postgres {
		newEnums := enumMap(newDB)
		for _, e := range oldDB.Enums {
			if _, exists := newEnums[e.Name]; !exists {
				hasOutput = true
				sb.WriteString(fmt.Sprintf("-- Removed enum type: %s\n", e.Name))
				sb.WriteString(fmt.Sprintf("DROP TYPE %s;\n\n", quoteIdent(e.Name, d)))
			}
		}
	}

	if !hasOutput {
		sb.WriteString("-- No changes detected\n")
	}

	return sb.String()
}

// diffTable generates ALTER TABLE statements for the diff between two table versions.
func diffTable(oldTbl, newTbl interpreter.Table, oldDB, newDB *interpreter.Database, d Dialect) []string {
	var stmts []string

	oldCols := colMap(oldTbl)
	newCols := colMap(newTbl)
	tblRef := quoteIdent(newTbl.Name, d)

	// Added columns (iterate in new-schema order)
	for _, col := range newTbl.Fields {
		if _, exists := oldCols[col.Name]; !exists {
			colDef := renderColumn(col, false, d, newDB) // no inline PK on ADD COLUMN
			stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", tblRef, colDef))
		}
	}

	// Dropped columns (iterate in old-schema order)
	for _, col := range oldTbl.Fields {
		if _, exists := newCols[col.Name]; !exists {
			if d == SQLite {
				stmts = append(stmts, fmt.Sprintf(
					"-- SQLite: ALTER TABLE %s DROP COLUMN %s; (requires SQLite >= 3.35.0)",
					tblRef, quoteIdent(col.Name, d)))
			} else {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s DROP COLUMN %s;", tblRef, quoteIdent(col.Name, d)))
			}
		}
	}

	// Modified columns
	for _, newCol := range newTbl.Fields {
		oldCol, exists := oldCols[newCol.Name]
		if !exists {
			continue
		}

		oldSQL, _ := mapType(oldCol.Type.TypeName, typeArgs(oldCol), d, oldDB)
		newSQL, _ := mapType(newCol.Type.TypeName, typeArgs(newCol), d, newDB)
		typeChanged := oldSQL != newSQL

		oldNN := oldCol.NotNull != nil && *oldCol.NotNull
		newNN := newCol.NotNull != nil && *newCol.NotNull
		nullChanged := oldNN != newNN

		if typeChanged {
			switch d {
			case Postgres:
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s TYPE %s;",
					tblRef, quoteIdent(newCol.Name, d), newSQL))
			case MySQL:
				colDef := renderColumn(newCol, false, d, newDB)
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s MODIFY COLUMN %s;", tblRef, colDef))
			case SQLite:
				stmts = append(stmts, fmt.Sprintf(
					"-- SQLite: cannot ALTER COLUMN type for %s.%s (requires table rebuild)",
					newTbl.Name, newCol.Name))
			}
		} else if nullChanged {
			switch d {
			case Postgres:
				if newNN {
					stmts = append(stmts, fmt.Sprintf(
						"ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
						tblRef, quoteIdent(newCol.Name, d)))
				} else {
					stmts = append(stmts, fmt.Sprintf(
						"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
						tblRef, quoteIdent(newCol.Name, d)))
				}
			case MySQL:
				colDef := renderColumn(newCol, false, d, newDB)
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s MODIFY COLUMN %s;", tblRef, colDef))
			case SQLite:
				stmts = append(stmts, fmt.Sprintf(
					"-- SQLite: cannot modify NOT NULL for %s.%s (requires table rebuild)",
					newTbl.Name, newCol.Name))
			}
		}
	}

	return stmts
}

func tableMap(db *interpreter.Database) map[string]interpreter.Table {
	m := make(map[string]interpreter.Table, len(db.Tables))
	for _, t := range db.Tables {
		m[t.Name] = t
	}
	return m
}

func enumMap(db *interpreter.Database) map[string]interpreter.Enum {
	m := make(map[string]interpreter.Enum, len(db.Enums))
	for _, e := range db.Enums {
		m[e.Name] = e
	}
	return m
}

func colMap(tbl interpreter.Table) map[string]interpreter.Column {
	m := make(map[string]interpreter.Column, len(tbl.Fields))
	for _, c := range tbl.Fields {
		m[c.Name] = c
	}
	return m
}
