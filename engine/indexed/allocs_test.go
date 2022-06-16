package indexed_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// TestExecuteAllocCount is the allocation tripwire for indexed.Execute.
// See ADR-0032: the constant freezes the exact alloc count of the hot
// path at a small, fixed workload. Intentional changes update the
// constant in the same commit; unintentional changes fire the test.
//
// Workload: 10 rules, single-condition equality on "k" with unique
// values per rule; input matches the last rule (worst-case scan
// inside the bucket -- but indexed lookup short-circuits the
// scan-rest entirely).
func TestExecuteAllocCount(t *testing.T) {
	e := buildAllocsEngine()
	req := engine.Request{Input: map[string]string{"k": "v9"}}
	ctx := context.Background()

	n := testing.AllocsPerRun(100, func() {
		_, _ = e.Execute(ctx, req)
	})

	const want = 2 // Frozen 2022-06-16 (projectFact's strings.Builder + Result.Matched slice).
	if int(n) != want {
		t.Fatalf("Execute allocs: want %d, got %.0f", want, n)
	}
}

func buildAllocsEngine() *indexed.Engine {
	e := indexed.New()
	for i := 0; i < 10; i++ {
		v := "v" + string(rune('0'+i))
		_ = e.AddRule(indexed.Rule{
			Name:  "rule-" + string(rune('A'+i)),
			Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: v},
		})
	}
	return e
}
