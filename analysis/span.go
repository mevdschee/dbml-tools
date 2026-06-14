package analysis

import (
	"sort"

	"github.com/mevdschee/dbml-tools/parser"
)

// SpanIndex maps a rune offset to the chain of AST nodes that contain it.
type SpanIndex struct {
	spans []span // sorted by Start asc, End desc
}

type span struct {
	Start int
	End   int
	Node  parser.Node
}

// BuildSpanIndex walks the program and indexes every node by its rune range.
func BuildSpanIndex(prog *parser.ProgramNode) *SpanIndex {
	idx := &SpanIndex{}
	if prog == nil {
		return idx
	}
	for _, c := range prog.Body {
		idx.walk(c)
	}
	sort.SliceStable(idx.spans, func(i, j int) bool {
		a, b := idx.spans[i], idx.spans[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.End > b.End
	})
	return idx
}

func (s *SpanIndex) walk(n parser.Node) {
	if n == nil {
		return
	}
	s.spans = append(s.spans, span{
		Start: n.FirstToken().Start,
		End:   n.LastToken().End,
		Node:  n,
	})
	switch node := n.(type) {
	case *parser.ElementDeclNode:
		s.walk(node.Name)
		s.walk(node.Alias)
		s.walk(node.Attrs)
		s.walk(node.Body)
	case *parser.BlockExprNode:
		for _, c := range node.Body {
			s.walk(c)
		}
	case *parser.FuncAppNode:
		s.walk(node.Callee)
		for _, a := range node.Args {
			s.walk(a)
		}
	case *parser.PrimaryExprNode:
		s.walk(node.Expr)
	case *parser.InfixExprNode:
		s.walk(node.Left)
		s.walk(node.Right)
	case *parser.ListExprNode:
		for _, it := range node.Items {
			s.walk(it)
		}
	case *parser.TupleExprNode:
		for _, it := range node.Items {
			s.walk(it)
		}
	case *parser.AttributeNode:
		s.walk(node.Name)
		s.walk(node.Value)
	}
}

// Innermost returns the chain of nodes containing offset, outermost first.
func (s *SpanIndex) Innermost(offset int) []parser.Node {
	var chain []parser.Node
	for _, sp := range s.spans {
		if sp.Start > offset {
			break
		}
		if offset >= sp.Start && offset <= sp.End {
			chain = append(chain, sp.Node)
		}
	}
	return chain
}

// Leaf returns the innermost node (last in Innermost). May be nil.
func (s *SpanIndex) Leaf(offset int) parser.Node {
	chain := s.Innermost(offset)
	if len(chain) == 0 {
		return nil
	}
	return chain[len(chain)-1]
}
