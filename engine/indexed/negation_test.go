package indexed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ADR-0035 tests: OpNeq / OpNotIn admitted as post-filter when
// paired with at least one indexable term. Pure-negation rules
// rejected with ErrNoIndexableTerms.

// ----- Mixed OpEq + OpNeq happy path ----------------------------------

func TestAddRuleAcceptsOpNeqAsPostFilter(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "premium-not-vip",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})
	if err != nil {
		t.Fatalf("AddRule (OpEq+OpNeq): unexpected error: %v", err)
	}
}

func TestExecuteAppliesOpNeqPostFilter(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-not-vip",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if len(res.Matched) != 1 || res.Matched[0] != "br-not-vip" {
		t.Fatalf("BR + tier=standard should match (tier != vip), got %v", res.Matched)
	}

	res, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "vip"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("BR + tier=vip should NOT match (post-filter rejects), got %v", res.Matched)
	}
}

func TestExecuteAppliesOpNotInPostFilter(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-not-vip-or-trial",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"vip", "trial"}},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("standard tier should pass NOT IN [vip, trial], got %v", res.Matched)
	}
	for _, blocked := range []string{"vip", "trial"} {
		res, _ := e.Execute(context.Background(), engine.Request{
			Input: map[string]string{"country": "BR", "tier": blocked},
		})
		if len(res.Matched) != 0 {
			t.Fatalf("tier=%s should be rejected by NOT IN, got %v", blocked, res.Matched)
		}
	}
}

func TestExecuteMultiplePostFiltersAllMustPass(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-not-vip-not-trial",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
			parser.StringCondition{Field: "plan", Op: parser.OpNeq, Value: "trial"},
		}},
	})

	// All three pass.
	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard", "plan": "paid"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("all three pass: want match, got %v", res.Matched)
	}
	// One post-filter fails.
	res, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard", "plan": "trial"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("plan=trial should be rejected, got %v", res.Matched)
	}
}

// ----- Pure-negation rejection ----------------------------------------

func TestAddRuleRejectsPureOpNeq(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "pure-neq",
		Match: parser.StringCondition{Field: "country", Op: parser.OpNeq, Value: "BR"},
	})
	if !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("want ErrNoIndexableTerms, got %v", err)
	}
}

func TestAddRuleRejectsPureNotInWithinAnd(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "and-of-neqs",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpNeq, Value: "BR"},
			parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"vip"}},
		}},
	})
	if !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("AND of only negations should return ErrNoIndexableTerms, got %v", err)
	}
}

// ----- Insertion-order tie-break across post-filter shape -------------

func TestPostFilterDoesNotAffectInsertionOrderTieBreak(t *testing.T) {
	e := indexed.New()
	// First rule: pure indexable on country=BR.
	_ = e.AddRule(indexed.Rule{
		Name:  "wide",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	// Second rule: same indexable + post-filter.
	_ = e.AddRule(indexed.Rule{
		Name: "narrow-with-postfilter",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})

	// Both could match -- but "wide" registered first, single-key-set
	// {country} walked before the two-key-set bucket.
	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if len(res.Matched) != 1 || res.Matched[0] != "wide" {
		t.Fatalf("first-registered key-set should win: want [wide], got %v", res.Matched)
	}
}

// ----- Action runs only after post-filter passes ----------------------

func TestActionFiresOnlyWhenPostFilterPasses(t *testing.T) {
	calls := 0
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "act",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
		Action: func(interface{}) interface{} {
			calls++
			return "ok"
		},
	})

	// Post-filter passes -- action runs.
	_, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if calls != 1 {
		t.Fatalf("Action call count after pass: want 1, got %d", calls)
	}

	// Post-filter fails -- action must NOT run.
	_, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "vip"},
	})
	if calls != 1 {
		t.Fatalf("Action must not run when post-filter rejects: call count went to %d", calls)
	}
}

// ----- Pointer-form negation as post-filter ---------------------------

func TestPostFilterAcceptsPointerOpNeq(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "ptr-mixed",
		Match: &parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})
	if err != nil {
		t.Fatalf("pointer-form mixed rule: unexpected error: %v", err)
	}

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "standard"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("pointer-form post-filter: want match, got %v", res.Matched)
	}
}

func TestPostFilterAcceptsPointerOpNotIn(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "ptr-not-in",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&parser.SetCondition{Field: "tier", Op: parser.OpNotIn, Values: []string{"vip", "trial"}},
		}},
	})
	if err != nil {
		t.Fatalf("pointer-form OpNotIn post-filter: unexpected error: %v", err)
	}
}
