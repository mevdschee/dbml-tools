// Package introspect connects to a database, reads the schema using
// engine-specific SQL queries, and renders the result as DBML.
package introspect

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

// Options controls which tables are included in the output.
type Options struct {
	// Include: if non-empty, only tables whose names match one of these patterns are emitted.
	Include []string
	// Exclude: tables whose names match any of these patterns are omitted.
	Exclude []string
	// Data: when true, also fetch row data for each table.
	Data bool
}

func (o Options) shouldInclude(name string) bool {
	if len(o.Include) > 0 {
		for _, pat := range o.Include {
			if matchGlob(pat, name) {
				return true
			}
		}
		return false
	}
	for _, pat := range o.Exclude {
		if matchGlob(pat, name) {
			return false
		}
	}
	return true
}

// matchGlob matches name against a glob pattern (case-insensitive, * and ? supported).
func matchGlob(pattern, name string) bool {
	matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(name))
	return err == nil && matched
}

// filterSchema returns a copy of schema with tables/FKs filtered by opts.
func filterSchema(schema *DBSchema, opts Options) *DBSchema {
	if len(opts.Include) == 0 && len(opts.Exclude) == 0 {
		return schema
	}
	included := make(map[string]bool, len(schema.Tables))
	for _, t := range schema.Tables {
		if opts.shouldInclude(t.Name) {
			included[t.Name] = true
		}
	}
	out := &DBSchema{}
	for _, t := range schema.Tables {
		if included[t.Name] {
			out.Tables = append(out.Tables, t)
		}
	}
	out.Enums = schema.Enums // enums are standalone types; keep all
	for _, fk := range schema.FKs {
		if included[fk.TableName] && included[fk.RefTable] {
			out.FKs = append(out.FKs, fk)
		}
	}
	return out
}

// Introspect reads the schema of an already-open database using engine-specific
// catalog queries and returns it as a DBSchema. The caller owns the connection
// (its lifetime, pooling and the identity it authenticates as), which lets a
// server reuse its own pool instead of opening a fresh connection. For PostgreSQL
// and MariaDB, schema is the schema/database name to read; for SQLite it is
// ignored. Every query is bound to ctx. Pair it with GenerateDBML to render DBML.
func Introspect(ctx context.Context, db *sql.DB, engine Engine, schema string) (*DBSchema, error) {
	switch engine {
	case EngineMariaDB:
		return introspectMariaDB(ctx, db, schema)
	case EnginePostgres:
		return introspectPostgres(ctx, db, schema)
	case EngineSQLite:
		return introspectSQLite(ctx, db)
	default:
		return nil, fmt.Errorf("unsupported engine %d", engine)
	}
}

// Run is a convenience entry point for the CLI. It parses dsn, opens its own
// database connection, introspects the schema, and returns DBML output. When
// normalize is true, column types are mapped to canonical DBML equivalents and
// database_type is set to "normalized"; otherwise native types are preserved and
// database_type reflects the actual engine. The driver named by the DSN scheme
// must be registered (blank-imported) by the calling binary.
func Run(dsn string, opts Options, normalize bool) (string, error) {
	parsed, err := ParseDSN(dsn)
	if err != nil {
		return "", err
	}

	driverName := map[Engine]string{
		EngineMariaDB:  "mysql",
		EnginePostgres: "postgres",
		EngineSQLite:   "sqlite",
	}[parsed.Engine]

	db, err := sql.Open(driverName, parsed.DSN)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return "", fmt.Errorf("connect to %s: %w", parsed.DSN, err)
	}

	schema, err := Introspect(ctx, db, parsed.Engine, parsed.Schema)
	if err != nil {
		return "", fmt.Errorf("introspect: %w", err)
	}

	filtered := filterSchema(schema, opts)
	if opts.Data {
		if err := fetchData(ctx, db, filtered, parsed.Engine); err != nil {
			return "", fmt.Errorf("fetch data: %w", err)
		}
	}
	engineType := map[Engine]string{
		EnginePostgres: "PostgreSQL",
		EngineMariaDB:  "MariaDB",
		EngineSQLite:   "SQLite",
	}[parsed.Engine]
	if normalize {
		engineType += " normalized"
	}
	return GenerateDBML(filtered, engineType, normalize), nil
}

// isBinaryType returns true if the column type holds binary data that should
// be hex-encoded in DBML records output.
func isBinaryType(colType string) bool {
	lower := strings.ToLower(colType)
	// Strip any size suffix, e.g. "binary(16)" → "binary"
	if i := strings.Index(lower, "("); i >= 0 {
		lower = lower[:i]
	}
	switch lower {
	case "binary", "varbinary", "blob", "tinyblob", "mediumblob", "longblob",
		"bytea", "bit",
		"geometry", "point", "linestring", "polygon",
		"multipoint", "multilinestring", "multipolygon", "geometrycollection":
		return true
	}
	return false
}

// fetchData queries all rows from each table and populates Table.Records.
func fetchData(ctx context.Context, db *sql.DB, schema *DBSchema, engine Engine) error {
	for _, t := range schema.Tables {
		colNames := make([]string, len(t.Columns))
		binaryCols := make([]bool, len(t.Columns))
		for i, c := range t.Columns {
			colNames[i] = c.Name
			binaryCols[i] = isBinaryType(c.Type)
		}

		var quotedCols []string
		for _, name := range colNames {
			quotedCols = append(quotedCols, quoteIdentSQL(name, engine))
		}
		query := fmt.Sprintf("SELECT %s FROM %s",
			strings.Join(quotedCols, ", "),
			quoteIdentSQL(t.Name, engine))

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("select %s: %w", t.Name, err)
		}

		var dataRows [][]string
		nCols := len(colNames)
		for rows.Next() {
			ptrs := make([]interface{}, nCols)
			for i := range ptrs {
				if binaryCols[i] {
					ptrs[i] = new(sql.RawBytes)
				} else {
					ptrs[i] = new(sql.NullString)
				}
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s: %w", t.Name, err)
			}
			row := make([]string, nCols)
			for i := range ptrs {
				if binaryCols[i] {
					raw := ptrs[i].(*sql.RawBytes)
					if *raw == nil {
						row[i] = "null"
					} else {
						row[i] = "X'" + hex.EncodeToString(*raw) + "'"
					}
				} else {
					v := ptrs[i].(*sql.NullString)
					if v.Valid {
						row[i] = v.String
					} else {
						row[i] = "null"
					}
				}
			}
			dataRows = append(dataRows, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows %s: %w", t.Name, err)
		}

		if len(dataRows) > 0 {
			t.Records = &Record{Columns: colNames, Rows: dataRows}
		}
	}
	return nil
}

// quoteIdentSQL quotes an identifier for use in a SQL query.
func quoteIdentSQL(name string, engine Engine) string {
	switch engine {
	case EngineMariaDB:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
}
