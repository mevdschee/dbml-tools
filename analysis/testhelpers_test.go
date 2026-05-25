package analysis

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// CursorMarker is the rune used in test fixtures to mark a cursor position.
const CursorMarker = '▮'

// stripCursor finds the first CursorMarker in src, removes it, and returns the
// remaining source plus the rune offset where the marker was.
//
// Multiple markers in a single fixture are not supported; the first wins.
func stripCursor(t *testing.T, src string) (string, int) {
	t.Helper()
	idx := strings.IndexRune(src, CursorMarker)
	if idx < 0 {
		t.Fatalf("no cursor marker %q in fixture:\n%s", string(CursorMarker), src)
	}
	// Count runes before the marker.
	prefix := src[:idx]
	offset := utf8.RuneCountInString(prefix)
	cleaned := src[:idx] + src[idx+utf8.RuneLen(CursorMarker):]
	return cleaned, offset
}

// allCursors returns every cursor offset in `src` (markers stripped) keyed by
// declaration order. Used for fixtures with multiple cursors.
func allCursors(t *testing.T, src string) (string, []int) {
	t.Helper()
	var offsets []int
	var b strings.Builder
	runes := []rune(src)
	pos := 0
	for _, r := range runes {
		if r == CursorMarker {
			offsets = append(offsets, pos)
			continue
		}
		b.WriteRune(r)
		pos++
	}
	if len(offsets) == 0 {
		t.Fatalf("no cursor markers in fixture:\n%s", src)
	}
	return b.String(), offsets
}

// analyzeWithCursor strips a cursor marker, runs Analyze, and returns both.
func analyzeWithCursor(t *testing.T, src string) (*Analysis, int) {
	t.Helper()
	clean, off := stripCursor(t, src)
	return Analyze(clean), off
}

// sub returns the rune slice src[r.Start:r.End].
func sub(src string, r Range) string {
	runes := []rune(src)
	if r.Start < 0 || r.End > len(runes) || r.Start > r.End {
		return ""
	}
	return string(runes[r.Start:r.End])
}
