package inmemory_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func BenchmarkExecuteOverTenRules(b *testing.B) {
	e := tenRuleEngine()
	ctx := context.Background()
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteWithCancellableContext(b *testing.B) {
	e := tenRuleEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteRecoveryOverhead(b *testing.B) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "noop",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in },
	})
	ctx := context.Background()
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteWithListenerOverTenRules(b *testing.B) {
	e := tenRuleEngine()
	e.AddListener(observability.NopExecutionListener{})
	ctx := context.Background()
	req := engine.Request{Input: 5}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkRuleNamesOverTenRules(b *testing.B) {
	e := tenRuleEngine()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.RuleNames()
	}
}

func BenchmarkRuleNamesOverHundredRules(b *testing.B) {
	e := nRuleEngine(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.RuleNames()
	}
}

func BenchmarkRuleInfosOverTenRules(b *testing.B) {
	e := tenRuleEngineWithTags()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.RuleInfos()
	}
}

func BenchmarkRuleInfosOverHundredRules(b *testing.B) {
	e := nRuleEngineWithTags(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.RuleInfos()
	}
}

func tenRuleEngineWithTags() *inmemory.Engine {
	return nRuleEngineWithTags(10)
}

func nRuleEngineWithTags(n int) *inmemory.Engine {
	e := inmemory.New()
	for i := 0; i < n; i++ {
		i := i
		_ = e.AddRule(inmemory.Rule{
			Name:        "rule-" + fmtInt(i),
			Description: "auto-generated rule",
			Tags:        []string{"benchmark", "synthetic"},
			Condition:   func(in interface{}) bool { return in.(int) >= i },
		})
	}
	return e
}

func nRuleEngine(n int) *inmemory.Engine {
	e := inmemory.New()
	for i := 0; i < n; i++ {
		i := i
		_ = e.AddRule(inmemory.Rule{
			Name:      "rule-" + fmtInt(i),
			Condition: func(in interface{}) bool { return in.(int) >= i },
		})
	}
	return e
}

func fmtInt(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b [10]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = digits[i%10]
		i /= 10
	}
	return string(b[pos:])
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
