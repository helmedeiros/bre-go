package indexed_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ADR-0039 tests: Engine.Diagnose dead-rule detection.

func TestDiagnoseEmptyEngineReturnsEmptyReport(t *testing.T) {
	e := indexed.New()
	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("empty engine: want no dead rules, got %v", report.DeadRules)
	}
}

func TestDiagnoseSingleRuleReturnsEmptyReport(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "only",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("single rule: want no dead rules, got %v", report.DeadRules)
	}
}

func TestDiagnoseIdenticalRulesSecondIsDead(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "first",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "second",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 1 {
		t.Fatalf("want 1 dead rule, got %v", report.DeadRules)
	}
	if report.DeadRules[0].Name != "second" {
		t.Fatalf("Name: want second, got %s", report.DeadRules[0].Name)
	}
	if report.DeadRules[0].ShadowedBy != "first" {
		t.Fatalf("ShadowedBy: want first, got %s", report.DeadRules[0].ShadowedBy)
	}
}

func TestDiagnoseNarrowerAfterBroaderIsDead(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "broader",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "narrower",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 1 || report.DeadRules[0].Name != "narrower" {
		t.Fatalf("want narrower dead, got %v", report.DeadRules)
	}
	if report.DeadRules[0].ShadowedBy != "broader" {
		t.Fatalf("ShadowedBy: want broader, got %s", report.DeadRules[0].ShadowedBy)
	}
}

func TestDiagnoseBroaderAfterNarrowerIsAlive(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "narrower",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "broader",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("broader after narrower should not be dead, got %v", report.DeadRules)
	}
}

func TestDiagnoseDisjointRulesNotDead(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "a",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "b",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "AR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "c",
		Match: parser.StringCondition{Field: "product", Op: parser.OpEq, Value: "flight"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("disjoint rules should produce no dead rules, got %v", report.DeadRules)
	}
}

func TestDiagnoseOpInShadowsOpEqSubset(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "set",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "single",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 1 || report.DeadRules[0].Name != "single" {
		t.Fatalf("want single dead, got %v", report.DeadRules)
	}
}

func TestDiagnoseOpEqDoesNotShadowOpInSuperset(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "single",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "set",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR"}},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("OpEq doesn't shadow superset OpIn, got %v", report.DeadRules)
	}
}

func TestDiagnoseEarlierWithPostFilterSkipsShadowing(t *testing.T) {
	// Conservative tier-1: we can't decide whether earlier fires,
	// so we don't report a shadow.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name: "earlier-with-postfilter",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "later-pure-eq",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("post-filter on earlier rule should prevent dead-rule report (conservative tier-1), got %v", report.DeadRules)
	}
}

func TestDiagnoseLaterWithPostFilterStillDead(t *testing.T) {
	// Earlier has no post-filter, so it fires before later's post-filter would.
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "earlier",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "later-with-postfilter",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpNeq, Value: "vip"},
		}},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 1 || report.DeadRules[0].Name != "later-with-postfilter" {
		t.Fatalf("later-with-postfilter should still be dead, got %v", report.DeadRules)
	}
}

func TestDiagnoseReportsOnlyFirstShadower(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "first-broad",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "second-also-broad",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "third-narrow",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			parser.StringCondition{Field: "tier", Op: parser.OpEq, Value: "premium"},
		}},
	})

	report := e.Diagnose()
	// Expect at least two: "second-also-broad" shadowed by "first-broad",
	// "third-narrow" shadowed by "first-broad" (the first matcher in order).
	foundThird := false
	for _, d := range report.DeadRules {
		if d.Name == "third-narrow" {
			foundThird = true
			if d.ShadowedBy != "first-broad" {
				t.Fatalf("third-narrow should be shadowed by first-broad (first in order), got %s", d.ShadowedBy)
			}
		}
	}
	if !foundThird {
		t.Fatalf("expected third-narrow in dead list, got %v", report.DeadRules)
	}
}

func TestDiagnoseWorksInBuilderPhase(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "a",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "b",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	if e.Built() {
		t.Fatalf("Diagnose precondition: engine should NOT be built")
	}
	report := e.Diagnose()
	if len(report.DeadRules) != 1 {
		t.Fatalf("Diagnose in builder phase: want 1 dead, got %v", report.DeadRules)
	}
	if e.Built() {
		t.Fatalf("Diagnose should NOT trigger implicit Build")
	}
}

func TestDiagnoseWorksInBuiltPhase(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "a",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "b",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	_ = e.Build()

	report := e.Diagnose()
	if len(report.DeadRules) != 1 {
		t.Fatalf("Diagnose in built phase: want 1 dead, got %v", report.DeadRules)
	}
}

