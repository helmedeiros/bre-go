package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

var _ engine.RuleLister = (*inmemory.Engine)(nil)

func TestInmemoryEngineSatisfiesRuleLister(t *testing.T) {
	var _ engine.RuleLister = inmemory.New()
}

func TestRuleNamesIsEmptyOnNewEngine(t *testing.T) {
	e := inmemory.New()

	if got := e.RuleNames(); len(got) != 0 {
		t.Fatalf("RuleNames: want empty, got %v", got)
	}
}

func TestRuleNamesContainsAddedRule(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleNames()

	if len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("RuleNames: want [alpha], got %v", got)
	}
}

func TestRuleNamesPreservesInsertionOrder(t *testing.T) {
	e := inmemory.New()
	for _, name := range []string{"third", "first", "second"} {
		_ = e.AddRule(inmemory.Rule{
			Name:      name,
			Condition: func(interface{}) bool { return true },
		})
	}

	got := e.RuleNames()

	want := []string{"third", "first", "second"}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("RuleNames[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

func TestRuleNamesReturnsAFreshCopy(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleNames()
	got[0] = "mutated"

	if again := e.RuleNames()[0]; again != "alpha" {
		t.Fatalf("mutating the returned slice changed engine state: got %q", again)
	}
}
