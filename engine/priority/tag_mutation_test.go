package priority_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/priority"
)

func TestRuleInfosTagsIsAFreshCopy(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
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
