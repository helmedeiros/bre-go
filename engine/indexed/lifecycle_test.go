package indexed_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/observability"
)

// ADR-0037 lifecycle tests: Build, Built, AddRule-after-Build,
// WithPostFilterHook-after-Build, implicit Build on first Execute.

func TestNewEngineIsNotBuilt(t *testing.T) {
	e := indexed.New()
	if e.Built() {
		t.Fatalf("a freshly constructed engine should not be Built")
	}
}

func TestBuildMarksEngineBuilt(t *testing.T) {
	e := indexed.New()
	if err := e.Build(); err != nil {
		t.Fatalf("Build on empty engine: unexpected error: %v", err)
	}
	if !e.Built() {
		t.Fatalf("Build should mark engine Built")
	}
}

func TestBuildTwiceReturnsAlreadyBuilt(t *testing.T) {
	e := indexed.New()
	_ = e.Build()
	err := e.Build()
	if !errors.Is(err, indexed.ErrAlreadyBuilt) {
		t.Fatalf("want ErrAlreadyBuilt, got %v", err)
	}
}

func TestAddRuleAfterBuildReturnsErrEngineBuilt(t *testing.T) {
	e := indexed.New()
	_ = e.Build()
	err := e.AddRule(indexed.Rule{
		Name:  "after-build",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if !errors.Is(err, indexed.ErrEngineBuilt) {
		t.Fatalf("want ErrEngineBuilt, got %v", err)
	}
}

func TestImplicitBuildOnFirstExecute(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "implicit",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if e.Built() {
		t.Fatalf("Built should be false before any Execute")
	}

	_, _ = e.Execute(context.Background(), engine.Request{Input: map[string]string{"k": "v"}})

	if !e.Built() {
		t.Fatalf("implicit Build should have run on first Execute")
	}

	// AddRule must now error.
	err := e.AddRule(indexed.Rule{
		Name:  "after-implicit",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "x"},
	})
	if !errors.Is(err, indexed.ErrEngineBuilt) {
		t.Fatalf("AddRule after implicit Build: want ErrEngineBuilt, got %v", err)
	}
}

func TestWithPostFilterHookAfterBuildPanics(t *testing.T) {
	e := indexed.New()
	_ = e.Build()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("WithPostFilterHook on built engine should have panicked")
		}
	}()
	e.WithPostFilterHook(func(parser.Condition) bool { return true })
}

func TestRuleNamesWorksInBothPhases(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "before-build",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	// Pre-Build: RuleNames reads from builder.
	if names := e.RuleNames(); len(names) != 1 || names[0] != "before-build" {
		t.Fatalf("RuleNames (builder): want [before-build], got %v", names)
	}

	_ = e.Build()

	// Post-Build: RuleNames reads from snapshot.
	if names := e.RuleNames(); len(names) != 1 || names[0] != "before-build" {
		t.Fatalf("RuleNames (snapshot): want [before-build], got %v", names)
	}
}

func TestRuleInfosWorksInBothPhases(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:        "with-desc",
		Description: "test",
		Tags:        []string{"a", "b"},
		Match:       parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	infos := e.RuleInfos()
	if len(infos) != 1 || infos[0].Description != "test" || len(infos[0].Tags) != 2 {
		t.Fatalf("RuleInfos (builder): %+v", infos)
	}

	_ = e.Build()

	infos = e.RuleInfos()
	if len(infos) != 1 || infos[0].Description != "test" {
		t.Fatalf("RuleInfos (snapshot): %+v", infos)
	}
}

// ----- Concurrent Execute (ADR-0037 §3) -------------------------------

func TestConcurrentExecuteSafe(t *testing.T) {
	e := indexed.New()
	// Populate a small but multi-key-set rule set.
	_ = e.AddRule(indexed.Rule{
		Name:  "br",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "br-premium",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})
	if err := e.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	const goroutines = 16
	const itersPer = 500
	ctx := context.Background()
	req := engine.Request{Input: map[string]string{"country": "BR", "tier": "premium"}}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < itersPer; i++ {
				res, err := e.Execute(ctx, req)
				if err != nil {
					t.Errorf("Execute: %v", err)
					return
				}
				if len(res.Matched) != 1 {
					t.Errorf("expected exactly one match, got %v", res.Matched)
					return
				}
			}
		}()
	}
	wg.Wait()
	// The race detector (via `go test -race`) is the actual assertion:
	// any unsynchronized access during this concurrent walk would
	// flag here.
}

