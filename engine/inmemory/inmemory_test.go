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

func TestEmptyEngineProducesEmptyResult(t *testing.T) {
	e := inmemory.New()

	got, err := e.Execute(engine.Request{Input: "anything"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if got.Output != nil {
		t.Errorf("Output: want nil, got %v", got.Output)
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

func TestExecuteMatchesRulesWhoseConditionIsTrue(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "never",
		Condition: func(interface{}) bool { return false },
	})

	got, err := e.Execute(engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if len(got.Matched) != 1 || got.Matched[0] != "always" {
		t.Fatalf("Matched: want [always], got %v", got.Matched)
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

	got, err := e.Execute(engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(got.Matched) != len(want) {
		t.Fatalf("Matched: want %v, got %v", want, got.Matched)
	}
	for i, n := range want {
		if got.Matched[i] != n {
			t.Errorf("Matched[%d]: want %q, got %q", i, n, got.Matched[i])
		}
	}
}

func TestExecuteUsesInputForConditionDecision(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name: "starts-with-a",
		Condition: func(in interface{}) bool {
			s, ok := in.(string)
			return ok && len(s) > 0 && s[0] == 'a'
		},
	})

	t.Run("matches when input starts with a", func(t *testing.T) {
		got, err := e.Execute(engine.Request{Input: "apple"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 1 {
			t.Fatalf("Matched: want one match, got %v", got.Matched)
		}
	})

	t.Run("does not match when input does not", func(t *testing.T) {
		got, err := e.Execute(engine.Request{Input: "banana"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 0 {
			t.Fatalf("Matched: want empty, got %v", got.Matched)
		}
	})
}

func TestExecuteSkipsRulesWithNilCondition(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{Name: "no-condition"})

	got, err := e.Execute(engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteRunsActionOfMatchingRule(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "doubler",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in.(int) * 2 },
	})

	got, err := e.Execute(engine.Request{Input: 21})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if got.Output != 42 {
		t.Fatalf("Output: want 42, got %v", got.Output)
	}
}

func TestExecuteDoesNotRunActionOfUnmatchedRule(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "guarded",
		Condition: func(interface{}) bool { return false },
		Action: func(interface{}) interface{} {
			t.Fatalf("Action of unmatched rule must not run")
			return nil
		},
	})

	got, err := e.Execute(engine.Request{Input: 1})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if got.Output != nil {
		t.Fatalf("Output: want nil, got %v", got.Output)
	}
}

func TestExecuteLaterMatchingActionWinsOnOutput(t *testing.T) {
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

	got, err := e.Execute(engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}

	if got.Output != "second" {
		t.Fatalf("Output: want \"second\", got %v", got.Output)
	}
	if len(got.Matched) != 2 {
		t.Fatalf("Matched: want both, got %v", got.Matched)
	}
}
