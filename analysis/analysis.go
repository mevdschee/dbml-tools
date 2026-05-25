// Package analysis builds a semantic index of a DBML source file suitable for
// driving LSP requests (hover, completion, definition, references, rename).
//
// All positions are *rune* offsets matching lexer.Token.Start/End. The LSP
// wire layer is responsible for converting them to UTF-16 line/column.
package analysis

import (
	"dbml-tools/interpreter"
	"dbml-tools/lexer"
	"dbml-tools/parser"
)

// Range is a half-open rune offset range [Start, End).
type Range struct {
	Start int
	End   int
}

func tokenRange(t lexer.Token) Range { return Range{t.Start, t.End} }

// Position is a 0-based line/column pair derived from a rune offset.
type Position struct {
	Line   int
	Column int
}

// Analysis is the full semantic snapshot of a source file.
type Analysis struct {
	Source   string
	Tokens   []lexer.Token
	Program  *parser.ProgramNode
	Database *interpreter.Database

	Symbols *SymbolIndex
	Refs    []ResolvedRef
	Spans   *SpanIndex

	LineOffsets []int // rune offset of each line start

	LexErrors    []lexer.Error
	ParseErrors  []parser.Error
	InterpErrors []interpreter.Error
}

// Analyze runs the full lex/parse/interpret pipeline and builds all indices.
func Analyze(source string) *Analysis {
	l := lexer.New(source)
	tokens := l.Lex()
	p := parser.New(tokens, source)
	prog := p.Parse()
	interp := interpreter.New()
	db := interp.Interpret(prog)

	a := &Analysis{
		Source:       source,
		Tokens:       tokens,
		Program:      prog,
		Database:     db,
		LineOffsets:  computeLineOffsets(source),
		LexErrors:    l.Errors,
		ParseErrors:  p.Errors,
		InterpErrors: interp.Errors,
	}
	a.Symbols = BuildSymbolIndex(prog, db)
	a.Spans = BuildSpanIndex(prog)
	a.Refs = ResolveRefs(prog, a.Symbols)
	return a
}

// ---------------------------------------------------------------------------
// Offset ↔ line/column helpers
// ---------------------------------------------------------------------------

func computeLineOffsets(src string) []int {
	offsets := []int{0}
	i := 0
	for _, r := range src {
		i++
		if r == '\n' {
			offsets = append(offsets, i)
		}
	}
	return offsets
}

// OffsetToPosition converts a rune offset to a 0-based Line/Column.
func (a *Analysis) OffsetToPosition(offset int) Position {
	lo := a.LineOffsets
	// binary search for the greatest lo[i] <= offset
	low, high := 0, len(lo)-1
	for low < high {
		mid := (low + high + 1) / 2
		if lo[mid] <= offset {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return Position{Line: low, Column: offset - lo[low]}
}
