package introspect

// Column represents a single column in a table.
type Column struct {
	Name         string
	Type         string
	Default      *string // nil when no default
	DefaultType  string  // "number", "string", "boolean", "expression", "increment", "null"
	IsNullable   bool
	IsPrimaryKey bool
	IsUnique     bool
	IsIncrement  bool
	Comment      string
	Generated    string
}

// Index represents a non-constraint index on a table.
type Index struct {
	Name       string
	Columns    []string
	IsUnique   bool
	Type       string // "btree", "hash", etc.
	Expression string // for functional/expression indexes
}

// Constraint represents a PRIMARY KEY or UNIQUE constraint.
type Constraint struct {
	Name      string
	Columns   []string
	IsPrimary bool
}

// ForeignKey represents a FOREIGN KEY constraint.
type ForeignKey struct {
	Name       string
	TableName  string
	Schema     string // empty for MariaDB/SQLite
	Columns    []string
	RefSchema  string
	RefTable   string
	RefColumns []string
	OnDelete   string
	OnUpdate   string
}

// CheckConstraint represents a CHECK constraint.
type CheckConstraint struct {
	Name       string
	ColumnName string // empty if table-level
	Expression string
}

// Table represents a database table.
type Table struct {
	Schema           string // empty for MariaDB/SQLite
	Name             string
	Comment          string
	Columns          []*Column
	Indexes          []*Index
	Constraints      []*Constraint
	CheckConstraints []*CheckConstraint
}

// Enum represents a named enum type (PostgreSQL) or an inline enum column (MariaDB).
type Enum struct {
	Schema string
	Name   string
	Values []string
}

// DBSchema holds the full introspected schema.
type DBSchema struct {
	Tables []*Table
	Enums  []*Enum
	FKs    []*ForeignKey
}
