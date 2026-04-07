// Package introspect connects to a database, reads the schema using
// engine-specific SQL queries, and renders the result as DBML.
package introspect

import (
	"database/sql"
	"embed"
	"fmt"
	"path"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Options controls which tables are included in the output.
type Options struct {
	// Include: if non-empty, only tables whose names match one of these patterns are emitted.
	Include []string
	// Exclude: tables whose names match any of these patterns are omitted.
	Exclude []string
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

// Run is the main entry point. It parses dsn, opens the database, introspects
// the schema using the SQL files embedded in sqlFS, and returns DBML output.
func Run(dsn string, sqlFS embed.FS, opts Options) (string, error) {
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

	if err := db.Ping(); err != nil {
		return "", fmt.Errorf("connect to %s: %w", parsed.DSN, err)
	}

	var schema *DBSchema
	switch parsed.Engine {
	case EngineMariaDB:
		schema, err = introspectMariaDB(db, parsed.Schema, sqlFS)
	case EnginePostgres:
		schema, err = introspectPostgres(db, parsed.Schema, sqlFS)
	case EngineSQLite:
		schema, err = introspectSQLite(db, sqlFS)
	}
	if err != nil {
		return "", fmt.Errorf("introspect: %w", err)
	}

	return GenerateDBML(filterSchema(schema, opts)), nil
}

// readSQL reads a SQL file from the embedded FS.
func readSQL(sqlFS embed.FS, path string) (string, error) {
	b, err := sqlFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
