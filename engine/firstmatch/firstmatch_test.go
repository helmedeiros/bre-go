package firstmatch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

func TestNewReturnsAnEngine(t *testing.T) {
	var _ engine.Engine = firstmatch.New()
}

func TestEmptyEngineProducesNoOutput(t *testing.T) {
	e := firstmatch.New()

	got, err := e.Execute(context.Background(), engine.Request{Input: "anything"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if got.Output != nil {
		t.Errorf("Output: want nil, got %v", got.Output)
	}
}

func TestEmptyEngineMatchesNothing(t *testing.T) {
	e := firstmatch.New()

	got, _ := e.Execute(context.Background(), engine.Request{Input: "anything"})

	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestAddRuleRejectsEmptyName(t *testing.T) {
	e := firstmatch.New()

	err := e.AddRule(firstmatch.Rule{
		Name:      "",
		Condition: func(interface{}) bool { return true },
	})

	if !errors.Is(err, firstmatch.ErrEmptyRuleName) {
		t.Fatalf("AddRule: want ErrEmptyRuleName, got %v", err)
	}
}

func TestAddRuleRejectsNilCondition(t *testing.T) {
	e := firstmatch.New()

	err := e.AddRule(firstmatch.Rule{Name: "no-condition"})

	if !errors.Is(err, firstmatch.ErrNilCondition) {
		t.Fatalf("AddRule: want ErrNilCondition, got %v", err)
	}
}

func TestAddRuleRejectsDuplicateName(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	err := e.AddRule(firstmatch.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	if !errors.Is(err, firstmatch.ErrDuplicateRuleName) {
		t.Fatalf("AddRule: want ErrDuplicateRuleName, got %v", err)
	}
}

func TestExecuteReturnsFirstMatchingRuleOnly(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{Name: "first", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "second", Condition: func(interface{}) bool { return true }},
	)

	got := execute(t, e)

	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want exactly 1 entry, got %v", got.Matched)
	}
}

func TestExecuteMatchedEntryIsTheFirstMatchingRule(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{Name: "first", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "second", Condition: func(interface{}) bool { return true }},
	)

	got := execute(t, e)

	if got.Matched[0] != "first" {
		t.Fatalf("Matched[0]: want %q, got %q", "first", got.Matched[0])
	}
}

func TestExecuteSkipsNonMatchingRulesUntilTheFirstMatch(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{Name: "skip-me", Condition: func(interface{}) bool { return false }},
		firstmatch.Rule{Name: "pick-me", Condition: func(interface{}) bool { return true }},
	)

	got := execute(t, e)

	if got.Matched[0] != "pick-me" {
		t.Fatalf("Matched[0]: want %q, got %q", "pick-me", got.Matched[0])
	}
}

func TestExecuteDoesNotRunLaterActions(t *testing.T) {
	laterActionRan := false
	e := newEngine(t,
		firstmatch.Rule{
			Name:      "first",
			Condition: func(interface{}) bool { return true },
			Action:    func(interface{}) interface{} { return "first-output" },
		},
		firstmatch.Rule{
			Name:      "second",
			Condition: func(interface{}) bool { return true },
			Action: func(interface{}) interface{} {
				laterActionRan = true
				return "second-output"
			},
		},
	)

	_ = execute(t, e)

	if laterActionRan {
		t.Fatalf("second rule's action ran; first-match must stop after the first hit")
	}
}

func TestExecuteOutputComesFromTheFirstMatchingAction(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{
			Name:      "first",
			Condition: func(interface{}) bool { return true },
			Action:    func(interface{}) interface{} { return "first-output" },
		},
		firstmatch.Rule{
			Name:      "second",
			Condition: func(interface{}) bool { return true },
			Action:    func(interface{}) interface{} { return "second-output" },
		},
	)

	got := execute(t, e)

	if got.Output != "first-output" {
		t.Fatalf("Output: want %q, got %v", "first-output", got.Output)
	}
}

func newEngine(t *testing.T, rules ...firstmatch.Rule) *firstmatch.Engine {
	t.Helper()
	e := firstmatch.New()
	for _, r := range rules {
		if err := e.AddRule(r); err != nil {
			t.Fatalf("AddRule(%q): unexpected error: %v", r.Name, err)
		}
	}
	return e
}

func execute(t *testing.T, e *firstmatch.Engine) engine.Result {
	t.Helper()
	got, err := e.Execute(context.Background(), engine.Request{Input: "x"})
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	return got
}
