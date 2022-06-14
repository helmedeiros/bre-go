//go:build stress

package priority_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
	"github.com/helmedeiros/bre-go/observability"
)

func TestExecuteSurvivesHighVolume(t *testing.T) {
	e := buildStressEngine()
	req := engine.Request{Input: 9}
	ctx := context.Background()

	before := runtime.NumGoroutine()
	for i := 0; i < 100_000; i++ {
		res, err := e.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Execute iter %d: %v", i, err)
		}
		if len(res.Matched) == 0 {
			t.Fatalf("Execute iter %d: empty Matched", i)
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

	req := engine.Request{Input: 9}
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

func buildStressEngine() *priority.Engine {
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
