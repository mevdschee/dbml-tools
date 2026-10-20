package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mevdschee/dbml-tools/interpreter"
)

// Dialect represents the target SQL dialect.
type Dialect int

const (
	// Generic is a passthrough dialect: types are emitted as-is without normalization or validation.
	Generic Dialect = iota
	Postgres
	MariaDB
	SQLite
)

// ParseDialect parses a dialect name into a Dialect constant.
// Accepts driver names (postgres, mariadb, sqlite), DBML database_type values
// (PostgreSQL, MariaDB, SQLite), and aliases. A " normalized" suffix is
// stripped before matching (e.g. "MariaDB normalized" → MariaDB).
// An empty string or bare "normalized" returns Generic mode.
func ParseDialect(s string) (Dialect, error) {
	s = strings.TrimSuffix(strings.ToLower(s), " normalized")
	switch s {
	case "", "generic", "normalized":
		return Generic, nil
	case "postgres", "postgresql", "pg":
		return Postgres, nil
	case "mariadb", "mysql", "mysql2":
		return MariaDB, nil
	case "sqlite":
		return SQLite, nil
	default:
		return Generic, fmt.Errorf("unknown dialect %q; supported: mariadb, postgres, sqlite", s)
	}
}

// IsNormalized returns true if the database_type string has the " normalized" suffix.
func IsNormalized(s string) bool {
	return strings.HasSuffix(strings.ToLower(s), " normalized")
}

// databaseType extracts the raw database_type string from a Database's Project setting.
func databaseType(db *interpreter.Database) string {
	proj, ok := db.Project.(map[string]interface{})
	if !ok {
		return ""
	}
	dt, _ := proj["databaseType"].(string)
	return dt
}

// DialectFromDatabase extracts the SQL dialect from an interpreter.Database's
// Project.database_type setting. The " normalized" suffix is stripped before
// matching, so "MariaDB normalized" returns MariaDB.
func DialectFromDatabase(db *interpreter.Database) Dialect {
	d, err := ParseDialect(databaseType(db))
	if err != nil {
		return Generic
	}
	return d
}

// IsNormalizedDatabase returns true if the database has a " normalized" suffix
// in its database_type Project setting.
func IsNormalizedDatabase(db *interpreter.Database) bool {
	return IsNormalized(databaseType(db))
}

