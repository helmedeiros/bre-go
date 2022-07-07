package parser_test

import (
	"math"
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

func TestRangeConditionInclusiveMatch(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 100, Max: 500}
	for _, v := range []string{"100", "250", "500"} {
		if !c.Eval(map[string]interface{}{"amount": v}) {
			t.Fatalf("Eval(%s): want true (inclusive), got false", v)
		}
	}
}

func TestRangeConditionRejectsOutsideRange(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 100, Max: 500}
	for _, v := range []string{"99", "501", "0", "10000"} {
		if c.Eval(map[string]interface{}{"amount": v}) {
			t.Fatalf("Eval(%s): want false, got true", v)
		}
	}
}

func TestRangeConditionAcceptsFractional(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 1.5, Max: 2.5}
	if !c.Eval(map[string]interface{}{"amount": "2.0"}) {
		t.Fatalf("Eval(2.0) in [1.5, 2.5]: want true")
	}
	if c.Eval(map[string]interface{}{"amount": "3.0"}) {
		t.Fatalf("Eval(3.0) in [1.5, 2.5]: want false")
	}
}

func TestRangeConditionUnboundedAbove(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 100, Max: math.Inf(+1)}
	if !c.Eval(map[string]interface{}{"amount": "1000000"}) {
		t.Fatalf("Eval(1M) in [100, +Inf): want true")
	}
	if c.Eval(map[string]interface{}{"amount": "99"}) {
		t.Fatalf("Eval(99) in [100, +Inf): want false")
	}
}

func TestRangeConditionUnboundedBelow(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: math.Inf(-1), Max: 100}
	if !c.Eval(map[string]interface{}{"amount": "-1000"}) {
		t.Fatalf("Eval(-1000) in (-Inf, 100]: want true")
	}
	if c.Eval(map[string]interface{}{"amount": "101"}) {
		t.Fatalf("Eval(101) in (-Inf, 100]: want false")
	}
}

func TestRangeConditionMissingFieldReturnsFalse(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 0, Max: 100}
	if c.Eval(map[string]interface{}{"other": "50"}) {
		t.Fatalf("missing field: want false")
	}
}

func TestRangeConditionNonStringValueReturnsFalse(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 0, Max: 100}
	if c.Eval(map[string]interface{}{"amount": 42}) {
		t.Fatalf("non-string value (int): want false (Eval requires string-form)")
	}
}

func TestRangeConditionUnparseableStringReturnsFalse(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 0, Max: 100}
	for _, v := range []string{"abc", "", "1.2.3", "10 USD"} {
		if c.Eval(map[string]interface{}{"amount": v}) {
			t.Fatalf("Eval(%q): want false for unparseable", v)
		}
	}
}

func TestRangeConditionDegenerateRangeNeverMatches(t *testing.T) {
	c := parser.RangeCondition{Field: "amount", Min: 500, Max: 100} // inverted
	for _, v := range []string{"100", "300", "500"} {
		if c.Eval(map[string]interface{}{"amount": v}) {
			t.Fatalf("Eval(%s) in degenerate [500, 100]: want false", v)
		}
	}
}
