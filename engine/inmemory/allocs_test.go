package inmemory_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

// TestExecuteAllocCount is the allocation tripwire for inmemory.Execute.
// See ADR-0032: the constant freezes the exact alloc count of the hot
// path at a small, fixed workload. Any change to the engine that
// introduces or removes an allocation in the rule loop fires this
// test. Intentional changes update the constant in the same commit.
//
// Workload: 10 rules, single-condition equality on int; input matches
// the last rule (visits all 10 rules).
func TestExecuteAllocCount(t *testing.T) {
	e := buildAllocsEngine()
	req := engine.Request{Input: 9}
	ctx := context.Background()

	n := testing.AllocsPerRun(100, func() {
		_, _ = e.Execute(ctx, req)
	})

	const want = 1 // Frozen 2022-06-13 (the Matched slice growing from nil to len=1).
	if int(n) != want {
		t.Fatalf("Execute allocs: want %d, got %.0f", want, n)
	}
}

func buildAllocsEngine() *inmemory.Engine {
	e := inmemory.New()
	for i := 0; i < 10; i++ {
		i := i
		_ = e.AddRule(inmemory.Rule{
			Name:      "rule-" + string(rune('A'+i)),
			Condition: func(in interface{}) bool { return in.(int) == i },
		})
	}
	return e
}
