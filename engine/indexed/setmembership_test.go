package indexed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ADR-0034 tests: OpIn set-membership and wildcard semantics in
// engine/indexed. Run with `go test ./engine/indexed/...`.

// ----- OpIn happy path -------------------------------------------------

func TestAddRuleAcceptsOpInSet(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "br-or-ar",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})
	if err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
}

func TestExecuteMatchesAnyOpInValue(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br-or-ar",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})

	for _, c := range []string{"BR", "AR"} {
		res, err := e.Execute(context.Background(), engine.Request{
			Input: map[string]string{"country": c},
		})
		if err != nil {
			t.Fatalf("Execute(%s): %v", c, err)
		}
		if len(res.Matched) != 1 || res.Matched[0] != "br-or-ar" {
			t.Fatalf("Execute(%s): want [br-or-ar], got %v", c, res.Matched)
		}
	}
}

func TestExecuteRejectsValueNotInOpIn(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br-or-ar",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "CL"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("Matched: want empty, got %v", res.Matched)
	}
}

func TestOpInSingleValueBehavesLikeOpEq(t *testing.T) {
	// A SetCondition with one value should match identically to a
	// StringCondition{OpEq} with the same value.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br-only",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR"}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 1 {
		t.Fatalf("OpIn[BR] should match country=BR, got %v", res.Matched)
	}
	res, _ = e.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "AR"}})
	if len(res.Matched) != 0 {
		t.Fatalf("OpIn[BR] should not match country=AR, got %v", res.Matched)
	}
}

// ----- Cartesian-product fan-out --------------------------------------

func TestExecuteOpInFanoutAcrossMultipleFields(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-or-ar-flight-or-train",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
			parser.SetCondition{Field: "product", Op: parser.OpIn, Values: []string{"flight", "train"}},
		}},
	})

	combinations := []map[string]string{
		{"country": "BR", "product": "flight"},
		{"country": "BR", "product": "train"},
		{"country": "AR", "product": "flight"},
		{"country": "AR", "product": "train"},
	}
	for _, c := range combinations {
		res, _ := e.Execute(context.Background(), engine.Request{Input: c})
		if len(res.Matched) != 1 || res.Matched[0] != "br-or-ar-flight-or-train" {
			t.Fatalf("Execute(%v): want match, got %v", c, res.Matched)
		}
	}
}

func TestExecuteOpInMixedWithOpEq(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "br-only-flight-or-train",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "product", Op: parser.OpIn, Values: []string{"flight", "train"}},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "product": "train"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("BR/train should match, got %v", res.Matched)
	}

	res, _ = e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "AR", "product": "train"},
	})
	if len(res.Matched) != 0 {
		t.Fatalf("AR should not match the BR-only rule, got %v", res.Matched)
	}
}

// ----- Canonicalization -----------------------------------------------

func TestOpInDeduplicatesValues(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "BR", "BR"}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 1 {
		t.Fatalf("Matched: want 1, got %d (dedup should have collapsed three BR values into one bucket)", len(res.Matched))
	}
}

func TestOpInOrderIndependent(t *testing.T) {
	// Two rules differ only in the order they list their OpIn values.
	// Both should produce the same bucket structure (i.e., the second
	// should be a duplicate-name detection trigger when registered after
	// being canonicalized identically -- but since we use different
	// names here, they end up in the same value-key bucket and
	// insertion order picks the first one).
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "a-listed-first",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "b-listed-different-order",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"AR", "BR"}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 1 || res.Matched[0] != "a-listed-first" {
		t.Fatalf("first-registered should win the tie, got %v", res.Matched)
	}
}

// ----- Rejections -----------------------------------------------------

func TestAddRuleRejectsOpInEmptySet(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "empty-in",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{}},
	})
	if !errors.Is(err, indexed.ErrNonIndexableCondition) {
		t.Fatalf("want ErrNonIndexableCondition, got %v", err)
	}
}

func TestAddRuleRejectsFanoutTooLarge(t *testing.T) {
	// 5 OpIn fields x 5 values each = 3125 > 1024 cap.
	values := []string{"a", "b", "c", "d", "e"}
	children := []parser.Condition{
		parser.SetCondition{Field: "f1", Op: parser.OpIn, Values: values},
		parser.SetCondition{Field: "f2", Op: parser.OpIn, Values: values},
		parser.SetCondition{Field: "f3", Op: parser.OpIn, Values: values},
		parser.SetCondition{Field: "f4", Op: parser.OpIn, Values: values},
		parser.SetCondition{Field: "f5", Op: parser.OpIn, Values: values},
	}

	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "fanout-bomb",
		Match: parser.AndCondition{Children: children},
	})

	var fanoutErr *indexed.FanoutTooLargeError
	if !errors.As(err, &fanoutErr) {
		t.Fatalf("want *FanoutTooLargeError, got %T (%v)", err, err)
	}
	if fanoutErr.RuleName() != "fanout-bomb" {
		t.Fatalf("RuleName: want fanout-bomb, got %s", fanoutErr.RuleName())
	}
	if fanoutErr.Limit != 1024 {
		t.Fatalf("Limit: want 1024, got %d", fanoutErr.Limit)
	}
	if fanoutErr.Cardinality <= fanoutErr.Limit {
		t.Fatalf("Cardinality should exceed Limit: got %d (limit %d)", fanoutErr.Cardinality, fanoutErr.Limit)
	}
}

