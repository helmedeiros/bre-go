package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

type bufferLogger struct {
	calls int
}

func (b *bufferLogger) Info(string, ...observability.Field)  { b.calls++ }
func (b *bufferLogger) Error(string, ...observability.Field) {}

func TestExecuteFiresEveryListenerInACompositeWiring(t *testing.T) {
	counter := &observability.CountingListener{}
	timing := &observability.TimingListener{}
	logger := &bufferLogger{}
	logging := observability.NewLoggingListener(logger)

	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in },
	})
	e.AddListener(counter)
	e.AddListener(timing)
	e.AddListener(logging)

	_, _ = e.Execute(engine.Request{Input: 1})

	if counter.Total() != 1 {
		t.Fatalf("counter.Total: want 1, got %d", counter.Total())
	}
	if timing.MatchesInLastExecution() != 1 {
		t.Fatalf("timing.MatchesInLastExecution: want 1, got %d", timing.MatchesInLastExecution())
	}
	if !timing.HasObservedExecution() {
		t.Fatalf("timing.HasObservedExecution: want true, got false")
	}
	if logger.calls != 1 {
		t.Fatalf("bufferLogger.calls: want 1, got %d", logger.calls)
	}
}

func TestCompositeWiringSurvivesARuleSetWithTwoMatchingRules(t *testing.T) {
	counter := &observability.CountingListener{}
	timing := &observability.TimingListener{}

	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "beta",
		Condition: func(interface{}) bool { return true },
	})
	e.AddListener(counter)
	e.AddListener(timing)

	_, _ = e.Execute(engine.Request{Input: 1})

	if counter.Total() != 2 {
		t.Fatalf("counter.Total: want 2, got %d", counter.Total())
	}
	if timing.MatchesInLastExecution() != 2 {
		t.Fatalf("timing.MatchesInLastExecution: want 2, got %d", timing.MatchesInLastExecution())
	}
}
