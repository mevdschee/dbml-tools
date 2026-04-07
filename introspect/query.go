package introspect

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var namedParamRe = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)

// runQuery executes sqlText against db, substituting named :param placeholders
// with the engine-appropriate syntax and values from params.
func runQuery(db *sql.DB, engine Engine, sqlText string, params map[string]interface{}) (*sql.Rows, error) {
	switch engine {
	case EngineMariaDB:
		return runMariaDB(db, stripLineComments(sqlText), params)
	case EnginePostgres:
		return runPostgres(db, stripLineComments(sqlText), params)
	case EngineSQLite:
		return db.Query(stripLineComments(sqlText))
	default:
		return nil, fmt.Errorf("unsupported engine %d", engine)
	}
}

// stripLineComments removes SQL -- line comments so that :param tokens and ?
// characters inside comments are not mistaken for placeholders.
func stripLineComments(sql string) string {
	lines := strings.Split(sql, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// runMariaDB replaces each :param occurrence with ? and appends the value to args
// (each occurrence is a separate positional argument).
func runMariaDB(db *sql.DB, sqlText string, params map[string]interface{}) (*sql.Rows, error) {
	var args []interface{}
	prepared := namedParamRe.ReplaceAllStringFunc(sqlText, func(match string) string {
		name := match[1:]
		args = append(args, params[name])
		return "?"
	})
	return db.Query(prepared, args...)
}

// runPostgres replaces each unique :param with $N; the same param always maps
// to the same $N so it only appears once in args.
func runPostgres(db *sql.DB, sqlText string, params map[string]interface{}) (*sql.Rows, error) {
	// Hide the PostgreSQL cast operator :: before scanning for :param tokens.
	const castMark = "\x00\x00"
	sqlText = strings.ReplaceAll(sqlText, "::", castMark)

	idx := map[string]int{}
	counter := 1
	var args []interface{}

	prepared := namedParamRe.ReplaceAllStringFunc(sqlText, func(match string) string {
		name := match[1:]
		if _, ok := idx[name]; !ok {
			idx[name] = counter
			args = append(args, params[name])
			counter++
		}
		return fmt.Sprintf("$%d", idx[name])
	})

	prepared = strings.ReplaceAll(prepared, castMark, "::")
	return db.Query(prepared, args...)
}
