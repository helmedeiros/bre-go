package firstmatch_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/observability"
)

func BenchmarkExecuteFirstRuleMatches(b *testing.B) {
	e := tenRuleEngine(b)
	req := engine.Request{Input: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteLastRuleMatches(b *testing.B) {
	e := tenRuleEngine(b)
	req := engine.Request{Input: 9}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteWithListener(b *testing.B) {
	e := tenRuleEngine(b)
	e.AddListener(observability.NopExecutionListener{})
	req := engine.Request{Input: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func tenRuleEngine(tb testing.TB) *firstmatch.Engine {
	tb.Helper()
	e := firstmatch.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(firstmatch.Rule{
			Name:      ruleName(i),
			Condition: func(in interface{}) bool { return in.(int) == i },
			Action:    func(in interface{}) interface{} { return in.(int) + 1 },
		})
	}
	return e
}

func ruleName(i int) string {
	return "rule-" + string(rune('A'+i))
}