func quoteIdent(name string, d Dialect) string {
	if d == MariaDB {
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	}
	// Generic, Postgres, SQLite all use ANSI double-quote identifiers.
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// schemaRef renders a schema-qualified identifier, quoting each part
// separately so that `my_schema.my_table` becomes "my_schema"."my_table"
// instead of a single "my_schema.my_table" identifier.
func schemaRef(schema *string, name string, d Dialect) string {
	if schema != nil && *schema != "" {
		return quoteIdent(*schema, d) + "." + quoteIdent(name, d)
	}
	return quoteIdent(name, d)
}

// qualifiedName joins an optional schema with an object name, unquoted, for
// use as a map key or in a comment.
func qualifiedName(schema *string, name string) string {
	if schema != nil && *schema != "" {
		return *schema + "." + name
	}
	return name
}

func tableRef(tbl interpreter.Table, d Dialect) string {
	return schemaRef(tbl.SchemaName, tbl.Name, d)
}

func enumRef(e interpreter.Enum, d Dialect) string {
	return schemaRef(e.SchemaName, e.Name, d)
}

func endpointRef(ep interpreter.RefEndpoint, d Dialect) string {
	return schemaRef(ep.SchemaName, ep.TableName, d)
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

// findEnum looks up an enum by type name. A schema-qualified type name
// (`my_schema.my_enum`) matches the enum declared in that schema; a bare name
// matches on the enum name alone.
func findEnum(typeName string, db *interpreter.Database) *interpreter.Enum {
	lower := strings.ToLower(typeName)
	for i := range db.Enums {
		e := &db.Enums[i]
		if strings.ToLower(e.Name) == lower ||
			strings.ToLower(qualifiedName(e.SchemaName, e.Name)) == lower {
			return e
		}
	}
	return nil
}

// mapType maps a DBML column type + args to a SQL/DBML type string.
// Returns (sqlType, isSerial) where isSerial indicates auto-increment semantics.
//
// Generic mode normalises type names to canonical DBML equivalents
// (e.g. "integer" → "int", "character varying" → "varchar"). Unrecognised
// types are copied as-is to support custom/vendor-specific types.
//
// Dialect-specific modes (Postgres, MariaDB, SQLite) pass types through unchanged;
// only SQL syntax (identifier quoting, enum expansion, auto-increment keywords,
// etc.) is affected by the dialect.
func mapType(ct interpreter.ColumnType, args []string, d Dialect, db *interpreter.Database) (string, bool) {
	// A schema-qualified type keeps its schema in the emitted SQL.
	typeName := qualifiedName(ct.SchemaName, ct.TypeName)

	argStr := ""
	if len(args) > 0 {
		argStr = "(" + strings.Join(args, ", ") + ")"
	}

	if d != Generic {
		// Dialect-specific: pass type through as-is.
		// Enum types still need expansion because not all dialects support named types.
		if e := findEnum(typeName, db); e != nil {
			switch d {
			case MariaDB:
				vals := make([]string, len(e.Values))
				for i, v := range e.Values {
					vals[i] = "'" + strings.ReplaceAll(v.Name, "'", "''") + "'"
				}
				return "ENUM(" + strings.Join(vals, ", ") + ")", false
			case SQLite:
				return "TEXT", false
			default: // Postgres: reference the named type
				return enumRef(*e, Postgres), false
			}
		}
		// Expand serial semantics per dialect.
		switch strings.ToLower(typeName) {
		case "serial":
			if d == Postgres {
				return "SERIAL", true
			}
			return "int", true // MariaDB/SQLite: bare int; AUTO_INCREMENT added by renderColumn
		case "bigserial":
			if d == Postgres {
				return "BIGSERIAL", true
			}
			return "bigint", true // MariaDB/SQLite: bare bigint
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
			if d == MariaDB || d == SQLite {
				return "1"
			}
			return "TRUE"
		}
		if strings.EqualFold(v, "false") {
			if d == MariaDB || d == SQLite {
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
	sqlType, isAutoInc := columnType(col, inlinePK, d, db)

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

	// MariaDB: inline column comment
	if d == MariaDB && col.Note != nil && col.Note.Value != "" {
		parts = append(parts, "COMMENT '"+strings.ReplaceAll(col.Note.Value, "'", "''")+"'")
	}

	return strings.Join(parts, " ")
}

// columnType returns the SQL type of a column as rendered, including the
// auto-increment form of the dialect, and whether the column auto-increments.
func columnType(col interpreter.Column, inlinePK bool, d Dialect, db *interpreter.Database) (string, bool) {
	args := typeArgs(col)

	sqlType, isSerialType := mapType(col.Type, args, d, db)
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

	// For Generic with [increment]: represent as canonical serial/bigserial.
	if d == Generic && !isSerialType && isAutoInc {
		if strings.ToLower(col.Type.TypeName) == "bigint" {
			sqlType = "bigserial"
		} else {
			sqlType = "serial"
		}
		isSerialType = true
	}

	// For MariaDB with serial type or [increment]: append AUTO_INCREMENT keyword.
	if d == MariaDB && isAutoInc {
		sqlType += " AUTO_INCREMENT"
	}
	// SQLite only accepts AUTOINCREMENT on a column declared exactly INTEGER
	// (a 64-bit rowid alias), so int/bigint/serial keys are emitted as INTEGER.
	if d == SQLite && isAutoInc && col.PK && inlinePK {
		sqlType = "INTEGER"
	}
	// Generic: no auto-increment keyword; the type string is emitted as-is.
	return sqlType, isAutoInc
}

// createEnumTypePostgres generates a PostgreSQL CREATE TYPE ... AS ENUM statement.
func createEnumTypePostgres(e interpreter.Enum) string {
	vals := make([]string, len(e.Values))
	for i, v := range e.Values {
		vals[i] = "    '" + strings.ReplaceAll(v.Name, "'", "''") + "'"
	}
	return fmt.Sprintf("CREATE TYPE %s AS ENUM (\n%s\n);",
		enumRef(e, Postgres),
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

	// A primary key is declared on its column, unless it spans several
	// columns or is named. MariaDB always names the primary key PRIMARY.
	// SQLite keeps a single-column key inline, where AUTOINCREMENT needs it.
	pkName := pkIndexName(tbl)
	if d == MariaDB {
		pkName = ""
	}
	inlinePK := pkCount == 1 && (pkName == "" || d == SQLite)

	sb.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", tableRef(tbl, d)))

	var lines []string
	for _, col := range tbl.Fields {
		lines = append(lines, "    "+renderColumn(col, inlinePK, d, db))
	}

	// PRIMARY KEY constraint
	if pkCount > 0 && !inlinePK {
		var pkCols []string
		for _, col := range tbl.Fields {
			if col.PK {
				pkCols = append(pkCols, quoteIdent(col.Name, d))
			}
		}
		constraint := ""
		if pkName != "" {
			constraint = "CONSTRAINT " + quoteIdent(pkName, d) + " "
		}
		lines = append(lines, "    "+constraint+"PRIMARY KEY ("+strings.Join(pkCols, ", ")+")")
	}

	if d == SQLite {
		for _, fk := range inlineFKs(tbl, db, d) {
			lines = append(lines, "    "+fk)
		}
	}

	sb.WriteString(strings.Join(lines, ",\n"))
	if d == MariaDB && tbl.Note != nil && tbl.Note.Value != "" {
		sb.WriteString("\n) COMMENT = '" + strings.ReplaceAll(tbl.Note.Value, "'", "''") + "';\n")
	} else {
		sb.WriteString("\n);\n")
	}
	return sb.String()
}

// foreignKey is a Ref resolved into the table holding the FK and the table it references.
type foreignKey struct {
	child, parent interpreter.RefEndpoint
	ref           interpreter.Ref
}

// foreignKeys resolves Ref declarations into foreign keys, skipping
// many-to-many refs (they need a junction table) and refs without columns.
func foreignKeys(db *interpreter.Database) []foreignKey {
	var result []foreignKey
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
		result = append(result, foreignKey{child: child, parent: parent, ref: ref})
	}
	return result
}

// fkClause renders "CONSTRAINT name FOREIGN KEY (...) REFERENCES parent (...)"
// plus any referential actions, without a trailing semicolon.
func fkClause(fk foreignKey, d Dialect) string {
	constraintName := fmt.Sprintf("fk_%s_%s",
		fk.child.TableName,
		strings.Join(fk.child.FieldNames, "_"),
	)

	childCols := make([]string, len(fk.child.FieldNames))
	for i, f := range fk.child.FieldNames {
		childCols[i] = quoteIdent(f, d)
	}
	parentCols := make([]string, len(fk.parent.FieldNames))
	for i, f := range fk.parent.FieldNames {
		parentCols[i] = quoteIdent(f, d)
	}

	// SQLite does not allow a schema-qualified parent table in REFERENCES;
	// the parent must live in the same database as the child.
	parentTable := endpointRef(fk.parent, d)
	if d == SQLite {
		parentTable = quoteIdent(fk.parent.TableName, d)
	}

	return fmt.Sprintf(
		"CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s%s",
		quoteIdent(constraintName, d),
		strings.Join(childCols, ", "),
		parentTable,
		strings.Join(parentCols, ", "),
		refAction("ON DELETE", fk.ref.OnDelete),
		refAction("ON UPDATE", fk.ref.OnUpdate),
	)
}

// fkConstraints generates ALTER TABLE ADD CONSTRAINT FOREIGN KEY statements from Ref declarations.
func fkConstraints(db *interpreter.Database, d Dialect) []string {
	var result []string
	for _, fk := range foreignKeys(db) {
		result = append(result, fmt.Sprintf("ALTER TABLE %s ADD %s;",
			endpointRef(fk.child, d), fkClause(fk, d)))
	}
	return result
}

// inlineFKs returns the FOREIGN KEY table constraints for tbl. SQLite cannot
// add constraints with ALTER TABLE, so its foreign keys go inside CREATE TABLE.
func inlineFKs(tbl interpreter.Table, db *interpreter.Database, d Dialect) []string {
	var result []string
	for _, fk := range foreignKeys(db) {
		if endpointKey(fk.child) == tableKey(tbl) {
			result = append(result, fkClause(fk, d))
		}
	}
	return result
}

// refAction renders a referential action clause (e.g. " ON DELETE SET NULL").
// Unset actions and the SQL default NO ACTION produce no clause.
func refAction(clause string, action *string) string {
	if action == nil {
		return ""
	}
	a := strings.ToUpper(strings.Join(strings.Fields(*action), " "))
	if a == "" || a == "NO ACTION" {
		return ""
	}
	return " " + clause + " " + a
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

// insertSQL generates INSERT INTO statements from a table's records data.
func insertSQL(tbl interpreter.Table, d Dialect) string {
	rec := tbl.Records
	var sb strings.Builder

	cols := make([]string, len(rec.Columns))
	for i, c := range rec.Columns {
		cols[i] = quoteIdent(c, d)
	}
	header := fmt.Sprintf("INSERT INTO %s (%s) VALUES\n",
		tableRef(tbl, d), strings.Join(cols, ", "))

	sb.WriteString(header)
	for i, row := range rec.Rows {
		vals := make([]string, len(row))
		for j, v := range row {
			if v == "null" {
				vals[j] = "NULL"
			} else if strings.HasPrefix(v, "X'") {
				vals[j] = v
			} else {
				vals[j] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
			}
		}
		if i < len(rec.Rows)-1 {
			sb.WriteString("(" + strings.Join(vals, ", ") + "),\n")
		} else {
			sb.WriteString("(" + strings.Join(vals, ", ") + ");\n")
		}
	}
	return sb.String()
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

	// SQLite declares foreign keys inside CREATE TABLE, so with
	// PRAGMA foreign_keys = ON a row can only be inserted once its parent
	// table exists. Records therefore follow all tables, in one transaction
	// with deferred FK checks so that insert order does not matter.
	inlineRecords := d != SQLite
	hasRecords := false
	names := indexNames(db, d)
	for _, tbl := range db.Tables {
		sb.WriteString(createTableSQL(tbl, db, d))
		for _, stmt := range tableIndexes(tbl, names[tableKey(tbl)], d) {
			sb.WriteString(stmt + "\n")
		}
		if tbl.Records != nil && len(tbl.Records.Rows) > 0 {
			hasRecords = true
			if inlineRecords {
				sb.WriteString(insertSQL(tbl, d))
			}
		}
		sb.WriteString("\n")
	}
	if !inlineRecords && hasRecords {
		sb.WriteString("BEGIN;\nPRAGMA defer_foreign_keys = ON;\n")
		for _, tbl := range db.Tables {
			if tbl.Records != nil && len(tbl.Records.Rows) > 0 {
				sb.WriteString(insertSQL(tbl, d))
			}
		}
		sb.WriteString("COMMIT;\n\n")
	}

	// Foreign key constraints (SQLite declares them inside CREATE TABLE instead)
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

// Migrate generates ALTER TABLE statements to migrate oldDB to newDB. It
// returns an empty string when the schemas do not differ.
func Migrate(oldDB, newDB *interpreter.Database, d Dialect) string {
	var sb strings.Builder

	oldTables := tableMap(oldDB)
	newTables := tableMap(newDB)
	oldNames := indexNames(oldDB, d)
	newNames := indexNames(newDB, d)

	// New enum types (PostgreSQL only)
	if d == Postgres {
		oldEnums := enumMap(oldDB)
		for _, e := range newDB.Enums {
			if _, exists := oldEnums[enumKey(e)]; !exists {
				sb.WriteString("-- New enum type\n")
				sb.WriteString(createEnumTypePostgres(e))
				sb.WriteString("\n\n")
			}
		}
	}

	// Tables that are removed, by schema-qualified name. In PostgreSQL and
	// SQLite an index cannot take such a name before the table is dropped.
	removed := map[string]bool{}
	for _, tbl := range oldDB.Tables {
		if _, exists := newTables[tableKey(tbl)]; !exists && d != MariaDB {
			removed[tableKey(tbl)] = true
		}
	}

	// Modified tables (same name, different columns). They go before removed
	// tables, so a dropped column takes its foreign key to a removed table
	// along, and before new tables, as PostgreSQL and SQLite share one
	// namespace between tables and indexes: a new table may take the name of
	// an index dropped here.
	var late []string
	for _, newTbl := range newDB.Tables {
		oldTbl, exists := oldTables[tableKey(newTbl)]
		if !exists {
			continue
		}
		drops, creates, lateCreates := diffIndexes(oldTbl, newTbl,
			oldNames[tableKey(oldTbl)], newNames[tableKey(newTbl)], removed, d)
		dropPK, addPK := diffPK(oldTbl, newTbl, d)
		// Drop indexes and the primary key first, as a dropped column may
		// still be indexed, and create them last, as a new index or key may
		// use a new column.
		stmts := append(drops, dropPK...)
		stmts = append(stmts, diffTable(oldTbl, newTbl, oldDB, newDB, d)...)
		stmts = append(stmts, addPK...)
		stmts = append(stmts, creates...)
		if len(stmts) > 0 {
			sb.WriteString(fmt.Sprintf("-- Modified table: %s\n", tableKey(newTbl)))
			for _, stmt := range stmts {
				sb.WriteString(stmt + "\n")
			}
			sb.WriteString("\n")
		}
		if len(lateCreates) > 0 {
			late = append(late, fmt.Sprintf("-- Modified table: %s\n%s\n\n",
				tableKey(newTbl), strings.Join(lateCreates, "\n")))
		}
	}

	// Removed tables
	for _, tbl := range oldDB.Tables {
		if _, exists := newTables[tableKey(tbl)]; !exists {
			sb.WriteString(fmt.Sprintf("-- Removed table: %s\n", tableKey(tbl)))
			sb.WriteString(fmt.Sprintf("DROP TABLE %s;\n\n", tableRef(tbl, d)))
		}
	}

	// Indexes named after a removed table
	for _, stmt := range late {
		sb.WriteString(stmt)
	}

	// New tables
	for _, tbl := range newDB.Tables {
		if _, exists := oldTables[tableKey(tbl)]; !exists {
			sb.WriteString(fmt.Sprintf("-- New table: %s\n", tableKey(tbl)))
			sb.WriteString(createTableSQL(tbl, newDB, d))
			for _, stmt := range tableIndexes(tbl, newNames[tableKey(tbl)], d) {
				sb.WriteString(stmt + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Removed enum types (PostgreSQL only)
	if d == Postgres {
		newEnums := enumMap(newDB)
		for _, e := range oldDB.Enums {
			if _, exists := newEnums[enumKey(e)]; !exists {
				sb.WriteString(fmt.Sprintf("-- Removed enum type: %s\n", enumKey(e)))
				sb.WriteString(fmt.Sprintf("DROP TYPE %s;\n\n", enumRef(e, d)))
			}
		}
	}

	return sb.String()
}

// diffTable generates ALTER TABLE statements for the diff between two table versions.
func diffTable(oldTbl, newTbl interpreter.Table, oldDB, newDB *interpreter.Database, d Dialect) []string {
	var stmts []string

	oldCols := colMap(oldTbl)
	newCols := colMap(newTbl)
	tblRef := tableRef(newTbl, d)
	oldInlinePK := len(pkColumns(oldTbl)) == 1
	newInlinePK := len(pkColumns(newTbl)) == 1

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

		oldSQL := diffType(oldCol, oldInlinePK, d, oldDB)
		newSQL := diffType(newCol, newInlinePK, d, newDB)
		typeChanged := !sameType(oldSQL, newSQL, isEnumColumn(oldCol, oldDB) || isEnumColumn(newCol, newDB), d)
		// When either schema is normalized, compare via normalizeForDiff
		// so that equivalent types with different names or serial semantics
		// (e.g. "binary" vs "blob", "serial" vs "int" with increment)
		// are recognized as identical.
		if typeChanged && (IsNormalizedDatabase(oldDB) || IsNormalizedDatabase(newDB)) {
			typeChanged = normalizeForDiff(oldCol, oldDB) != normalizeForDiff(newCol, newDB)
		}

		// PK implies NOT NULL, so treat PK columns as NOT NULL for comparison.
		oldNN := (oldCol.NotNull != nil && *oldCol.NotNull) || oldCol.PK
		newNN := (newCol.NotNull != nil && *newCol.NotNull) || newCol.PK
		nullChanged := oldNN != newNN

		if typeChanged {
			switch d {
			case Postgres:
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s ALTER COLUMN %s TYPE %s;",
					tblRef, quoteIdent(newCol.Name, d), newSQL))
			case MariaDB:
				colDef := renderColumn(newCol, false, d, newDB)
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s MODIFY COLUMN %s;", tblRef, colDef))
			case SQLite:
				stmts = append(stmts, fmt.Sprintf(
					"-- SQLite: cannot ALTER COLUMN type for %s.%s (requires table rebuild)",
					tableKey(newTbl), newCol.Name))
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
			case MariaDB:
				colDef := renderColumn(newCol, false, d, newDB)
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TABLE %s MODIFY COLUMN %s;", tblRef, colDef))
			case SQLite:
				stmts = append(stmts, fmt.Sprintf(
					"-- SQLite: cannot modify NOT NULL for %s.%s (requires table rebuild)",
					tableKey(newTbl), newCol.Name))
			}
		}
	}

	return stmts
}

// diffType returns the type a column is compared by in a migration. SQLite
// stores an auto-increment key as INTEGER whatever type the DBML declares.
// inlinePK tells whether the table's primary key is a single column.
func diffType(col interpreter.Column, inlinePK bool, d Dialect, db *interpreter.Database) string {
	sqlType, _ := mapType(col.Type, typeArgs(col), d, db)
	if d == SQLite {
		if t, _ := columnType(col, inlinePK, d, db); t == "INTEGER" {
			return t
		}
	}
	return sqlType
}

// sameType compares two rendered column types. SQL type names are not case
// sensitive (SQLite even reports INTEGER and TEXT for integer and text), but
// a quoted enum type name is. Aliases the database reports under another name
// compare as equal, see canonicalType.
func sameType(a, b string, isEnum bool, d Dialect) bool {
	if isEnum {
		return a == b
	}
	return canonicalType(a, d) == canonicalType(b, d)
}

// displayWidth matches the display width MariaDB reports on integer types,
// such as the (20) in bigint(20) or bigint(20) unsigned.
var displayWidth = regexp.MustCompile(`^(tinyint|smallint|mediumint|int|bigint)\(\d+\)`)

// typeAliases maps type names to the name the database reports them under.
var typeAliases = map[Dialect]map[string]string{
	Postgres: {
		"int": "integer", "int4": "integer", "int2": "smallint", "int8": "bigint",
		"serial4": "serial", "serial8": "bigserial",
		"bool": "boolean", "float8": "double precision", "float4": "real",
		"decimal": "numeric", "varchar": "character varying", "char": "character",
		"bpchar": "character", "timestamp": "timestamp without time zone",
		"timestamptz": "timestamp with time zone", "time": "time without time zone",
		"timetz": "time with time zone", "varbit": "bit varying",
	},
	MariaDB: {
		"integer": "int", "bool": "tinyint(1)", "boolean": "tinyint(1)",
		"dec": "decimal", "numeric": "decimal", "fixed": "decimal",
		"double precision": "double", "real": "double",
		"character varying": "varchar", "character": "char",
	},
}

// typeArgsRe matches the arguments of a type, such as (10, 2) or (3).
var typeArgsRe = regexp.MustCompile(`\([^)]*\)`)

// canonicalType reduces a rendered column type to the form the database
// reports it in, so a DBML type compares equal to the same type read back
// from a live database: PostgreSQL reports int as integer, and MariaDB
// reports bigint as bigint(20) and boolean as tinyint(1).
func canonicalType(t string, d Dialect) string {
	t = strings.Join(strings.Fields(strings.ToLower(t)), " ")
	if d == MariaDB && !strings.HasPrefix(t, "tinyint(1)") {
		t = displayWidth.ReplaceAllString(t, "$1")
	}
	// Take the arguments out, as PostgreSQL puts them inside a multi-word
	// name: timestamptz(3) reads back as timestamp(3) with time zone.
	args := ""
	if loc := typeArgsRe.FindStringIndex(t); loc != nil {
		args = strings.ReplaceAll(t[loc[0]:loc[1]], " ", "")
		t = strings.TrimSpace(t[:loc[0]] + t[loc[1]:])
	}
	aliases := typeAliases[d]
	// Map the type name, keeping any suffix such as AUTO_INCREMENT, unsigned
	// or []. A multi-word name such as "double precision" is one name.
	name, rest := t, ""
	if i := strings.IndexAny(t, " ["); i >= 0 {
		name, rest = t[:i], t[i:]
	}
	for alias, canon := range aliases {
		for _, n := range []string{alias, canon} {
			if strings.Contains(n, " ") && len(n) > len(name) && strings.HasPrefix(t, n) &&
				(len(t) == len(n) || t[len(n)] == ' ' || t[len(n)] == '[') {
				name, rest = n, t[len(n):]
			}
		}
	}
	if a, ok := aliases[name]; ok {
		name = a
	}
	return name + args + rest
}

// isEnumColumn reports whether a column's type is an enum declared in db.
func isEnumColumn(col interpreter.Column, db *interpreter.Database) bool {
	return findEnum(qualifiedName(col.Type.SchemaName, col.Type.TypeName), db) != nil
}

// normalizeForDiff returns a canonical type string for a column, suitable for
// cross-schema comparison. It maps to Generic (collapsing dialect synonyms like
// blob→binary) and folds serial/bigserial down to their base types (int/bigint)
// so that "serial" and "int [increment]" compare as equal.
func normalizeForDiff(col interpreter.Column, db *interpreter.Database) string {
	generic, isSerial := mapType(col.Type, typeArgs(col), Generic, db)
	if isSerial {
		// serial → int, bigserial → bigint
		switch generic {
		case "serial":
			return "int"
		case "bigserial":
			return "bigint"
		}
	}
	return generic
}

// tableKey returns the schema-qualified table name used to match tables
// across two schemas, so that tables with the same name in different schemas
// are not confused.
func tableKey(t interpreter.Table) string {
	return qualifiedName(t.SchemaName, t.Name)
}

// endpointKey returns the schema-qualified table name of a ref endpoint.
func endpointKey(ep interpreter.RefEndpoint) string {
	return qualifiedName(ep.SchemaName, ep.TableName)
}

// enumKey returns the schema-qualified enum name used to match enums across
// two schemas.
func enumKey(e interpreter.Enum) string {
	return qualifiedName(e.SchemaName, e.Name)
}

func tableMap(db *interpreter.Database) map[string]interpreter.Table {
	m := make(map[string]interpreter.Table, len(db.Tables))
	for _, t := range db.Tables {
		m[tableKey(t)] = t
	}
	return m
}

func enumMap(db *interpreter.Database) map[string]interpreter.Enum {
	m := make(map[string]interpreter.Enum, len(db.Enums))
	for _, e := range db.Enums {
		m[enumKey(e)] = e
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
