package engine_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func TestEverySeededAdapterExecutesUnderThePort(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedInmemory},
		{name: "firstmatch", seed: seedFirstmatch},
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
