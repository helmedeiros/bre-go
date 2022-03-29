package engine_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/engine/priority"
	"github.com/helmedeiros/bre-go/observability"
)

func TestEverySeededAdapterExecutesUnderThePort(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemory},
		{name: "firstmatch", seed: seedFirstmatch},
		{name: "priority", seed: seedPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t)

			res, err := eng.Execute(engine.Request{Input: "anything"})
			if err != nil {
				t.Fatalf("Execute: unexpected error: %v", err)
			}
			if len(res.Matched) == 0 {
				t.Fatalf("Matched: want at least one entry, got empty")
			}
		})
	}
}

func TestEveryListenerHostFiresThroughTheTypeAssertion(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemory},
		{name: "firstmatch", seed: seedFirstmatch},
		{name: "priority", seed: seedPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t)
			host, ok := eng.(engine.ListenerHost)
			if !ok {
				t.Fatalf("%s: want engine.ListenerHost, got plain Engine", tc.name)
			}
			counter := &observability.CountingListener{}
			host.AddListener(counter)

			_, _ = eng.Execute(engine.Request{Input: "anything"})

			if counter.Total() == 0 {
				t.Fatalf("counter.Total: want > 0, got 0")
			}
		})
	}
}

func TestEveryRuleListerEnumeratesThroughTheTypeAssertion(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemory},
		{name: "firstmatch", seed: seedFirstmatch},
		{name: "priority", seed: seedPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t)
			lister, ok := eng.(engine.RuleLister)
			if !ok {
				t.Fatalf("%s: want engine.RuleLister, got plain Engine", tc.name)
			}

			names := lister.RuleNames()

			if len(names) != 1 || names[0] != "always" {
				t.Fatalf("RuleNames: want [always], got %v", names)
			}
		})
	}
}

func TestEveryAdapterRecoversFromPanickingActions(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedPanickingInmemory},
		{name: "firstmatch", seed: seedPanickingFirstmatch},
		{name: "priority", seed: seedPanickingPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t)

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic leaked from Execute: %v", r)
				}
			}()
			_, err := eng.Execute(engine.Request{Input: nil})

			if err == nil {
				t.Fatalf("Execute: want non-nil error from panicking action, got nil")
			}
		})
	}
}

func seedPanickingInmemory(t *testing.T) engine.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(inmemory.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	}); err != nil {
		t.Fatalf("inmemory.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedPanickingFirstmatch(t *testing.T) engine.Engine {
	t.Helper()
	e := firstmatch.New()
	if err := e.AddRule(firstmatch.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	}); err != nil {
		t.Fatalf("firstmatch.AddRule: unexpected error: %v", err)
	}
	return e
}

func TestEveryRuleInfoListerSurfacesMetadataThroughTheTypeAssertion(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemory},
		{name: "firstmatch", seed: seedFirstmatch},
		{name: "priority", seed: seedPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			eng := tc.seed(t)
			lister, ok := eng.(engine.RuleInfoLister)
			if !ok {
				t.Fatalf("%s: want engine.RuleInfoLister, got plain Engine", tc.name)
			}

			infos := lister.RuleInfos()

			if len(infos) != 1 || infos[0].Name != "always" {
				t.Fatalf("RuleInfos: want one entry with Name=always, got %v", infos)
			}
		})
	}
}

func seedInmemory(t *testing.T) engine.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	}); err != nil {
		t.Fatalf("inmemory.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedFirstmatch(t *testing.T) engine.Engine {
	t.Helper()
	e := firstmatch.New()
	if err := e.AddRule(firstmatch.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	}); err != nil {
		t.Fatalf("firstmatch.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedPriority(t *testing.T) engine.Engine {
	t.Helper()
	e := priority.New()
	if err := e.AddRule(priority.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	}); err != nil {
		t.Fatalf("priority.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedPanickingPriority(t *testing.T) engine.Engine {
	t.Helper()
	e := priority.New()
	if err := e.AddRule(priority.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	}); err != nil {
		t.Fatalf("priority.AddRule: unexpected error: %v", err)
	}
	return e
}
