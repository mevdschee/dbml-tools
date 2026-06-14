package lsp

import (
	"unicode/utf16"

	"github.com/mevdschee/dbml-tools/analysis"
)

// document holds the source text along with a precomputed line offsets table
// and per-line rune→UTF-16 mappings, so we can convert offsets quickly.
type document struct {
	uri      string
	version  int
	text     string
	lines    []int  // rune offset of each line start
	runes    []rune // cached []rune(text)
	analysis *analysis.Analysis
}

func newDocument(uri string, version int, text string) *document {
	d := &document{uri: uri, version: version, text: text}
	d.update(text)
	return d
}

func (d *document) update(text string) {
	d.text = text
	d.runes = []rune(text)
	d.lines = computeLineOffsets(d.runes)
	d.analysis = analysis.Analyze(text)
}

func computeLineOffsets(runes []rune) []int {
	offsets := []int{0}
	for i, r := range runes {
		if r == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// runeOffsetToPosition converts a rune offset to LSP (line, UTF-16 character).
func (d *document) runeOffsetToPosition(off int) Position {
	if off < 0 {
		off = 0
	}
	if off > len(d.runes) {
		off = len(d.runes)
	}
	// binary search for greatest line start <= off
	lo, hi := 0, len(d.lines)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if d.lines[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	lineStart := d.lines[lo]
	// Count UTF-16 code units between [lineStart, off).
	col := 0
	for i := lineStart; i < off; i++ {
		r := d.runes[i]
		col += utf16Len(r)
	}
	return Position{Line: lo, Character: col}
}

// positionToRuneOffset converts an LSP (line, UTF-16 character) to a rune offset.
func (d *document) positionToRuneOffset(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(d.lines) {
		return len(d.runes)
	}
	lineStart := d.lines[p.Line]
	var lineEnd int
	if p.Line+1 < len(d.lines) {
		lineEnd = d.lines[p.Line+1] - 1
	} else {
		lineEnd = len(d.runes)
	}
	// Walk runes adding UTF-16 lengths until reaching p.Character.
	col := 0
	for i := lineStart; i < lineEnd; i++ {
		if col >= p.Character {
			return i
		}
		col += utf16Len(d.runes[i])
	}
	return lineEnd
}

func utf16Len(r rune) int {
	r1, _ := utf16.EncodeRune(r)
	if r1 == 0xFFFD && (r < 0x10000) {
		return 1
	}
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// analysisRange converts an analysis.Range (rune offsets) to an LSPRange.
func (d *document) analysisRange(r analysis.Range) LSPRange {
	return LSPRange{
		Start: d.runeOffsetToPosition(r.Start),
		End:   d.runeOffsetToPosition(r.End),
	}
}