func TestDeadRuleReasonIsConsistent(t *testing.T) {
	e := indexed.New()
	_ = e.AddRule(indexed.Rule{
		Name:  "a",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "b",
		Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 1 {
		t.Fatalf("expected exactly one dead rule")
	}
	if report.DeadRules[0].Reason == "" {
		t.Fatalf("Reason should be set")
	}
}

// hookOnlyCondition is admitted only via WithPostFilterHook;
// Diagnose treats rules using it as unanalyzable.
type hookOnlyCondition struct {
	Field string
}

func (h *hookOnlyCondition) Eval(map[string]interface{}) bool { return true }

func TestDiagnoseCustomConditionRuleIsUnanalyzable(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool {
		_, ok := c.(*hookOnlyCondition)
		return ok
	})
	_ = e.AddRule(indexed.Rule{
		Name: "with-custom",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&hookOnlyCondition{Field: "extra"},
		}},
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "would-be-shadowed",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})

	report := e.Diagnose()
	if len(report.DeadRules) != 0 {
		t.Fatalf("rule with custom hook condition has post-filter; should not shadow. Got %v", report.DeadRules)
	}
}

func TestDiagnoseLaterRuleWithCustomConditionIsUnanalyzable(t *testing.T) {
	e := indexed.New().WithPostFilterHook(func(c parser.Condition) bool {
		_, ok := c.(*hookOnlyCondition)
		return ok
	})
	_ = e.AddRule(indexed.Rule{
		Name:  "broad",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	})
	_ = e.AddRule(indexed.Rule{
		Name: "narrow-with-custom",
		Match: parser.AndCondition{Children: []parser.Condition{
			parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
			&hookOnlyCondition{Field: "ctx"},
		}},
	})

	report := e.Diagnose()
	// "narrow-with-custom" has the broad rule's constraints + a
	// custom term, AND Diagnose can't extract its shape (the custom
	// condition isn't recognized without the hook context). So
	// later is unanalyzable and skipped entirely.
	if len(report.DeadRules) != 0 {
		t.Fatalf("later rule with unanalyzable shape should be skipped, got %v", report.DeadRules)
	}
}

func TestDiagnoseTruthTable(t *testing.T) {
	type rule struct {
		name  string
		match parser.Condition
	}
	type setup struct {
		name       string
		rules      []rule
		wantDead   []string // names expected to appear in DeadRules
		wantShadow map[string]string
	}
	cases := []setup{
		{
			name: "same field same value -> later dead",
			rules: []rule{
				{"r1", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
				{"r2", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
			},
			wantDead:   []string{"r2"},
			wantShadow: map[string]string{"r2": "r1"},
		},
		{
			name: "same field different value -> alive",
			rules: []rule{
				{"r1", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
				{"r2", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "2"}},
			},
			wantDead: nil,
		},
		{
			name: "different fields, no shadow",
			rules: []rule{
				{"r1", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
				{"r2", parser.StringCondition{Field: "b", Op: parser.OpEq, Value: "2"}},
			},
			wantDead: nil,
		},
		{
			name: "OpIn earlier shadows narrower OpEq later",
			rules: []rule{
				{"r1", parser.SetCondition{Field: "a", Op: parser.OpIn, Values: []string{"1", "2", "3"}}},
				{"r2", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "2"}},
			},
			wantDead:   []string{"r2"},
			wantShadow: map[string]string{"r2": "r1"},
		},
		{
			name: "OpEq earlier does NOT shadow OpIn later with extra value",
			rules: []rule{
				{"r1", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
				{"r2", parser.SetCondition{Field: "a", Op: parser.OpIn, Values: []string{"1", "2"}}},
			},
			wantDead: nil,
		},
		{
			name: "shared field shadowed, extra field on later still dead",
			rules: []rule{
				{"r1", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
				{"r2", parser.AndCondition{Children: []parser.Condition{
					parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"},
					parser.StringCondition{Field: "b", Op: parser.OpEq, Value: "x"},
				}}},
			},
			wantDead:   []string{"r2"},
			wantShadow: map[string]string{"r2": "r1"},
		},
		{
			name: "earlier constrains more fields -> later not dead",
			rules: []rule{
				{"r1", parser.AndCondition{Children: []parser.Condition{
					parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"},
					parser.StringCondition{Field: "b", Op: parser.OpEq, Value: "x"},
				}}},
				{"r2", parser.StringCondition{Field: "a", Op: parser.OpEq, Value: "1"}},
			},
			wantDead: nil,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			e := indexed.New()
			for _, r := range tc.rules {
				if err := e.AddRule(indexed.Rule{Name: r.name, Match: r.match}); err != nil {
					t.Fatalf("AddRule(%s): %v", r.name, err)
				}
			}
			report := e.Diagnose()
			if len(report.DeadRules) != len(tc.wantDead) {
				t.Fatalf("DeadRules len: want %d, got %d (%v)", len(tc.wantDead), len(report.DeadRules), report.DeadRules)
			}
			for _, want := range tc.wantDead {
				found := false
				for _, d := range report.DeadRules {
					if d.Name == want {
						found = true
						if exp, ok := tc.wantShadow[want]; ok && d.ShadowedBy != exp {
							t.Fatalf("ShadowedBy for %s: want %s, got %s", want, exp, d.ShadowedBy)
						}
						break
					}
				}
				if !found {
					t.Fatalf("expected %s in dead list, got %v", want, report.DeadRules)
				}
			}
		})
	}
}
