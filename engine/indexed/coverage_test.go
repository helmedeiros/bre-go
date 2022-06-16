package indexed_test

import (
	"context"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// Coverage-completion tests: cover the defensive branches and
// pointer-form parser shapes the unit tests don't otherwise reach.

func TestActionPanicErrorErrorString(t *testing.T) {
	e := &indexed.ActionPanicError{Rule: "br", Value: "boom"}
	msg := e.Error()
	if !strings.Contains(msg, "br") || !strings.Contains(msg, "boom") {
		t.Fatalf("Error: missing rule or panic value, got %q", msg)
	}
}

func TestRuleInfosForRuleWithoutTags(t *testing.T) {
	// Covers copyTags(nil) -- triggered only by RuleInfos.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "no-tags", Match: eq("k", "v")})

	infos := e.RuleInfos()
	if len(infos) != 1 || infos[0].Tags != nil {
		t.Fatalf("RuleInfos with nil Tags: want nil, got %v", infos[0].Tags)
	}
}

func TestCollectPairsAcceptsPointerStringCondition(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "ptr-string",
		Match: &parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	if err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR"},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "ptr-string" {
		t.Fatalf("Matched: want [ptr-string], got %v", got.Matched)
	}
}

func TestCollectPairsAcceptsPointerAndCondition(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "ptr-and",
		Match: &parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})
	if err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}

	got, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "premium"},
	})
	if len(got.Matched) != 1 || got.Matched[0] != "ptr-and" {
		t.Fatalf("Matched: want [ptr-and], got %v", got.Matched)
	}
}

func TestCollectPairsRejectsPointerStringConditionWithOpNeq(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "ptr-neq",
		Match: &parser.StringCondition{Field: "x", Op: parser.OpNeq, Value: "y"},
	})
	if err != indexed.ErrNonIndexableCondition {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestCollectPairsRejectsPointerAndWithNonIndexableChild(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "ptr-and-bad",
		Match: &parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "x", Op: parser.OpEq, Value: "1"},
			parser.NotCondition{Child: parser.StringCondition{Field: "y", Op: parser.OpEq, Value: "2"}},
		}},
	})
	if err != indexed.ErrNonIndexableCondition {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestCollectPairsRejectsAndWithNonIndexableChild(t *testing.T) {
	// Value-form AndCondition with a non-indexable child -- exercises
	// the recursion-error path for the (non-pointer) AndCondition arm.
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name: "val-and-bad",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "x", Op: parser.OpEq, Value: "1"},
			parser.NotCondition{Child: parser.StringCondition{Field: "y", Op: parser.OpEq, Value: "2"}},
		}},
	})
	if err != indexed.ErrNonIndexableCondition {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestExtractEqualityPairsRejectsEmptyAnd(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "empty-and",
		Match: parser.AndCondition{}, // zero children -> no pairs extracted
	})
	if err != indexed.ErrNonIndexableCondition {
		t.Fatalf("want ErrNonIndexableCondition for empty And, got %v", err)
	}
}

func TestExecuteAcceptsNilContext(t *testing.T) {
	// Per the engine.Engine port contract: nil ctx is treated as
	// context.Background().
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "br", Match: eq("country", "BR")})

	//nolint:staticcheck // SA1012: deliberately testing the nil-ctx contract
	got, err := e.Execute(nil, engine.Request{Input: map[string]string{"country": "BR"}})
	if err != nil {
		t.Fatalf("Execute(nil ctx): %v", err)
	}
	if len(got.Matched) != 1 {
		t.Fatalf("Matched: want 1, got %v", got.Matched)
	}
}

func TestExecuteCancelledMidKeysetWalk(t *testing.T) {
	// Two key-sets registered; cancel ctx after construction so the
	// keyset-loop's ctx.Err check fires on entry.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "a", Match: eq("k1", "v")})
	_ = e.AddRule(indexed.Rule{Name: "b", Match: and(eq("k1", "v"), eq("k2", "v"))})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.Execute(ctx, engine.Request{
		Input: map[string]string{"k1": "v", "k2": "v"},
	})
	if err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestExecuteCancelledBetweenKeysets(t *testing.T) {
	// Register two key-sets. Use a context whose Done fires after one
	// hash lookup so the second iteration of the keyset-walk catches
	// the cancellation. We simulate this by pre-canceling and
	// counting on the loop-top check.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{Name: "k1-only", Match: eq("k1", "x")}) // won't match v
	_ = e.AddRule(indexed.Rule{Name: "both", Match: and(eq("k1", "v"), eq("k2", "v"))})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.Execute(ctx, engine.Request{
		Input: map[string]string{"k1": "v", "k2": "v"},
	})
	if err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
