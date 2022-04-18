package inmemory_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

var _ engine.ListenerHost = (*inmemory.Engine)(nil)

func TestInmemoryEngineSatisfiesListenerHost(t *testing.T) {
	var _ engine.ListenerHost = inmemory.New()
}

func TestListenerHostAssertionAllowsListenerRegistration(t *testing.T) {
	var eng engine.Engine = inmemory.New()
	host, ok := eng.(engine.ListenerHost)
	if !ok {
		t.Fatalf("engine does not satisfy ListenerHost")
	}

	counter := &observability.CountingListener{}
	host.AddListener(counter)
	_ = engineWithMatchingRule(t, eng.(*inmemory.Engine), "fire")
	_, _ = eng.Execute(context.Background(), engine.Request{Input: "x"})

	if counter.Total() != 1 {
		t.Fatalf("counter total: want 1, got %d", counter.Total())
	}
}
