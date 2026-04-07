package main

import (
	"encoding/json"
	"fmt"
	"os"

	"dbml-tools/interpreter"
	"dbml-tools/lexer"
	"dbml-tools/parser"
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: dbml-tools <command> <file>\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  lex        Tokenize and output lexer JSON\n")
	fmt.Fprintf(os.Stderr, "  parse      Parse and output AST JSON\n")
	fmt.Fprintf(os.Stderr, "  interpret  Interpret and output database schema JSON\n")
	fmt.Fprintf(os.Stderr, "  check      Check for parse errors\n")
	os.Exit(1)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	cmd := os.Args[1]
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
	case "check":
		doCheck(file, source)
	default:
		usage()
	}
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

func doCheck(file, source string) {
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
