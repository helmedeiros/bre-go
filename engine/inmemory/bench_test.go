package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func BenchmarkExecuteOverTenRules(b *testing.B) {
	e := tenRuleEngine()
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteWithListenerOverTenRules(b *testing.B) {
	e := tenRuleEngine()
	e.AddListener(observability.NopExecutionListener{})
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func tenRuleEngine() *inmemory.Engine {
	e := inmemory.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(inmemory.Rule{
			Name:      ruleName(i),
			Condition: func(in interface{}) bool { return in.(int) >= i },
			Action:    func(in interface{}) interface{} { return in.(int) + 1 },
		})
	}
	return e
}

func ruleName(i int) string {
	return "rule-" + string(rune('A'+i))
}