func TestFanoutTooLargeErrorMessageIncludesNameAndCardinality(t *testing.T) {
	err := &indexed.FanoutTooLargeError{Rule: "huge", Cardinality: 9999, Limit: 1024}
	msg := err.Error()
	for _, want := range []string{"huge", "9999", "1024"} {
		if !contains(msg, want) {
			t.Fatalf("Error() should include %q, got %q", want, msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAddRuleAcceptsAtMaxFanout(t *testing.T) {
	// 2^10 = 1024 -- right at the limit, should be accepted.
	children := make([]parser.Condition, 10)
	for i := range children {
		children[i] = parser.SetCondition{
			Field:  "f" + string(rune('0'+i)),
			Op:     parser.OpIn,
			Values: []string{"a", "b"},
		}
	}
	e := indexed.New()
	err := e.AddRule(indexed.Rule{Name: "at-limit", Match: parser.AndCondition{Children: children}})
	if err != nil {
		t.Fatalf("AddRule at limit (1024): unexpected error: %v", err)
	}
}

// ----- Pointer-form OpIn ----------------------------------------------

func TestCollectAcceptsPointerSetCondition(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "ptr-in",
		Match: &parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR"}},
	})
	if err != nil {
		t.Fatalf("AddRule (pointer SetCondition): %v", err)
	}

	res, _ := e.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if len(res.Matched) != 1 {
		t.Fatalf("Matched: want 1, got %v", res.Matched)
	}
}

// Pointer-form pure OpNotIn: v0.10.0 admits OpNotIn as a
// post-filter when paired with indexable terms, but a rule with
// ONLY OpNotIn returns ErrNoIndexableTerms.
func TestAddRulePurePointerOpNotInRejectedAsNoIndexableTerms(t *testing.T) {
	e := indexed.New()
	err := e.AddRule(indexed.Rule{
		Name:  "ptr-not-in",
		Match: &parser.SetCondition{Field: "country", Op: parser.OpNotIn, Values: []string{"BR"}},
	})
	if !errors.Is(err, indexed.ErrNoIndexableTerms) {
		t.Fatalf("want ErrNoIndexableTerms, got %v", err)
	}
}

// ----- Wildcard semantics (ADR-0034 §3) -------------------------------

func TestWildcardRuleFiresOnAnyValueOfOmittedField(t *testing.T) {
	// A rule that only constrains "country" should match inputs that
	// also carry "tier" / "product" / etc -- those extra fields are
	// "wildcarded" via the key-set walker.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "br-anywhere",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	inputs := []map[string]string{
		{"country": "BR"},
		{"country": "BR", "tier": "premium"},
		{"country": "BR", "tier": "standard", "product": "flight"},
	}
	for _, in := range inputs {
		res, _ := e.Execute(context.Background(), engine.Request{Input: in})
		if len(res.Matched) != 1 || res.Matched[0] != "br-anywhere" {
			t.Fatalf("input %v should match br-anywhere, got %v", in, res.Matched)
		}
	}
}

func TestKeysetWalkOrderRespectsRegistrationAcrossWidths(t *testing.T) {
	// Two rules, different key-set widths, both could match. First-
	// registered key-set wins the tie (insertion order across
	// key-sets is the universal tie-break per ADR-0019).
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "wide",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "narrow",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})

	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "tier": "premium"},
	})
	if len(res.Matched) != 1 || res.Matched[0] != "wide" {
		t.Fatalf("first-registered key-set should win: want [wide], got %v", res.Matched)
	}
}

func TestMixedWildcardAndOpInRule(t *testing.T) {
	// Combination of wildcard (no field) + OpIn fan-out.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "any-tier-br-flight-or-train",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.SetCondition{Field: "product", Op: parser.OpIn, Values: []string{"flight", "train"}},
		}},
	})

	// Input includes "tier" which the rule does not constrain (wildcard
	// via omission). Should still match.
	res, _ := e.Execute(context.Background(), engine.Request{
		Input: map[string]string{"country": "BR", "product": "flight", "tier": "premium"},
	})
	if len(res.Matched) != 1 {
		t.Fatalf("BR + product=flight + tier=premium should match, got %v", res.Matched)
	}
}
