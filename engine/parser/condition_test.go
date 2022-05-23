package parser_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

func TestStringConditionEqMatches(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	if !c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("StringCondition Eq: want true for equal value")
	}
}

func TestStringConditionEqRejectsDifferent(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	if c.Eval(map[string]interface{}{"tier": "economy"}) {
		t.Fatalf("StringCondition Eq: want false for different value")
	}
}

func TestStringConditionNeqMatches(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "economy"}
	if !c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("StringCondition Neq: want true for different value")
	}
}

func TestStringConditionMissingFieldIsFalse(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	if c.Eval(map[string]interface{}{}) {
		t.Fatalf("StringCondition: want false on missing field")
	}
}

func TestStringConditionNonStringValueIsFalse(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	if c.Eval(map[string]interface{}{"tier": 42}) {
		t.Fatalf("StringCondition: want false on non-string fact value")
	}
}

func TestStringConditionUnknownOpIsFalse(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: "??", Value: "vip"}
	if c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("StringCondition: want false for unknown op")
	}
}

func TestSetConditionInMatches(t *testing.T) {
	c := parser.SetCondition{Field: "tier", Op: parser.OpIn, Values: []string{"vip", "premium"}}
	if !c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("SetCondition In: want true for listed value")
	}
}

func TestSetConditionNotInRejectsListed(t *testing.T) {
	c := parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"vip"}}
	if c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("SetCondition NotIn: want false for listed value")
	}
}

func TestSetConditionMissingFieldIsFalse(t *testing.T) {
	c := parser.SetCondition{Field: "tier", Op: parser.OpIn, Values: []string{"vip"}}
	if c.Eval(map[string]interface{}{}) {
		t.Fatalf("SetCondition: want false on missing field")
	}
}

func TestSetConditionNonStringIsFalse(t *testing.T) {
	c := parser.SetCondition{Field: "tier", Op: parser.OpIn, Values: []string{"vip"}}
	if c.Eval(map[string]interface{}{"tier": 42}) {
		t.Fatalf("SetCondition: want false on non-string fact value")
	}
}

func TestSetConditionUnknownOpIsFalse(t *testing.T) {
	c := parser.SetCondition{Field: "tier", Op: "??", Values: []string{"vip"}}
	if c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("SetCondition: want false for unknown op")
	}
}

func TestAndConditionEmptyIsTrue(t *testing.T) {
	c := parser.AndCondition{}
	if !c.Eval(nil) {
		t.Fatalf("AndCondition empty: want true (identity for conjunction)")
	}
}

func TestOrConditionEmptyIsFalse(t *testing.T) {
	c := parser.OrCondition{}
	if c.Eval(nil) {
		t.Fatalf("OrCondition empty: want false (identity for disjunction)")
	}
}

func TestAndConditionShortCircuits(t *testing.T) {
	right := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	c := parser.AndCondition{Children: []parser.Condition{
		parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "economy"},
		right,
	}}
	if c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("AndCondition: want false when first child is false")
	}
}

func TestOrConditionShortCircuits(t *testing.T) {
	c := parser.OrCondition{Children: []parser.Condition{
		parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"},
		parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "economy"},
	}}
	if !c.Eval(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("OrCondition: want true when first child is true")
	}
}

func TestNotConditionInverts(t *testing.T) {
	c := parser.NotCondition{Child: parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}}
	if !c.Eval(map[string]interface{}{"tier": "economy"}) {
		t.Fatalf("NotCondition: want true when inner is false")
	}
}

func TestParseToConditionReturnsTypedTree(t *testing.T) {
	c, err := parser.ParseToCondition(`tier == "vip"`)
	if err != nil {
		t.Fatalf("ParseToCondition: %v", err)
	}
	sc, ok := c.(parser.StringCondition)
	if !ok {
		t.Fatalf("ParseToCondition: want StringCondition, got %T", c)
	}
	if sc.Field != "tier" || sc.Op != parser.OpEq || sc.Value != "vip" {
		t.Fatalf("StringCondition fields: got %+v", sc)
	}
}

func TestParseToConditionFlattensAndChain(t *testing.T) {
	c, _ := parser.ParseToCondition(`a == "x" AND b == "y" AND c == "z"`)
	and, ok := c.(parser.AndCondition)
	if !ok {
		t.Fatalf("want AndCondition, got %T", c)
	}
	if len(and.Children) != 3 {
		t.Fatalf("Children: want 3 flattened, got %d", len(and.Children))
	}
}

func TestParseToConditionFlattensOrChain(t *testing.T) {
	c, _ := parser.ParseToCondition(`a == "x" OR b == "y" OR c == "z"`)
	or, ok := c.(parser.OrCondition)
	if !ok {
		t.Fatalf("want OrCondition, got %T", c)
	}
	if len(or.Children) != 3 {
		t.Fatalf("Children: want 3 flattened, got %d", len(or.Children))
	}
}

func TestParseToConditionProducesSetCondition(t *testing.T) {
	c, _ := parser.ParseToCondition(`tier IN ("vip", "premium")`)
	set, ok := c.(parser.SetCondition)
	if !ok {
		t.Fatalf("want SetCondition, got %T", c)
	}
	if set.Op != parser.OpIn || len(set.Values) != 2 {
		t.Fatalf("SetCondition fields: got %+v", set)
	}
}

func TestParseToConditionProducesNotCondition(t *testing.T) {
	c, _ := parser.ParseToCondition(`NOT tier == "vip"`)
	if _, ok := c.(parser.NotCondition); !ok {
		t.Fatalf("want NotCondition, got %T", c)
	}
}

func TestParseToConditionPropagatesError(t *testing.T) {
	_, err := parser.ParseToCondition(`@ broken`)
	if err == nil {
		t.Fatalf("expected error on broken expression")
	}
}

func TestAsPredicateBridge(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	pred := parser.AsPredicate(c)
	if !pred(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("AsPredicate: want true for matching fact")
	}
}

func TestAsRuleConditionBridge(t *testing.T) {
	c := parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "vip"}
	factOf := func(in interface{}) map[string]interface{} {
		return map[string]interface{}{"tier": in.(string)}
	}
	rc := parser.AsRuleCondition(c, factOf)
	if !rc("vip") {
		t.Fatalf("AsRuleCondition: want true for matching input")
	}
	if rc("economy") {
		t.Fatalf("AsRuleCondition: want false for non-matching input")
	}
}
