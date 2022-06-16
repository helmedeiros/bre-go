//go:build stress

package indexed_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/observability"
)

func TestExecuteSurvivesHighVolume(t *testing.T) {
	e := buildStressEngine()
	req := engine.Request{Input: map[string]string{"k": "v9"}}
	ctx := context.Background()

	before := runtime.NumGoroutine()
	for i := 0; i < 100_000; i++ {
		res, err := e.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Execute iter %d: %v", i, err)
		}
		if len(res.Matched) == 0 {
			t.Fatalf("Execute iter %d: empty Matched (expected last rule)", i)
		}
	}
	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine count drifted: before=%d after=%d", before, after)
	}
}

func TestNoGoroutineLeakUnderListenerFanout(t *testing.T) {
	e := buildStressEngine()
	for i := 0; i < 5; i++ {
		e.AddListener(&observability.SnapshotListener{})
	}

	req := engine.Request{Input: map[string]string{"k": "v9"}}
	ctx := context.Background()

	before := runtime.NumGoroutine()
	for i := 0; i < 10_000; i++ {
		if _, err := e.Execute(ctx, req); err != nil {
			t.Fatalf("Execute iter %d: %v", i, err)
		}
	}
	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine leak detected: before=%d after=%d", before, after)
	}
}

func buildStressEngine() *indexed.Engine {
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
