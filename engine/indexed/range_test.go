package indexed_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ADR-0036 tests: RangeCondition admitted as post-filter, plus
// WithPostFilterHook lets callers add custom shapes.

// ----- RangeCondition happy path --------------------------------------

func TestAddRuleAcceptsRangeConditionMixedWithEquality(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "br-100-to-500",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: 500},
		}},
	})
	if err != nil {
		t.Fatalf("AddRule (OpEq + Range): unexpected error: %v", err)
	}
}

func TestExecuteAppliesRangeConditionPostFilter(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-100-to-500",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: 500},
		}},
	})

	// In range -> match.
	for _, v := range []string{"100", "250", "500"} {
		res, _ := e.Execute(context.Background(), engine.Request{
			Input: map[string]string{"country": "BR", "amount": v},
		})
		if len(res.Matched) != 1 {
			t.Fatalf("amount=%s should match (in [100, 500]): got %v", v, res.Matched)
		}
	}

	// Out of range -> no match.
	for _, v := range []string{"99", "501", "0"} {
		res, _ := e.Execute(context.Background(), engine.Request{
			Input: map[string]string{"country": "BR", "amount": v},
		})
		if len(res.Matched) != 0 {
			t.Fatalf("amount=%s should NOT match (outside [100, 500]): got %v", v, res.Matched)
		}
	}
}

func TestExecuteRangeConditionUnboundedAbove(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-min-100",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "amount": "1000000"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("unbounded-above: want match, got %v", res.Matched)
	}
}

func TestExecuteRangeConditionRejectsNonNumericInput(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-with-range",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.RangeCondition{Field: "amount", Min: 0, Max: 1000},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "amount": "not-a-number"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("non-numeric amount: want no match, got %v", res.Matched)
	}
}

// ----- Pure-range rejection -------------------------------------------

func TestAddRuleRejectsPureRangeAsNoIndexableTerms(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "pure-range",
		Match: parser.RangeCondition{Field: "amount", Min: 0, Max: 100},
	})
	if !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("want ErrNoIndexableTerms, got %v", err)
	}
}

func TestAddRulePurePointerRangeAlsoRejected(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "ptr-pure-range",
		Match: &parser.RangeCondition{Field: "amount", Min: 0, Max: 100},
	})
	if !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("want ErrNoIndexableTerms, got %v", err)
	}
}

// ----- Pointer-form RangeCondition ------------------------------------

func TestPointerRangeConditionAcceptedAsPostFilter(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "ptr-range-mixed",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&parser.RangeCondition{Field: "amount", Min: 100, Max: 500},
		}},
	})
	if err != nil {
		t.Fatalf("AddRule (pointer RangeCondition): unexpected error: %v", err)
	}

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "amount": "250"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("pointer-form range: want match, got %v", res.Matched)
	}
}

// ----- PostFilterHook for caller-defined shapes -----------------------

// customCondition is a caller-defined parser.Condition that the
// built-in classifier doesn't recognize. The hook claims it; the
// adapter routes its Eval through the post-filter path.
type customCondition struct {
	Field      string
	Allowed    string
	evalCalled int // probe for testing
}

func (c *customCondition) Eval(fact map[string]interface{}) bool {
	c.evalCalled++
	got, ok := fact[c.Field]
	if !ok {
		return false
	}
	return got == c.Allowed
}

func TestWithPostFilterHookAdmitsCustomCondition(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool {
		_, ok := c.(*customCondition)
		return ok
	})

	custom := &customCondition{Field: "trace_id", Allowed: "request-42"}
	err := e.AddRule(indexed.Rule{
		Name: "br-with-custom",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			custom,
		}},
	})
	if err != nil {
		t.Fatalf("AddRule with hook-classified condition: %v", err)
	}

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "trace_id": "request-42"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("custom condition (Eval=true): want match, got %v", res.Matched)
	}
	if custom.evalCalled == 0 {
		t.Fatalf("custom condition's Eval was never called")
	}

	// Negative case: same rule, different trace_id.
	res, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "trace_id": "different"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("custom condition (Eval=false): want no match, got %v", res.Matched)
	}
}

func TestWithoutPostFilterHookCustomConditionRejected(t *testing.T) {
	// Same rule shape, no hook -> ErrNonIndexableCondition.
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "needs-hook",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&customCondition{Field: "x", Allowed: "y"},
		}},
	})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("without hook: want ErrNonIndexableCondition, got %v", err)
	}
}

func TestPostFilterHookDeclinesFallsBackToError(t *testing.T) {
	// Hook returns false for everything; should not admit anything.
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool {
		return false
	})
	err := e.AddRule(indexed.Rule{
		Name: "decliner",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&customCondition{Field: "x", Allowed: "y"},
		}},
	})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("declining hook: want ErrNonIndexableCondition, got %v", err)
	}
}

func TestPostFilterHookDoesNotOverrideBuiltinClassification(t *testing.T) {
	// The hook claims StringCondition too, but built-in classification
	// runs first and treats OpEq as a bucket-key contributor. Adding
	// a rule that's bucketed-only-via-OpEq should NOT go through the
	// hook.
	hookCalls := 0
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool {
		hookCalls++
		return true
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "plain-eq",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	if hookCalls != 0 {
		t.Fatalf("hook must not be called for built-in shapes: got %d calls", hookCalls)
	}
}

func TestWithPostFilterHookReturnsEngineForChaining(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool { return false })
	if e == nil {
		t.Fatalf("WithPostFilterHook returned nil; should return the engine")
	}
}

func TestWithPostFilterHookReplacesPriorHook(t *testing.T) {
	firstCalls := 0
	secondCalls := 0
	e := indexed.New().
		WithPostFilterHook(func(c parser.Condition) bool { firstCalls++; return false }).
		WithPostFilterHook(func(c parser.Condition) bool { secondCalls++; return true })

	_ = e.AddRule(indexed.Rule{
		Name: "uses-second-hook",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&customCondition{Field: "x", Allowed: "y"},
		}},
	})
	if firstCalls != 0 {
		t.Fatalf("first hook should have been replaced; got %d calls", firstCalls)
	}
	if secondCalls != 1 {
		t.Fatalf("second hook should have been called once; got %d", secondCalls)
	}
}
