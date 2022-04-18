package priority_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func TestNewReturnsAnEngine(t *testing.T) {
	var _ engine.Engine = priority.New()
}

func TestAddRuleRejectsEmptyName(t *testing.T) {
	e := priority.New()

	err := e.AddRule(priority.Rule{
		Name:      "",
		Condition: func(interface{}) bool { return true },
	})

	if !errors.Is(err, priority.ErrEmptyRuleName) {
		t.Fatalf("AddRule: want ErrEmptyRuleName, got %v", err)
	}
}

func TestAddRuleRejectsNilCondition(t *testing.T) {
	e := priority.New()

	err := e.AddRule(priority.Rule{Name: "no-condition"})

	if !errors.Is(err, priority.ErrNilCondition) {
		t.Fatalf("AddRule: want ErrNilCondition, got %v", err)
	}
}

func TestAddRuleRejectsDuplicateName(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	err := e.AddRule(priority.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	if !errors.Is(err, priority.ErrDuplicateRuleName) {
		t.Fatalf("AddRule: want ErrDuplicateRuleName, got %v", err)
	}
}

func TestExecuteEvaluatesHighestPriorityFirst(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "low",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "high",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e)

	if got.Matched[0] != "high" {
		t.Fatalf("Matched[0]: want %q, got %q", "high", got.Matched[0])
	}
}

func TestExecuteIsAFirstMatchPolicy(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "high",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "low",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e)

	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want exactly 1 entry, got %v", got.Matched)
	}
}

func TestExecuteBreaksTiesByInsertionOrder(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "first",
		Priority:  5,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "second",
		Priority:  5,
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e)

	if got.Matched[0] != "first" {
		t.Fatalf("Matched[0] for tie: want %q (inserted first), got %q", "first", got.Matched[0])
	}
}

func TestExecuteSkipsNonMatchingHigherPriorityRules(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "skip-me",
		Priority:  100,
		Condition: func(interface{}) bool { return false },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "pick-me",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})

	got := execute(t, e)

	if got.Matched[0] != "pick-me" {
		t.Fatalf("Matched[0]: want %q, got %q", "pick-me", got.Matched[0])
	}
}

func TestExecuteRunsTheMatchingRulesAction(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "match",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { return "ran" },
	})

	got := execute(t, e)

	if got.Output != "ran" {
		t.Fatalf("Output: want %q, got %v", "ran", got.Output)
	}
}

func TestRuleNamesReturnsInsertionOrder(t *testing.T) {
	e := priority.New()
	for _, r := range []priority.Rule{
		{Name: "low", Priority: 1, Condition: func(interface{}) bool { return true }},
		{Name: "high", Priority: 10, Condition: func(interface{}) bool { return true }},
	} {
		_ = e.AddRule(r)
	}

	names := e.RuleNames()

	if names[0] != "low" || names[1] != "high" {
		t.Fatalf("RuleNames: want insertion order [low high], got %v", names)
	}
}

func execute(t *testing.T, e *priority.Engine) engine.Result {
	t.Helper()
	got, err := e.Execute(context.Background(), engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	return got
}
