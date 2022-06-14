package firstmatch_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

// TestExecuteAllocCount is the allocation tripwire for
// firstmatch.Execute. See ADR-0032: the constant freezes the exact
// alloc count of the hot path. Intentional changes update the
// constant in the same commit; unintentional changes fire the test.
//
// Workload: 10 rules, single-condition equality on int; input matches
// the last rule (worst-case scan -- visits all 10 rules before
// returning).
func TestExecuteAllocCount(t *testing.T) {
	e := buildAllocsEngine()
	req := engine.Request{Input: 9}
	ctx := context.Background()

	n := testing.AllocsPerRun(100, func() {
		_, _ = e.Execute(ctx, req)
	})

	const want = 1 // Frozen 2022-06-13 (Result.Matched slice for the single matching rule).
	if int(n) != want {
		t.Fatalf("Execute allocs: want %d, got %.0f", want, n)
	}
}

func buildAllocsEngine() *firstmatch.Engine {
	e := firstmatch.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(firstmatch.Rule{
			Name:      "rule-" + string(rune('A'+i)),
			Condition: func(in interface{}) bool { return in.(int) == i },
		})
	}
	return e
}
