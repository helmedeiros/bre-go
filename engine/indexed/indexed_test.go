package indexed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// helper: build a single-condition equality match.
func eq(field, value string) parser.Condition {
	return parser.StringCondition{Field: field, Op: parser.OpEq, Value: value}
}

// helper: build a conjunction of equality matches.
func and(conds ...parser.Condition) parser.Condition {
	return parser.AndCondition{Children: conds}
}

func TestNewReturnsAnEngine(t *testing.T) {
	var _ engine.Engine = indexed.New()
}

func TestEmptyEngineProducesNoOutput(t *testing.T) {
	e := indexed.New()
	got, err := e.Execute(context.Background(), engine.Request{Input: map[string]string{}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.Output != nil {
		t.Fatalf("Output: want nil, got %v", got.Output)
	}
}

func TestEmptyEngineMatchesNothing(t *testing.T) {
	e := indexed.New()
	got, _ := e.Execute(context.Background(), engine.Request{Input: map[string]string{}})
	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestAddRuleRejectsEmptyName(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{Name: "", Match: eq("country", "BR")})
	if !errors.Is(err, indexed.ErrEmptyRuleName) {
		t.Fatalf("want ErrEmptyRuleName, got %v", err)
	}
}

func TestAddRuleRejectsNilMatch(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{Name: "r1"})
	if !errors.Is(err, indexed.ErrNilMatch) {
		t.Fatalf("want ErrNilMatch, got %v", err)
	}
}

func TestAddRuleRejectsDuplicateName(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "r1", Match: eq("country", "BR")})
	err := e.AddRule(indexed.Rule{Name: "r1", Match: eq("country", "AR")})
	if !errors.Is(err, indexed.ErrDuplicateRuleName) {
		t.Fatalf("want ErrDuplicateRuleName, got %v", err)
	}
}

func TestAddRuleRejectsOpNeq(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "neq-rule",
		Match: parser.StringCondition{Field: "country", Op: parser.OpNeq, Value: "BR"},
	})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestAddRuleRejectsOrCondition(t *testing.T) {
	e := indexed.New()
	or := parser.OrCondition{Children: []parser.Condition{eq("country", "BR"), eq("country", "AR")}}
	err := e.AddRule(indexed.Rule{Name: "or-rule", Match: or})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

// TestAddRuleRejectsOpNotIn -- OpIn became indexable in v0.9.0
// (ADR-0034); OpNotIn stays rejected, slated for v0.10.0's
// post-filter work.
func TestAddRuleRejectsOpNotIn(t *testing.T) {
	e := indexed.New()
	notIn := parser.SetCondition{Field: "country", Op: parser.OpNotIn, Values: []string{"BR", "AR"}}
	err := e.AddRule(indexed.Rule{Name: "not-in-rule", Match: notIn})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestAddRuleRejectsDuplicateFieldInConjunction(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "dup-field",
		Match: and(eq("country", "BR"), eq("country", "AR")),
	})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestExecuteSingleConditionMatch(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	got, err := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 || got.Matched[0] != "br" {
		t.Fatalf("Matched: want [br], got %v", got.Matched)
	}
}

func TestExecuteSingleConditionMiss(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "AR"},
	})
	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteMultiDimMatch(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "premium-br",
		Match: and(eq("country", "BR"), eq("tier", "premium")),
	})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "premium"},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "premium-br" {
		t.Fatalf("Matched: want [premium-br], got %v", got.Matched)
	}
}

func TestExecuteMultiDimMissOnOneField(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "premium-br",
		Match: and(eq("country", "BR"), eq("tier", "premium")),
	})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", got.Matched)
	}
}

func TestExecuteFirstMatchSemantics(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "first", Match: eq("country", "BR")})
	_ = e.AddRule(indexed.Rule{Name: "second", Match: eq("country", "BR")})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "first" {
		t.Fatalf("first-match: want [first], got %v", got.Matched)
	}
}

func TestExecuteHeterogeneousKeysetWalkInInsertionOrder(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "by-country",
		Match: eq("country", "BR"),
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "by-country-and-tier",
		Match: and(eq("country", "BR"), eq("tier", "premium")),
	})

	// Input matches both key-sets. by-country key-set was registered
	// first, so it should win.
	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "premium"},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "by-country" {
		t.Fatalf("key-set order: want [by-country], got %v", got.Matched)
	}
}

