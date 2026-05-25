package analysis

// Definition returns the NameRange of the symbol declared at the use site
// under offset. If offset is on a declaration, returns that declaration's
// NameRange. Returns nil for unresolved or non-symbol positions.
func (a *Analysis) Definition(offset int) *Range {
	// Use site?
	if r := a.RefAt(offset); r != nil {
		if r.Target != nil {
			out := r.Target.NameRange
			return &out
		}
		return nil
	}
	// Declaration site?
	if sym := a.symbolAtDecl(offset); sym != nil {
		// For aliases, jump to the underlying table's declaration.
		if sym.Kind == SymAlias && sym.Parent != nil {
			out := sym.Parent.NameRange
			return &out
		}
		out := sym.NameRange
		return &out
	}
	return nil
}

// ReferencesOf returns the SiteRange of every ResolvedRef whose target is
// the given symbol. If includeDecl is true, the declaration's own NameRange
// is included.
//
// For tables, this also returns sites that go through an alias.
func (a *Analysis) ReferencesOf(sym *Symbol, includeDecl bool) []Range {
	if sym == nil {
		return nil
	}
	// Aliases: caller may pass the alias symbol; convert to the underlying table.
	if sym.Kind == SymAlias && sym.Parent != nil {
		sym = sym.Parent
	}
	var out []Range
	if includeDecl {
		out = append(out, sym.NameRange)
	}
	for _, r := range a.Refs {
		if r.Target == sym {
			out = append(out, r.SiteRange)
		}
	}
	return out
}
