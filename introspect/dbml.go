package introspect

import (
	"fmt"
	"strings"
)

// GenerateDBML converts a DBSchema to a DBML string.
// databaseType, if non-empty, is emitted as a Project block header (e.g. "PostgreSQL", "MySQL").
// When normalize is true, column types are mapped to canonical DBML equivalents;
// otherwise the raw SQL types from the database are preserved.
func GenerateDBML(schema *DBSchema, databaseType string, normalize bool) string {
	var sb strings.Builder

	if databaseType != "" {
		fmt.Fprintf(&sb, "Project {\n  database_type: '%s'\n}\n\n", databaseType)
	}

	for _, t := range schema.Tables {
		writeTable(&sb, t, normalize)
		sb.WriteByte('\n')
	}

	for _, e := range schema.Enums {
		writeEnum(&sb, e)
		sb.WriteByte('\n')
	}

	for _, fk := range schema.FKs {
		writeRef(&sb, fk)
	}

	return sb.String()
}

func writeTable(sb *strings.Builder, t *Table, normalize bool) {
	sb.WriteString("Table ")
	sb.WriteString(tableIdent(t.Schema, t.Name))
	sb.WriteString(" {\n")

	for _, col := range t.Columns {
		var dbmlType string
		var isSerial bool
		if normalize {
			dbmlType, isSerial = normalizedDBMLType(col)
		} else {
			dbmlType = col.Type
		}
		attrs := columnAttrs(col, isSerial)
		if len(attrs) > 0 {
			fmt.Fprintf(sb, "  %q %s [%s]\n", col.Name, dbmlType, strings.Join(attrs, ", "))
		} else {
			fmt.Fprintf(sb, "  %q %s\n", col.Name, dbmlType)
		}
	}

	// Collect index/constraint lines
	var indexLines []string

	for _, c := range t.Constraints {
		if len(c.Columns) <= 1 {
			continue // already reflected on the column
		}
		var attrs []string
		if c.IsPrimary {
			attrs = append(attrs, "pk")
		} else {
			attrs = append(attrs, "unique")
		}
		if c.Name != "" {
			attrs = append(attrs, fmt.Sprintf("name: %q", c.Name))
		}
		indexLines = append(indexLines,
			fmt.Sprintf("    (%s) [%s]", quotedCols(c.Columns), strings.Join(attrs, ", ")))
	}

	for _, idx := range t.Indexes {
		var colStr string
		if idx.Expression != "" {
			colStr = fmt.Sprintf("(`%s`)", idx.Expression)
		} else {
			colStr = fmt.Sprintf("(%s)", quotedCols(idx.Columns))
		}
		var attrs []string
		if idx.IsUnique {
			attrs = append(attrs, "unique")
		}
		if idx.Name != "" {
			attrs = append(attrs, fmt.Sprintf("name: %q", idx.Name))
		}
		if t := strings.ToLower(idx.Type); t != "" && t != "btree" {
			attrs = append(attrs, "type: "+t)
		}
		line := "    " + colStr
		if len(attrs) > 0 {
			line += fmt.Sprintf(" [%s]", strings.Join(attrs, ", "))
		}
		indexLines = append(indexLines, line)
	}

	if len(indexLines) > 0 {
		sb.WriteString("\n  indexes {\n")
		for _, line := range indexLines {
			sb.WriteString(line + "\n")
		}
		sb.WriteString("  }\n")
	}

	if t.Records != nil && len(t.Records.Rows) > 0 {
		writeRecords(sb, t.Records)
	}

	if t.Comment != "" {
		fmt.Fprintf(sb, "\n  Note: %q\n", t.Comment)
	}

	sb.WriteString("}\n")
}

