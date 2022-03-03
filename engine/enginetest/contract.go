// Package enginetest is the shared engine.Engine contract suite.
// Adapters call RunContractTests from their own *_test.go.
package enginetest

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

// Factory builds a fresh empty engine and a SeedFunc.
type Factory func(t *testing.T) (engine.Engine, SeedFunc)

// SeedFunc registers a named rule. Returning an error skips the
// affected case (adapter does not support that rule shape).
type SeedFunc func(name string, match func(input interface{}) bool, action func(input interface{}) interface{}) error

// RunContractTests runs the suite against factory.
func RunContractTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("empty engine produces empty result", func(t *testing.T) {
		eng, _ := factory(t)
		got, err := eng.Execute(engine.Request{Input: "anything"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if got.Output != nil {
			t.Errorf("Output: want nil, got %v", got.Output)
		}
		if len(got.Matched) != 0 {
			t.Errorf("Matched: want empty, got %v", got.Matched)
		}
	})

	t.Run("matching rule appears in Matched", func(t *testing.T) {
		eng, seed := factory(t)
		if err := seed("always", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		got, err := eng.Execute(engine.Request{Input: "x"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 1 || got.Matched[0] != "always" {
			t.Fatalf("Matched: want [always], got %v", got.Matched)
		}
	})

	t.Run("non-matching rule does not appear in Matched", func(t *testing.T) {
		eng, seed := factory(t)
		if err := seed("never", func(interface{}) bool { return false }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		got, err := eng.Execute(engine.Request{Input: "x"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 0 {
			t.Fatalf("Matched: want empty, got %v", got.Matched)
		}
	})

	t.Run("matching rule with action produces output", func(t *testing.T) {
		eng, seed := factory(t)
		err := seed("identity",
			func(interface{}) bool { return true },
			func(in interface{}) interface{} { return in },
		)
		if err != nil {
			t.Skipf("adapter does not support action rules: %v", err)
		}
		got, err := eng.Execute(engine.Request{Input: 42})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if got.Output != 42 {
			t.Fatalf("Output: want 42, got %v", got.Output)
		}
	})

	t.Run("seed rejects a duplicate rule name", func(t *testing.T) {
		_, seed := factory(t)
		if err := seed("dup", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		if err := seed("dup", func(interface{}) bool { return true }, nil); err == nil {
			t.Fatalf("second seed of the same name: want error, got nil")
		}
	})

	t.Run("condition can read the input", func(t *testing.T) {
		eng, seed := factory(t)
		err := seed("starts-with-a",
			func(in interface{}) bool {
				s, ok := in.(string)
				return ok && len(s) > 0 && s[0] == 'a'
			},
			nil,
		)
		if err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		gotApple, errApple := eng.Execute(engine.Request{Input: "apple"})
		if errApple != nil {
			t.Fatalf("Execute(apple): unexpected error: %v", errApple)
		}
		if len(gotApple.Matched) != 1 {
			t.Fatalf("apple: want one match, got %v", gotApple.Matched)
		}
		gotBanana, errBanana := eng.Execute(engine.Request{Input: "banana"})
		if errBanana != nil {
			t.Fatalf("Execute(banana): unexpected error: %v", errBanana)
		}
		if len(gotBanana.Matched) != 0 {
			t.Fatalf("banana: want no matches, got %v", gotBanana.Matched)
		}
	})

	t.Run("if ListenerHost is satisfied, OnRuleMatched fires for matches", func(t *testing.T) {
		eng, seed := factory(t)
		host, ok := eng.(engine.ListenerHost)
		if !ok {
			t.Skip("adapter does not satisfy engine.ListenerHost")
		}
		if err := seed("always", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		counter := &observability.CountingListener{}
		host.AddListener(counter)
		if _, err := eng.Execute(engine.Request{Input: "x"}); err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if counter.Total() == 0 {
			t.Fatalf("counter.Total: want > 0 after a match, got 0")
		}
	})

	t.Run("if ListenerHost is satisfied, lifecycle listeners receive one start and one finish per Execute", func(t *testing.T) {
		eng, seed := factory(t)
		host, ok := eng.(engine.ListenerHost)
		if !ok {
			t.Skip("adapter does not satisfy engine.ListenerHost")
		}
		if err := seed("always", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		rec := &lifecycleRecorder{}
		host.AddListener(rec)
		if _, err := eng.Execute(engine.Request{Input: "x"}); err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if rec.startedCalls != 1 {
			t.Fatalf("OnExecutionStarted: want 1 call, got %d", rec.startedCalls)
		}
		if rec.finishedCalls != 1 {
			t.Fatalf("OnExecutionFinished: want 1 call, got %d", rec.finishedCalls)
		}
	})

	t.Run("if RuleLister is satisfied, RuleNames lists the seeded rules", func(t *testing.T) {
		eng, seed := factory(t)
		lister, ok := eng.(engine.RuleLister)
		if !ok {
			t.Skip("adapter does not satisfy engine.RuleLister")
		}
		if err := seed("alpha", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		names := lister.RuleNames()
		if len(names) != 1 || names[0] != "alpha" {
			t.Fatalf("RuleNames: want [alpha], got %v", names)
		}
	})
}
