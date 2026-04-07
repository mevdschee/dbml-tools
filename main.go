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
	fmt.Fprintf(os.Stderr, "  check      [-d driver] <file>           Check for parse/semantic errors\n")
	fmt.Fprintf(os.Stderr, "  introspect <dsn>                        Connect to a database and output DBML\n")
	fmt.Fprintf(os.Stderr, "  dump       [-d driver] <file>           Generate CREATE TABLE SQL\n")
	fmt.Fprintf(os.Stderr, "  migrate    [-d driver] [--apply] <old> <new>  Generate migration SQL\n")
	fmt.Fprintf(os.Stderr, "\nDrivers: postgres, mysql, sqlite (omit for generic/normalized mode)\n")
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
	case "introspect":
		doIntrospect(os.Args[2:])
	case "dump":
		doDump(os.Args[2:])
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

func doDump(args []string) {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	dialectStr := fs.String("dialect", "", "SQL dialect: postgres, mysql, sqlite (omit for generic passthrough)")
	fs.StringVar(dialectStr, "d", "", "SQL dialect shorthand (same as --dialect)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools dump [-d postgres|mysql|sqlite] <file>\n")
		fmt.Fprintf(os.Stderr, "       Without -d, types are emitted as-is (generic/passthrough mode).\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	d, err := generator.ParseDialect(*dialectStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	db := parseAndInterpret(readFile(fs.Arg(0)))
	fmt.Print(generator.Dump(db, d))
}

func doMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dialectStr := fs.String("dialect", "", "SQL dialect: postgres, mysql, sqlite (auto-detected from DSN if omitted)")
	fs.StringVar(dialectStr, "d", "", "SQL dialect shorthand (same as --dialect)")
	apply := fs.Bool("apply", false, "remove dry-run header (statements are ready to apply)")
	excludeStr := fs.String("exclude", "", "comma-separated table name patterns to exclude (supports * glob)")
	includeStr := fs.String("include", "", "comma-separated table name patterns to include (all others excluded)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools migrate [-d postgres|mysql|sqlite] [--apply] [--exclude pattern,...] <old> <new>\n")
		fmt.Fprintf(os.Stderr, "       <old> and <new> may each be a .dbml file path or a connection string.\n")
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

	// Resolve dialect: explicit flag > auto-detect from DSN > default postgres.
	d, err := resolveDialect(*dialectStr, oldArg, newArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
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

	fmt.Print(generator.Migrate(oldDB, newDB, d, !*apply))
}

// loadSchema reads a schema from either a DBML file path or a live database DSN.
func loadSchema(arg string, opts introspect.Options) (*interpreter.Database, error) {
	if _, err := introspect.ParseDSN(arg); err == nil {
		dbml, err := introspect.Run(arg, sqlFiles, opts)
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

// resolveDialect determines the dialect from explicit flag, DSN auto-detect, or default.
func resolveDialect(explicit, arg1, arg2 string) (generator.Dialect, error) {
	if explicit != "" {
		return generator.ParseDialect(explicit)
	}
	// Auto-detect from first DSN found.
	for _, arg := range []string{arg1, arg2} {
		if parsed, err := introspect.ParseDSN(arg); err == nil {
			switch parsed.Engine {
			case introspect.EnginePostgres:
				return generator.Postgres, nil
			case introspect.EngineMariaDB:
				return generator.MySQL, nil
			case introspect.EngineSQLite:
				return generator.SQLite, nil
			}
		}
	}
	return generator.Postgres, nil // default
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
	dialectStr := fs.String("d", "", "database driver for type validation: postgres, mysql, sqlite (omit to accept any type)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools check [-d postgres|mysql|sqlite] <file>\n")
		fmt.Fprintf(os.Stderr, "       Without -d, column types are accepted as-is without validation.\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	// Validate -d value when given.
	if *dialectStr != "" {
		if _, err := generator.ParseDialect(*dialectStr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
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

	_ = prog // suppress unused
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
