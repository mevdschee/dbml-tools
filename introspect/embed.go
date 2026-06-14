package introspect

import (
	"embed"
	"fmt"
)

// sqlFiles holds the engine-specific catalog queries used by the introspection
// functions. Embedding them in this package (rather than passing an embed.FS in)
// keeps the package self-contained, so an importer can call Introspect without
// supplying the SQL itself.
//
//go:embed sql
var sqlFiles embed.FS

// readSQL reads one embedded SQL file by its package-relative path, e.g.
// "sql/postgresql/tables_columns.sql".
func readSQL(path string) (string, error) {
	b, err := sqlFiles.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
