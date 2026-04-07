package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"dbml-tools/generator"
	"dbml-tools/interpreter"
	"dbml-tools/introspect"
	"dbml-tools/lexer"
	"dbml-tools/parser"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: dbml-tools <command> [args]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  lex        <file>              Tokenize and output lexer JSON\n")
	fmt.Fprintf(os.Stderr, "  parse      <file>              Parse and output AST JSON\n")
	fmt.Fprintf(os.Stderr, "  interpret  <file>              Interpret and output database schema JSON\n")
	fmt.Fprintf(os.Stderr, "  check      <file>              Check for parse/semantic errors\n")
	fmt.Fprintf(os.Stderr, "  todbml     [--normalize] <dsn> Connect to a database and output DBML\n")
	fmt.Fprintf(os.Stderr, "  tosql      <file>              Generate CREATE TABLE SQL\n")
	fmt.Fprintf(os.Stderr, "  migrate    [--apply] <old> <new>  Generate migration SQL\n")
	fmt.Fprintf(os.Stderr, "\nSQL dialect is determined by the database_type Project setting in DBML files.\n")
	fmt.Fprintf(os.Stderr, "\nConnection string examples:\n")
	fmt.Fprintf(os.Stderr, "  mariadb://user:pass@host:3306/mydb\n")
	fmt.Fprintf(os.Stderr, "  postgres://user:pass@host:5432/mydb[?schema=public]\n")
	fmt.Fprintf(os.Stderr, "  sqlite:///path/to/file.db\n")
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]

	switch cmd {
	case "todbml":
		doToDBML(os.Args[2:])
	case "tosql":
		doToSQL(os.Args[2:])
	case "migrate":
		doMigrate(os.Args[2:])
	case "check":
		doCheck(os.Args[2:])
	default:
		if len(os.Args) < 3 {
			usage()
		}
		file := os.Args[2]
		src, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		source := string(src)
		switch cmd {
		case "lex":
			doLex(source)
		case "parse":
			doParse(source)
		case "interpret":
			doInterpret(source)
		default:
			usage()
		}
	}
}

func readFile(path string) string {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return string(src)
}

func parseAndInterpret(source string) *interpreter.Database {
	l := lexer.New(source)
	tokens := l.Lex()
	p := parser.New(tokens, source)
	prog := p.Parse()
	interp := interpreter.New()
	return interp.Interpret(prog)
}

func doToSQL(args []string) {
	fs := flag.NewFlagSet("tosql", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools tosql <file>\n")
		fmt.Fprintf(os.Stderr, "       SQL dialect is determined by the database_type Project setting.\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	db := parseAndInterpret(readFile(fs.Arg(0)))
	d := generator.DialectFromDatabase(db)
	fmt.Print(generator.Dump(db, d))
}

func doMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	apply := fs.Bool("apply", false, "remove dry-run header (statements are ready to apply)")
	excludeStr := fs.String("exclude", "", "comma-separated table name patterns to exclude (supports * glob)")
	includeStr := fs.String("include", "", "comma-separated table name patterns to include (all others excluded)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools migrate [--apply] [--exclude pattern,...] <old> <new>\n")
		fmt.Fprintf(os.Stderr, "       <old> and <new> may each be a .dbml file path or a connection string.\n")
		fmt.Fprintf(os.Stderr, "       SQL dialect is auto-detected from DSN or database_type Project setting.\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 2 {
		fs.Usage()
		os.Exit(1)
	}

	oldArg, newArg := fs.Arg(0), fs.Arg(1)
	opts := introspect.Options{
		Exclude: parseCSV(*excludeStr),
		Include: parseCSV(*includeStr),
	}

	oldDB, err := loadSchema(oldArg, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", oldArg, err)
		os.Exit(1)
	}
	newDB, err := loadSchema(newArg, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", newArg, err)
		os.Exit(1)
	}

	// Resolve dialect: auto-detect from DSN > database_type from DBML > default generic.
	d := resolveDialect(oldArg, newArg, oldDB, newDB)

	fmt.Print(generator.Migrate(oldDB, newDB, d, !*apply))
}

// loadSchema reads a schema from either a DBML file path or a live database DSN.
func loadSchema(arg string, opts introspect.Options) (*interpreter.Database, error) {
	if _, err := introspect.ParseDSN(arg); err == nil {
		dbml, err := introspect.Run(arg, sqlFiles, opts, false)
		if err != nil {
			return nil, err
		}
		return parseAndInterpret(dbml), nil
	}
	src, err := os.ReadFile(arg)
	if err != nil {
		return nil, err
	}
	return parseAndInterpret(string(src)), nil
}

// resolveDialect determines the dialect from DSN auto-detect or database_type Project setting.
func resolveDialect(arg1, arg2 string, db1, db2 *interpreter.Database) generator.Dialect {
	// Auto-detect from first DSN found.
	for _, arg := range []string{arg1, arg2} {
		if parsed, err := introspect.ParseDSN(arg); err == nil {
			switch parsed.Engine {
			case introspect.EnginePostgres:
				return generator.Postgres
			case introspect.EngineMariaDB:
				return generator.MySQL
			case introspect.EngineSQLite:
				return generator.SQLite
			}
		}
	}
	// Fall back to database_type from DBML schemas.
	for _, db := range []*interpreter.Database{db1, db2} {
		if d := generator.DialectFromDatabase(db); d != generator.Generic {
			return d
		}
	}
	return generator.Generic
}

func doLex(source string) {
	l := lexer.New(source)
	tokens := l.Lex()
	simple := lexer.ToSimpleTokens(tokens)
	writeJSON(simple)
}

func doParse(source string) {
	l := lexer.New(source)
	tokens := l.Lex()
	p := parser.New(tokens, source)
	prog := p.Parse()
	writeJSON(prog.ToJSON())
}

func doInterpret(source string) {
	l := lexer.New(source)
	tokens := l.Lex()
	p := parser.New(tokens, source)
	prog := p.Parse()
	interp := interpreter.New()
	db := interp.Interpret(prog)
	writeJSON(db)
}

func doCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools check <file>\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	file := fs.Arg(0)
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	source := string(src)

	l := lexer.New(source)
	tokens := l.Lex()
	p := parser.New(tokens, source)
	prog := p.Parse()
	interp := interpreter.New()
	interp.Interpret(prog)

	hasErrors := false

	for _, e := range l.Errors {
		fmt.Fprintf(os.Stderr, "%s:%s\n", file, e.Error())
		hasErrors = true
	}
	for _, e := range p.Errors {
		fmt.Fprintf(os.Stderr, "%s:%s\n", file, e.Error())
		hasErrors = true
	}
	for _, e := range interp.Errors {
		fmt.Fprintf(os.Stderr, "%s:%s\n", file, e.Error())
		hasErrors = true
	}

	if hasErrors {
		os.Exit(1)
	}
}

func writeJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "json error: %v\n", err)
		os.Exit(1)
	}
}
