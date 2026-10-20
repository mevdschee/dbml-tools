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
	Name     string
	Columns  []string // one entry per key: a column name or an expression
	Exprs    []bool   // aligned with Columns; true where the key is an expression
	IsUnique bool
	Type     string // "btree", "hash", etc.
}

// addKey appends a key, a column name or an expression, to the index.
func (idx *Index) addKey(value string, expr bool) {
	idx.Columns = append(idx.Columns, value)
	idx.Exprs = append(idx.Exprs, expr)
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

// Record holds the column names and row values for a table's data.
type Record struct {
	Columns []string
	Rows    [][]string // each inner slice is one row of string-encoded values
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
	Records          *Record // populated when --data is used
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
