package firstmatch_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

var _ engine.RuleLister = (*firstmatch.Engine)(nil)

func TestFirstmatchEngineSatisfiesRuleLister(t *testing.T) {
	var _ engine.RuleLister = firstmatch.New()
}

func TestRuleNamesIsEmptyOnNewEngine(t *testing.T) {
	e := firstmatch.New()

	if got := e.RuleNames(); len(got) != 0 {
		t.Fatalf("RuleNames: want empty, got %v", got)
	}
}

func TestRuleNamesReflectsThePrecedenceChain(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{Name: "premium", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "standard", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "default", Condition: func(interface{}) bool { return true }},
	)

	got := e.RuleNames()

	want := []string{"premium", "standard", "default"}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("RuleNames[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

func TestRuleNamesReturnsAFreshCopy(t *testing.T) {
	e := newEngine(t,
		firstmatch.Rule{Name: "alpha", Condition: func(interface{}) bool { return true }},
	)

	got := e.RuleNames()
	got[0] = "mutated"

	if again := e.RuleNames()[0]; again != "alpha" {
		t.Fatalf("mutating the returned slice changed engine state: got %q", again)
	}
}