func TestConcurrentExecuteWithImplicitBuild(t *testing.T) {
	// Without an explicit Build, many goroutines race to trigger
	// implicit Build on first Execute. Use a release-barrier so
	// every goroutine starts simultaneously -- this guarantees the
	// double-checked-locking recheck inside readSnapshot fires
	// (one goroutine wins the seal, others observe the snapshot
	// already set under mu).
	//
	// Repeat the experiment multiple times to make the race-window
	// observation deterministic under -race's scheduler.
	const goroutines = 32
	const itersPer = 50
	const repeats = 50
	ctx := context.Background()
	req := engine.Request{Input: map[string]string{"k": "v"}}

	for rep := 0; rep < repeats; rep++ {
		e := indexed.New()
		_ = e.AddRule(indexed.Rule{
			Name:  "r",
			Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		})

		start := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for i := 0; i < itersPer; i++ {
					_, _ = e.Execute(ctx, req)
				}
			}()
		}
		close(start) // release all goroutines simultaneously
		wg.Wait()
		if !e.Built() {
			t.Fatalf("Built should be true after concurrent Executes")
		}
	}
}

// concurrentSafeListener is a Notifier-only ExecutionListener that
// uses an atomic counter so concurrent OnRuleMatched calls are safe.
// (SnapshotListener mutates its own slices and isn't safe for
// concurrent dispatch from the same instance.)
type concurrentSafeListener struct {
	matches uint64
}

func (l *concurrentSafeListener) OnRuleMatched(observability.Match) {
	atomic.AddUint64(&l.matches, 1)
}

func TestConcurrentListenersSafe(t *testing.T) {
	// Multiple goroutines: some attach listeners, others run Execute.
	// The race detector must not fire on the Notifier's listener
	// slice. Listener instances themselves must be concurrent-safe
	// (this is a documented contract from ADR-0037).
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "r",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	if err := e.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	const (
		executors = 8
		attachers = 4
		iters     = 200
	)
	ctx := context.Background()
	req := engine.Request{Input: map[string]string{"k": "v"}}

	var wg sync.WaitGroup
	for g := 0; g < executors; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_, _ = e.Execute(ctx, req)
			}
		}()
	}
	for g := 0; g < attachers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				e.AddListener(&concurrentSafeListener{})
			}
		}()
	}
	wg.Wait()
}

// ----- Hot-reload pattern -------------------------------------------

func TestHotReloadSwapsRuleSet(t *testing.T) {
	// The documented hot-reload pattern uses atomic.Value or sync.Mutex
	// in caller code; here we exercise it manually.
	build := func(value string) *indexed.Engine {
		e := indexed.New()
		_ = e.AddRule(indexed.Rule{
			Name:   "match-it",
			Match:  parser.StringCondition{Field: "country", Op: parser.OpEq, Value: value},
			Action: func(interface{}) interface{} { return value },
		})
		if err := e.Build(); err != nil {
			t.Fatalf("Build: %v", err)
		}
		return e
	}

	old := build("BR")
	newGen := build("AR")

	// Old generation matches BR, not AR.
	res, _ := old.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 1 {
		t.Fatalf("old engine should match BR")
	}
	res, _ = old.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "AR"}})
	if len(res.Matched) != 0 {
		t.Fatalf("old engine should NOT match AR (it's the BR generation)")
	}

	// New generation matches AR, not BR.
	res, _ = newGen.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "AR"}})
	if len(res.Matched) != 1 || res.Output != "AR" {
		t.Fatalf("new engine should match AR, got %+v", res)
	}
	res, _ = newGen.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 0 {
		t.Fatalf("new engine should NOT match BR")
	}
}
