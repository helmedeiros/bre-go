package inmemory_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func TestNewReturnsAnEngine(t *testing.T) {
	var _ engine.Engine = inmemory.New()
}

func TestEmptyEngineProducesNoOutput(t *testing.T) {
	e := inmemory.New()

	got, err := e.Execute(engine.Request{Input: "anything"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if got.Output != nil {
		t.Errorf("Output: want nil, got %v", got.Output)
	}
}

func TestEmptyEngineMatchesNothing(t *testing.T) {
	e := inmemory.New()

	got, err := e.Execute(engine.Request{Input: "anything"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if len(got.Matched) != 0 {
		t.Errorf("Matched: want empty, got %v", got.Matched)
	}
}

func TestAddRuleAcceptsNamedRule(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{
		Name:      "always-true",
		Condition: func(interface{}) bool { return true },
	})

	if err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
}

func TestAddRuleRejectsEmptyName(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{Name: ""})

	if !errors.Is(err, inmemory.ErrEmptyRuleName) {
		t.Fatalf("AddRule: want ErrEmptyRuleName, got %v", err)
	}
}

func TestAddRuleRejectsDuplicateName(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{Name: "alpha"})

	err := e.AddRule(inmemory.Rule{Name: "alpha"})

	if !errors.Is(err, inmemory.ErrDuplicateRuleName) {
		t.Fatalf("AddRule: want ErrDuplicateRuleName, got %v", err)
	}
}

func TestAddRuleDoesNotStoreDuplicate(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e, "x")

	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want 1 entry, got %d", len(got.Matched))
	}
}

func TestExecuteMatchesARuleWhoseConditionIsTrue(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e, "x")

	if len(got.Matched) == 0 {
		t.Fatalf("Matched: want at least one entry, got %v", got.Matched)
	}
}

func TestExecuteSkipsARuleWhoseConditionIsFalse(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "never",
		Condition: func(interface{}) bool { return false },
	})

	got := execute(t, e, "x")

	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteUsesTheRuleName(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "the-one",
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e, "x")

	if got.Matched[0] != "the-one" {
		t.Fatalf("Matched[0]: want %q, got %q", "the-one", got.Matched[0])
	}
}

func TestExecutePreservesInsertionOrder(t *testing.T) {
	e := inmemory.New()
	for _, name := range []string{"first", "second", "third"} {
		_ = e.AddRule(inmemory.Rule{
			Name:      name,
			Condition: func(interface{}) bool { return true },
		})
	}

	got := execute(t, e, "x")

	want := []string{"first", "second", "third"}
	for i, n := range want {
		if got.Matched[i] != n {
			t.Errorf("Matched[%d]: want %q, got %q", i, n, got.Matched[i])
		}
	}
}

func TestExecuteMatchesWhenInputSatisfiesCondition(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "starts-with-a",
		Condition: startsWithA,
	})

	got := execute(t, e, "apple")

	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want one entry, got %v", got.Matched)
	}
}

func TestExecuteSkipsWhenInputDoesNotSatisfyCondition(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "starts-with-a",
		Condition: startsWithA,
	})

	got := execute(t, e, "banana")

	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteSkipsRulesWithNilCondition(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{Name: "no-condition"})

	got := execute(t, e, "x")

	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteRunsActionOfMatchingRule(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "doubler",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in.(int) * 2 },
	})

	got := execute(t, e, 21)

	if got.Output != 42 {
		t.Fatalf("Output: want 42, got %v", got.Output)
	}
}

func TestExecuteDoesNotRunActionOfUnmatchedRule(t *testing.T) {
	e := newEngineWithRule(t, inmemory.Rule{
		Name:      "guarded",
		Condition: func(interface{}) bool { return false },
		Action: func(interface{}) interface{} {
			t.Fatalf("Action of unmatched rule must not run")
			return nil
		},
	})

	_ = execute(t, e, 1)
}

func TestExecuteLastMatchingActionWinsOnOutput(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "first",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { return "first" },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "second",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { return "second" },
	})

	got := execute(t, e, "x")

	if got.Output != "second" {
		t.Fatalf("Output: want %q, got %v", "second", got.Output)
	}
}

func TestExecuteRecordsEveryMatchingRule(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "first",
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "second",
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e, "x")

	if len(got.Matched) != 2 {
		t.Fatalf("Matched: want two entries, got %v", got.Matched)
	}
}

func startsWithA(in interface{}) bool {
	s, ok := in.(string)
	return ok && len(s) > 0 && s[0] == 'a'
}

func newEngineWithRule(t *testing.T, r inmemory.Rule) *inmemory.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(r); err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
	return e
}

func execute(t *testing.T, e *inmemory.Engine, input interface{}) engine.Result {
	t.Helper()
	got, err := e.Execute(engine.Request{Input: input})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	return got
}
