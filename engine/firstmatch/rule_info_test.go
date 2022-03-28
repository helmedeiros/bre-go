package firstmatch_test

import (
	"reflect"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

var _ engine.RuleInfoLister = (*firstmatch.Engine)(nil)

func TestFirstmatchEngineSatisfiesRuleInfoLister(t *testing.T) {
	var _ engine.RuleInfoLister = firstmatch.New()
}

func TestRuleInfosCarriesDescriptionAndTags(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:        "vip-block",
		Description: "denies vip-tier on blocklist",
		Tags:        []string{"compliance", "block"},
		Condition:   func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if got[0].Description != "denies vip-tier on blocklist" {
		t.Fatalf("Description: want %q, got %q", "denies vip-tier on blocklist", got[0].Description)
	}
	if !reflect.DeepEqual(got[0].Tags, []string{"compliance", "block"}) {
		t.Fatalf("Tags: want [compliance block], got %v", got[0].Tags)
	}
}
