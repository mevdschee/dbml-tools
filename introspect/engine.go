package introspect

import (
	"fmt"
	"net/url"
	"strings"
)

// Engine identifies a supported database backend.
type Engine int

const (
	EngineMariaDB Engine = iota
	EnginePostgres
	EngineSQLite
)

// ParsedDSN carries the normalised connection info.
type ParsedDSN struct {
	Engine Engine
	DSN    string // driver-specific data-source name
	Schema string // schema / database name to introspect
}

// ParseDSN parses a URL-form connection string and returns a ParsedDSN.
//
// Supported schemes:
//
//	mariadb://user:pass@host:port/dbname
//	mysql://user:pass@host:port/dbname
//	postgres://user:pass@host:port/dbname[?schema=myschema]
//	postgresql://user:pass@host:port/dbname[?schema=myschema]
//	sqlite:///path/to/file.db
//	sqlite3:///path/to/file.db
func ParseDSN(rawDSN string) (*ParsedDSN, error) {
	u, err := url.Parse(rawDSN)
	if err != nil {
		return nil, fmt.Errorf("invalid connection string: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "mariadb", "mysql":
		return parseMariaDBDSN(u)
	case "postgres", "postgresql":
		return parsePostgresDSN(u)
	case "sqlite", "sqlite3":
		return parseSQLiteDSN(u)
	default:
		return nil, fmt.Errorf(
			"unsupported engine %q — supported schemes: mariadb, postgresql, sqlite",
			u.Scheme,
		)
	}
}

func parseMariaDBDSN(u *url.URL) (*ParsedDSN, error) {
	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return nil, fmt.Errorf("database name required in mariadb connection string")
	}

	var userPass string
	if u.User != nil {
		pass, _ := u.User.Password()
		userPass = fmt.Sprintf("%s:%s@", u.User.Username(), pass)
	}
	query := ""
	if q := u.RawQuery; q != "" {
		query = "?" + q
	}
	dsn := fmt.Sprintf("%stcp(%s:%s)/%s%s", userPass, host, port, dbName, query)

	return &ParsedDSN{Engine: EngineMariaDB, DSN: dsn, Schema: dbName}, nil
}

func parsePostgresDSN(u *url.URL) (*ParsedDSN, error) {
	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return nil, fmt.Errorf("database name required in postgresql connection string")
	}
	q := u.Query()
	schema := q.Get("schema")
	if schema == "" {
		schema = "public"
	}
	q.Del("schema")
	u.RawQuery = q.Encode()
	return &ParsedDSN{Engine: EnginePostgres, DSN: u.String(), Schema: schema}, nil
}

func parseSQLiteDSN(u *url.URL) (*ParsedDSN, error) {
	// sqlite:///absolute  →  host="", path="/absolute"
	// sqlite://rel/path   →  host="rel", path="/path"
	filePath := u.Host + u.Path
	if filePath == "" {
		return nil, fmt.Errorf("file path required in sqlite connection string")
	}
	return &ParsedDSN{Engine: EngineSQLite, DSN: filePath, Schema: ""}, nil
}
