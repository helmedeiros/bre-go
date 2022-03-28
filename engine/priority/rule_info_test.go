package priority_test

import (
	"reflect"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

var _ engine.RuleInfoLister = (*priority.Engine)(nil)

func TestPriorityEngineSatisfiesRuleInfoLister(t *testing.T) {
	var _ engine.RuleInfoLister = priority.New()
}

func TestRuleInfosCarriesDescriptionAndTags(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:        "blocklisted",
		Priority:    100,
		Description: "rejects tokens in the deny list",
		Tags:        []string{"compliance"},
		Condition:   func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if got[0].Description != "rejects tokens in the deny list" {
		t.Fatalf("Description: want %q, got %q", "rejects tokens in the deny list", got[0].Description)
	}
	if !reflect.DeepEqual(got[0].Tags, []string{"compliance"}) {
		t.Fatalf("Tags: want [compliance], got %v", got[0].Tags)
	}
}

func TestRuleInfosUsesInsertionOrderNotPriorityOrder(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "low",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "high",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if got[0].Name != "low" || got[1].Name != "high" {
		t.Fatalf("RuleInfos order: want insertion [low high], got [%s %s]", got[0].Name, got[1].Name)
	}
}