func writeRecords(sb *strings.Builder, rec *Record) {
	sb.WriteString("\n  records (")
	for i, col := range rec.Columns {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(sb, "%q", col)
	}
	sb.WriteString(") {\n")
	for _, row := range rec.Rows {
		sb.WriteString("    ")
		for i, val := range row {
			if i > 0 {
				sb.WriteString(", ")
			}
			if val == "null" {
				sb.WriteString("null")
			} else if strings.HasPrefix(val, "X'") {
				sb.WriteString("`" + val + "`")
			} else {
				sb.WriteString("'" + strings.ReplaceAll(val, "'", "\\'") + "'")
			}
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("  }\n")
}

// normalizedDBMLType returns the DBML canonical type name and whether the column
// is a serial (auto-increment primary key, collapsing type + increment into serial/bigserial).
func normalizedDBMLType(col *Column) (string, bool) {
	base := toDBMLType(col.Type)
	if col.IsIncrement {
		if base == "bigint" {
			return "bigserial", true
		}
		return "serial", true
	}
	return base, false
}

// toDBMLType normalizes a raw SQL type string to its DBML canonical equivalent.
func toDBMLType(sqlType string) string {
	lower := strings.ToLower(strings.TrimSpace(sqlType))

	base := lower
	args := ""
	if i := strings.Index(lower, "("); i >= 0 {
		base = strings.TrimSpace(lower[:i])
		args = lower[i:]
	}

	switch base {
	case "int", "integer", "mediumint":
		return "int"
	case "tinyint":
		if args == "(1)" {
			return "bool"
		}
		return "int"
	case "bigint":
		return "bigint"
	case "smallint":
		return "smallint"
	case "float", "real":
		return "float"
	case "double", "double precision":
		return "double"
	case "decimal", "numeric":
		return "decimal" + args
	case "varchar", "character varying":
		return "varchar" + args
	case "text", "tinytext", "mediumtext", "longtext":
		return "text"
	case "char", "character":
		return "char" + args
	case "date":
		return "date"
	case "time":
		return "time"
	case "datetime":
		return "datetime"
	case "timestamp", "timestamp without time zone", "timestamp with time zone":
		return "timestamp"
	case "bool", "boolean":
		return "bool"
	case "blob", "tinyblob", "mediumblob", "longblob", "binary", "bytea":
		return "binary"
	case "varbinary":
		return "varbinary" + args
	case "uuid":
		return "uuid"
	case "json":
		return "json"
	case "jsonb":
		return "jsonb"
	case "serial":
		return "serial"
	case "bigserial":
		return "bigserial"
	}
	return sqlType
}

func columnAttrs(col *Column, isSerial bool) []string {
	var attrs []string
	if col.IsPrimaryKey {
		attrs = append(attrs, "pk")
	}
	if col.IsIncrement && !isSerial {
		attrs = append(attrs, "increment")
	}
	if col.IsUnique {
		attrs = append(attrs, "unique")
	}
	if !col.IsNullable && !isSerial {
		attrs = append(attrs, "not null")
	}
	if col.Default != nil {
		switch col.DefaultType {
		case "number":
			attrs = append(attrs, "default: "+*col.Default)
		case "string":
			val := strings.Trim(*col.Default, "'\"")
			attrs = append(attrs, fmt.Sprintf("default: '%s'", val))
		case "boolean":
			attrs = append(attrs, "default: "+strings.ToLower(*col.Default))
		case "expression":
			attrs = append(attrs, fmt.Sprintf("default: `%s`", *col.Default))
		// "increment" and "null" are not emitted as default: values
		}
	}
	if col.Comment != "" {
		attrs = append(attrs, fmt.Sprintf("note: %q", col.Comment))
	}
	return attrs
}

func writeEnum(sb *strings.Builder, e *Enum) {
	sb.WriteString("Enum ")
	sb.WriteString(tableIdent(e.Schema, e.Name))
	sb.WriteString(" {\n")
	for _, v := range e.Values {
		fmt.Fprintf(sb, "  %s\n", v)
	}
	sb.WriteString("}\n")
}

func writeRef(sb *strings.Builder, fk *ForeignKey) {
	fromTbl := tableIdent(fk.Schema, fk.TableName)
	toTbl := tableIdent(fk.RefSchema, fk.RefTable)

	var fromRef, toRef string
	if len(fk.Columns) == 1 {
		fromRef = fmt.Sprintf("%s.%q", fromTbl, fk.Columns[0])
		toRef = fmt.Sprintf("%s.%q", toTbl, refCol(fk.RefColumns, 0))
	} else {
		fromRef = fmt.Sprintf("%s.(%s)", fromTbl, quotedCols(fk.Columns))
		toRef = fmt.Sprintf("%s.(%s)", toTbl, quotedCols(fk.RefColumns))
	}

	var refAttrs []string
	if d := normaliseAction(fk.OnDelete); d != "" {
		refAttrs = append(refAttrs, "delete: "+d)
	}
	if u := normaliseAction(fk.OnUpdate); u != "" {
		refAttrs = append(refAttrs, "update: "+u)
	}

	if len(refAttrs) > 0 {
		fmt.Fprintf(sb, "Ref: %s > %s [%s]\n", fromRef, toRef, strings.Join(refAttrs, ", "))
	} else {
		fmt.Fprintf(sb, "Ref: %s > %s\n", fromRef, toRef)
	}
}

// tableIdent returns the DBML identifier for a table/enum, with optional schema prefix.
// The "public" schema is omitted because it is the default in PostgreSQL.
func tableIdent(schema, name string) string {
	if schema != "" && schema != "public" {
		return fmt.Sprintf("%q.%q", schema, name)
	}
	return fmt.Sprintf("%q", name)
}

func quotedCols(cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf("%q", c)
	}
	return strings.Join(parts, ", ")
}

func refCol(cols []string, i int) string {
	if i < len(cols) {
		return cols[i]
	}
	return ""
}

// normaliseAction maps DB action names to lowercase DBML keywords.
// Returns empty string for "NO ACTION" and "RESTRICT" (DBML defaults).
func normaliseAction(action string) string {
	switch strings.ToUpper(action) {
	case "NO ACTION", "RESTRICT", "":
		return ""
	default:
		return strings.ToLower(action)
	}
}