func TestExecuteSkipsKeysetWhenFactMissesField(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "needs-tier",
		Match: and(eq("country", "BR"), eq("tier", "premium")),
	})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"}, // missing tier
	})
	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty (missing field), got %v", got.Matched)
	}
}

func TestExecuteRunsActionAndReturnsOutput(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:   "act",
		Match:  eq("country", "BR"),
		Action: func(interface{}) interface{} { return "approved" },
	})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"},
	})
	if got.Output != "approved" {
		t.Fatalf("Output: want approved, got %v", got.Output)
	}
}

func TestExecuteActionPanicProducesActionPanicError(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:   "boom",
		Match:  eq("country", "BR"),
		Action: func(interface{}) interface{} { panic("nope") },
	})

	got, err := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"},
	})
	var ape *indexed.ActionPanicError
	if !errors.As(err, &ape) {
		t.Fatalf("want *ActionPanicError, got %T (%v)", err, err)
	}
	if ape.RuleName() != "boom" {
		t.Fatalf("ActionPanicError.RuleName: want boom, got %s", ape.RuleName())
	}
	if len(got.Matched) != 1 || got.Matched[0] != "boom" {
		t.Fatalf("Matched on panic: want [boom], got %v", got.Matched)
	}
}

func TestExecuteRejectsIncompatibleInput(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	_, err := e.Execute(context.Background(), engine.Request{Input: 42})
	if !errors.Is(err, indexed.ErrIncompatibleInput) {
		t.Fatalf("want ErrIncompatibleInput, got %v", err)
	}
}

func TestExecuteAcceptsMapStringInterface(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	got, err := e.Execute(context.Background(), engine.Request{
		Input: map[string]interface{}{"country": "BR"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want 1, got %v", got.Matched)
	}
}

func TestExecuteStringifiesInterfaceValues(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "amount", Match: eq("amount", "42")})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]interface{}{"amount": 42},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "amount" {
		t.Fatalf("Matched: want [amount], got %v", got.Matched)
	}
}

func TestExecuteIgnoresNilValueInInterfaceMap(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]interface{}{"country": nil},
	})
	if len(got.Matched) != 0 {
		t.Fatalf("Matched: want empty for nil-valued field, got %v", got.Matched)
	}
}

func TestExecuteRespectsCancelledContext(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.Execute(ctx, engine.Request{Input: map[string]string{"country": "BR"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRuleNamesReturnsInsertionOrder(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "a", Match: eq("k", "1")})
	_ = e.AddRule(indexed.Rule{Name: "b", Match: eq("k", "2")})
	_ = e.AddRule(indexed.Rule{Name: "c", Match: eq("k", "3")})

	names := e.RuleNames()
	want := []string{"a", "b", "c"}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("RuleNames[%d]: want %s, got %s", i, n, names[i])
		}
	}
}

func TestRuleInfosCarryDescriptionAndTags(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:        "br",
		Description: "Brazilian decisions",
		Tags:        []string{"region", "geo"},
		Match:       eq("country", "BR"),
	})

	infos := e.RuleInfos()
	if len(infos) != 1 || infos[0].Description != "Brazilian decisions" {
		t.Fatalf("RuleInfos: %v", infos)
	}
	if len(infos[0].Tags) != 2 || infos[0].Tags[0] != "region" {
		t.Fatalf("RuleInfos.Tags: %v", infos[0].Tags)
	}
}

func TestRuleInfosReturnsFreshSliceCopies(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br",
		Tags:  []string{"region"},
		Match: eq("country", "BR"),
	})
	infos := e.RuleInfos()
	infos[0].Tags[0] = "mutated"

	freshInfos := e.RuleInfos()
	if freshInfos[0].Tags[0] != "region" {
		t.Fatalf("RuleInfos: external mutation leaked, got %q", freshInfos[0].Tags[0])
	}
}

func TestActionContextReceivesContextValue(t *testing.T) {
	type ctxKey struct{}
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "ctx-act",
		Match: eq("k", "v"),
		ActionContext: func(ctx context.Context, _ interface{}) interface{} {
			if v := ctx.Value(ctxKey{}); v != nil {
				return v
			}
			return "no-ctx"
		},
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, "from-ctx")
	got, _ := e.Execute(ctx, engine.Request{Input: map[string]string{"k": "v"}})
	if got.Output != "from-ctx" {
		t.Fatalf("ActionContext didn't read ctx value, got %v", got.Output)
	}
}
