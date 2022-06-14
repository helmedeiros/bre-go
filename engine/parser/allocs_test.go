package parser_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// TestParseAllocCount is the allocation tripwire for parser.Parse on
// a canonical small expression. See ADR-0032: the constant freezes
// the exact alloc count so that a regression in the lexer or
// recursive-descent path fires this test rather than hiding in a
// bench delta.
//
// Expression: country == "BR" -- a single equality condition. Small
// enough to be stable, large enough to exercise the lex + parse + tree
// build paths.
func TestParseAllocCount(t *testing.T) {
	const expr = `country == "BR"`

	n := testing.AllocsPerRun(100, func() {
		_, _ = parser.Parse(expr)
	})

	const want = 7 // Frozen 2022-06-13 (lexer + parse + Predicate wrapper for a single equality).
	if int(n) != want {
		t.Fatalf("Parse allocs: want %d, got %.0f", want, n)
	}
}

// TestParseToConditionAllocCount tracks the typed-tree variant
// separately -- ParseToCondition builds the AST without the
// Predicate-shaped wrapper around it, so the count differs.
func TestParseToConditionAllocCount(t *testing.T) {
	const expr = `country == "BR"`

	n := testing.AllocsPerRun(100, func() {
		_, _ = parser.ParseToCondition(expr)
	})

	const want = 6 // Frozen 2022-06-13 (lexer + parse + typed Condition node, no Predicate wrapper).
	if int(n) != want {
		t.Fatalf("ParseToCondition allocs: want %d, got %.0f", want, n)
	}
}
