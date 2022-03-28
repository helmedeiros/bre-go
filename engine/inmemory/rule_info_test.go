package inmemory_test

import (
	"reflect"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

var _ engine.RuleInfoLister = (*inmemory.Engine)(nil)

func TestInmemoryEngineSatisfiesRuleInfoLister(t *testing.T) {
	var _ engine.RuleInfoLister = inmemory.New()
}

func TestRuleInfosIsEmptyOnNewEngine(t *testing.T) {
	if got := inmemory.New().RuleInfos(); len(got) != 0 {
		t.Fatalf("RuleInfos: want empty, got %v", got)
	}
}

func TestRuleInfosCarriesDescription(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:        "alpha",
		Description: "rejects negative amounts",
		Condition:   func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if got[0].Description != "rejects negative amounts" {
		t.Fatalf("Description: want %q, got %q", "rejects negative amounts", got[0].Description)
	}
}

func TestRuleInfosCarriesTags(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Tags:      []string{"fraud", "high-priority"},
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if !reflect.DeepEqual(got[0].Tags, []string{"fraud", "high-priority"}) {
		t.Fatalf("Tags: want [fraud high-priority], got %v", got[0].Tags)
	}
}

func TestRuleInfosLeavesUnsetMetadataEmpty(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	got := e.RuleInfos()

	if got[0].Description != "" {
		t.Fatalf("Description: want empty, got %q", got[0].Description)
	}
	if got[0].Tags != nil {
		t.Fatalf("Tags: want nil, got %v", got[0].Tags)
	}
}

func TestRuleInfosPreservesInsertionOrder(t *testing.T) {
	e := inmemory.New()
	for _, name := range []string{"third", "first", "second"} {
		_ = e.AddRule(inmemory.Rule{
			Name:      name,
			Condition: func(interface{}) bool { return true },
		})
	}

	got := e.RuleInfos()

	want := []string{"third", "first", "second"}
	for i, w := range want {
		if got[i].Name != w {
			t.Fatalf("RuleInfos[%d].Name: want %q, got %q", i, w, got[i].Name)
		}
	}
}
