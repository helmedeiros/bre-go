package priority_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func BenchmarkExecuteHighestPriorityMatchesOverTenRules(b *testing.B) {
	e := tenRuleEngine(b, true)
	ctx := context.Background()
	req := engine.Request{Input: 9}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

func BenchmarkExecuteLowestPriorityMatchesOverTenRules(b *testing.B) {
	e := tenRuleEngine(b, false)
	ctx := context.Background()
	req := engine.Request{Input: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}

// tenRuleEngine builds an engine with ten rules priced 0..9 by
// priority. If highMatches is true the highest-priority rule (9)
// matches input == 9; otherwise the lowest-priority rule (0) matches
// input == 0 and the engine walks through all nine higher rules
// before finding it.
func tenRuleEngine(tb testing.TB, highMatches bool) *priority.Engine {
	tb.Helper()
	e := priority.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(priority.Rule{
			Name:      ruleName(i),
			Priority:  i,
			Condition: func(in interface{}) bool { return in.(int) == i },
			Action:    func(in interface{}) interface{} { return in },
		})
	}
	_ = highMatches
	return e
}

func ruleName(i int) string {
	return "rule-" + string(rune('A'+i))
}
