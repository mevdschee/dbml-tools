package analysis

import "strings"

// ---------------------------------------------------------------------------
// Static catalogues: keywords, attributes, builtin types.
// These drive both completion and hover.
// ---------------------------------------------------------------------------

// Keyword is a top-level declaration keyword.
type Keyword struct {
	Name   string
	Doc    string
	Insert string // snippet body
}

var Keywords = []Keyword{
	{"Table", "Table declaration — declares a database table with columns and constraints.", "Table ${1:name} {\n\t$0\n}"},
	{"Enum", "Enum declaration — declares an enumerated type with a fixed set of values.", "Enum ${1:name} {\n\t$0\n}"},
	{"Ref", "Ref declaration — declares a foreign-key relationship between two columns.", "Ref: ${1:t1}.${2:c1} > ${3:t2}.${4:c2}"},
	{"Note", "Note declaration — free-form documentation block.", "Note ${1:name} {\n\t'''$0'''\n}"},
	{"Project", "Project declaration — global settings (database_type, name).", "Project ${1:name} {\n\tdatabase_type: '${2|MariaDB,PostgreSQL,SQLite|}'\n}"},
	{"TableGroup", "TableGroup declaration — visual grouping of tables in diagrams.", "TableGroup ${1:name} {\n\t$0\n}"},
	{"TablePartial", "TablePartial declaration — a reusable set of columns/settings mixed into tables with `~name`.", "TablePartial ${1:name} {\n\t$0\n}"},
}

func KeywordByName(name string) *Keyword {
	for i := range Keywords {
		if strings.EqualFold(Keywords[i].Name, name) {
			return &Keywords[i]
		}
	}
	return nil
}

// ColumnAttribute describes a setting allowed inside [ … ] on a column.
type ColumnAttribute struct {
	Name      string
	TakesValue bool
	Doc       string
}

var ColumnAttributes = []ColumnAttribute{
	{"pk", false, "Marks the column as a primary key."},
	{"primary key", false, "Marks the column as a primary key (two-word form)."},
	{"unique", false, "Adds a UNIQUE constraint."},
	{"not null", false, "Disallows NULL values."},
	{"null", false, "Explicitly allows NULL values (the default)."},
	{"increment", false, "Auto-increment / IDENTITY column."},
	{"default", true, "Default value: a literal, identifier (true/false/null), or `expression`."},
	{"note", true, "Inline documentation for this column."},
	{"ref", true, "Inline foreign-key reference: `[ref: > other_table.col]`."},
}

func AttributeByName(name string) *ColumnAttribute {
	low := strings.ToLower(name)
	for i := range ColumnAttributes {
		if ColumnAttributes[i].Name == low {
			return &ColumnAttributes[i]
		}
	}
	return nil
}

// BuiltinType describes a column type recognised across dialects.
type BuiltinType struct {
	Name string
	Doc  string
	// SnippetArgs, if non-empty, is appended to the bare name on completion.
	SnippetArgs string
}

var BuiltinTypes = []BuiltinType{
	{"int", "Integer (32-bit in most dialects).", ""},
	{"integer", "Integer (32-bit in most dialects).", ""},
	{"bigint", "64-bit integer.", ""},
	{"smallint", "16-bit integer.", ""},
	{"tinyint", "8-bit integer (MariaDB/MySQL).", ""},
	{"decimal", "Fixed-precision numeric.", "(${1:10},${2:2})"},
	{"numeric", "Fixed-precision numeric (SQL standard alias for decimal).", "(${1:10},${2:2})"},
	{"float", "Single-precision floating point.", ""},
	{"double", "Double-precision floating point.", ""},
	{"real", "Floating point (alias for double in some dialects).", ""},
	{"boolean", "True/false value.", ""},
	{"bool", "True/false value (alias).", ""},
	{"char", "Fixed-length character string.", "(${1:1})"},
	{"varchar", "Variable-length character string.", "(${1:255})"},
	{"text", "Long text.", ""},
	{"mediumtext", "Medium-length text (MariaDB).", ""},
	{"longtext", "Long text (MariaDB).", ""},
	{"binary", "Fixed-length byte string.", "(${1:16})"},
	{"varbinary", "Variable-length byte string.", "(${1:255})"},
	{"blob", "Binary large object.", ""},
	{"bytea", "Byte array (PostgreSQL).", ""},
	{"date", "Calendar date.", ""},
	{"time", "Time of day.", ""},
	{"datetime", "Date + time without timezone.", ""},
	{"timestamp", "Timestamp.", ""},
	{"timestamptz", "Timestamp with timezone (PostgreSQL).", ""},
	{"json", "JSON value.", ""},
	{"jsonb", "Binary JSON (PostgreSQL).", ""},
	{"uuid", "UUID.", ""},
}

func BuiltinTypeByName(name string) *BuiltinType {
	low := strings.ToLower(name)
	for i := range BuiltinTypes {
		if BuiltinTypes[i].Name == low {
			return &BuiltinTypes[i]
		}
	}
	return nil
}

// IsRelationshipOp reports whether v is one of the four DBML relationship
// operators.
func IsRelationshipOp(v string) bool {
	switch v {
	case ">", "<", "-", "<>":
		return true
	}
	return false
}

// RelationshipOpDoc returns a one-line doc for a relationship operator.
func RelationshipOpDoc(v string) string {
	switch v {
	case ">":
		return "many-to-one"
	case "<":
		return "one-to-many"
	case "-":
		return "one-to-one"
	case "<>":
		return "many-to-many"
	}
	return ""
}

// ProjectSettings catalogues legal keys inside Project { … }.
type ProjectSetting struct {
	Name string
	Doc  string
}

var ProjectSettings = []ProjectSetting{
	{"database_type", "Target SQL dialect for downstream tools. One of: 'MariaDB', 'PostgreSQL', 'SQLite' (optionally suffixed ' normalized')."},
	{"note", "Free-form description of the project."},
}
