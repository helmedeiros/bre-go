package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func TestRuleInfosTagsIsAFreshCopy(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Tags:      []string{"original"},
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleInfos()
	got[0].Tags[0] = "mutated"

	again := e.RuleInfos()
	if again[0].Tags[0] != "original" {
		t.Fatalf("mutating returned Tags changed engine state: got %q", again[0].Tags[0])
	}
}
