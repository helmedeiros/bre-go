//go:build stress

package inmemory_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

// TestExecuteSurvivesHighVolume runs Execute 100k times against a
// populated engine, asserting no panic, no error, and stable goroutine
// count. Build-tagged so it does not slow `make test`; run via
// `make stress`.
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
			t.Fatalf("Execute iter %d: empty Matched (expected last rule)", i)
		}
	}
	after := runtime.NumGoroutine()
	if after > before {
		t.Fatalf("goroutine count drifted: before=%d after=%d", before, after)
	}
}

// TestNoGoroutineLeakUnderListenerFanout attaches several listeners
// and runs Execute 10k times; goroutine count must stay flat because
// listener dispatch is synchronous (per ADR-0029's Notifier).
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

func buildStressEngine() *inmemory.Engine {
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
