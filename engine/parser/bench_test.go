package parser_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

func BenchmarkParseSimpleEquality(b *testing.B) {
	expr := `origin == "DE"`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(expr)
	}
}

func BenchmarkParseComplexExpression(b *testing.B) {
	expr := `origin == "DE" AND (tier IN ("vip", "premium") OR partner == "preferred") AND NOT platform == "mobile"`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(expr)
	}
}

func BenchmarkPredicateEvaluation(b *testing.B) {
	pred, _ := parser.Parse(`origin == "DE" AND tier IN ("vip", "premium")`)
	fact := map[string]interface{}{"origin": "DE", "tier": "vip"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pred(fact)
	}
}

func BenchmarkParseAndEvaluate(b *testing.B) {
	expr := `origin == "DE" AND tier IN ("vip", "premium")`
	fact := map[string]interface{}{"origin": "DE", "tier": "vip"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pred, _ := parser.Parse(expr)
		_ = pred(fact)
	}
}
