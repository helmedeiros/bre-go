package priority_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

// TestExecuteAllocCount is the allocation tripwire for priority.Execute.
// See ADR-0032. priority allocates more than firstmatch / inmemory
// because it copies the rule set into a heap-ordered queue on every
// Execute -- the constant captures that overhead so it cannot grow
// silently.
//
// Workload: 10 rules with equal priority; input matches the last
// rule. Equal priorities preserve insertion-order tie-break
// (ADR-0019).
func TestExecuteAllocCount(t *testing.T) {
	e := buildAllocsEngine()
	req := engine.Request{Input: 9}
	ctx := context.Background()

	n := testing.AllocsPerRun(100, func() {
		_, _ = e.Execute(ctx, req)
	})

	const want = 5 // Frozen 2022-06-13 (per-Execute heap-ordered queue + Result slice).
	if int(n) != want {
		t.Fatalf("Execute allocs: want %d, got %.0f", want, n)
	}
}

func buildAllocsEngine() *priority.Engine {
	e := priority.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(priority.Rule{
			Name:      "rule-" + string(rune('A'+i)),
			Priority:  1,
			Condition: func(in interface{}) bool { return in.(int) == i },
		})
	}
	return e
}
