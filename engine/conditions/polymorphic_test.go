package conditions_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/conditions"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func TestCombinatorsDropIntoEveryAdapter(t *testing.T) {
	cond := conditions.And(
		conditions.Not(conditions.Never()),
		conditions.Or(conditions.Always(), conditions.Never()),
	)

	for _, tc := range []struct {
		name string
		seed func(t *testing.T, c func(interface{}) bool) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemoryWith},
		{name: "firstmatch", seed: seedFirstmatchWith},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t, cond)

			res, err := eng.Execute(context.Background(), engine.Request{Input: "anything"})
			if err != nil {
				t.Fatalf("Execute: unexpected error: %v", err)
			}
			if len(res.Matched) != 1 || res.Matched[0] != "combinator-rule" {
				t.Fatalf("Matched: want [combinator-rule], got %v", res.Matched)
			}
		})
	}
}

func seedInmemoryWith(t *testing.T, c func(interface{}) bool) engine.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(inmemory.Rule{Name: "combinator-rule", Condition: c}); err != nil {
		t.Fatalf("inmemory.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedFirstmatchWith(t *testing.T, c func(interface{}) bool) engine.Engine {
	t.Helper()
	e := firstmatch.New()
	if err := e.AddRule(firstmatch.Rule{Name: "combinator-rule", Condition: c}); err != nil {
		t.Fatalf("firstmatch.AddRule: unexpected error: %v", err)
	}
	return e
}
