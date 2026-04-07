package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"strings"

	"dbml-tools/introspect"
)

//go:embed sql
var sqlFiles embed.FS

func doIntrospect(args []string) {
	fs := flag.NewFlagSet("introspect", flag.ExitOnError)
	normalize := fs.Bool("normalize", false, "normalize column types to database-agnostic DBML equivalents")
	excludeStr := fs.String("exclude", "", "comma-separated table name patterns to exclude (supports * glob)")
	includeStr := fs.String("include", "", "comma-separated table name patterns to include (all others excluded)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: dbml-tools introspect [--normalize] [--exclude pattern,...] [--include pattern,...] <dsn>\n")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	opts := introspect.Options{
		Exclude: parseCSV(*excludeStr),
		Include: parseCSV(*includeStr),
	}

	dbml, err := introspect.Run(fs.Arg(0), sqlFiles, opts, *normalize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(dbml)
}

func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
