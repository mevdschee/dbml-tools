package analysis

import (
	"errors"
	"fmt"
	"regexp"
)

// TextEdit is a single text-replacement edit.
type TextEdit struct {
	Range   Range
	NewText string
}

// PrepareRenameResult is what `textDocument/prepareRename` returns.
type PrepareRenameResult struct {
	Range       Range
	Placeholder string
}

// PrepareRename checks whether the symbol at offset is renameable and returns
// the range to highlight + the suggested placeholder.
func (a *Analysis) PrepareRename(offset int) (*PrepareRenameResult, error) {
	sym, _ := a.resolveRenameTarget(offset)
	if sym == nil {
		return nil, errors.New("nothing renameable at cursor")
	}
	switch sym.Kind {
	case SymTable, SymColumn, SymEnum, SymEnumValue, SymAlias, SymTableGroup, SymRefName:
		// renameable
	default:
		return nil, errors.New("symbol is not renameable")
	}
	// Build the range from the current token under cursor (use-site or decl).
	var rng Range
	if r := a.RefAt(offset); r != nil {
		rng = r.SiteRange
	} else if local := a.symbolAtDecl(offset); local != nil {
		rng = local.NameRange
	} else {
		rng = sym.NameRange
	}
	return &PrepareRenameResult{Range: rng, Placeholder: sym.Name}, nil
}

// Rename returns the edits to rename the symbol at offset to newName.
func (a *Analysis) Rename(offset int, newName string) ([]TextEdit, error) {
	if err := validateRenameName(newName); err != nil {
		return nil, err
	}
	sym, _ := a.resolveRenameTarget(offset)
	if sym == nil {
		return nil, errors.New("nothing renameable at cursor")
	}
	switch sym.Kind {
	case SymTable, SymColumn, SymEnum, SymEnumValue, SymTableGroup, SymRefName:
		// renameable
	case SymAlias:
		// rename the alias only — TODO: full alias rename is a future feature
		return nil, errors.New("alias rename is not yet supported")
	default:
		return nil, errors.New("symbol is not renameable")
	}

	replacement := quoteIfNeeded(newName)

	var edits []TextEdit
	// Declaration name.
	edits = append(edits, TextEdit{Range: sym.NameRange, NewText: replacement})

	// All ref sites that resolve to this symbol.
	for _, r := range a.Refs {
		if r.Target == sym {
			edits = append(edits, TextEdit{Range: r.SiteRange, NewText: replacement})
		}
	}
	return edits, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveRenameTarget returns the symbol to rename for cursor at offset.
// If on a ref use site, returns the target. If on a declaration, returns it.
// If on a keyword or builtin type, returns nil.
func (a *Analysis) resolveRenameTarget(offset int) (*Symbol, *ResolvedRef) {
	if r := a.RefAt(offset); r != nil {
		return r.Target, r
	}
	if sym := a.symbolAtDecl(offset); sym != nil {
		return sym, nil
	}
	return nil, nil
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateRenameName(s string) error {
	if s == "" {
		return errors.New("new name is empty")
	}
	// Reject any whitespace character.
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("new name %q contains whitespace", s)
		}
	}
	// Reject a leading digit (won't tokenize as identifier; quoting can't fix).
	if s[0] >= '0' && s[0] <= '9' {
		return fmt.Errorf("new name %q starts with a digit", s)
	}
	return nil
}

func quoteIfNeeded(s string) string {
	if identRe.MatchString(s) {
		return s
	}
	return `"` + s + `"`
}
