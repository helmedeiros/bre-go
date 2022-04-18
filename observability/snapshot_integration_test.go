package observability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func TestSnapshotListenerCapturesEveryLifecycleEventOnInmemory(t *testing.T) {
	snap := &observability.SnapshotListener{}
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in },
	})
	e.AddListener(snap)

	_, _ = e.Execute(context.Background(), engine.Request{Input: 42})

	if len(snap.Started) != 1 {
		t.Fatalf("Started: want 1 event, got %d", len(snap.Started))
	}
	if len(snap.Matches) != 1 {
		t.Fatalf("Matches: want 1, got %d", len(snap.Matches))
	}
	if len(snap.Finished) != 1 {
		t.Fatalf("Finished: want 1 event, got %d", len(snap.Finished))
	}
	if len(snap.Errored) != 0 {
		t.Fatalf("Errored: want 0, got %d", len(snap.Errored))
	}
}

func TestSnapshotListenerCapturesPanicAsErrored(t *testing.T) {
	snap := &observability.SnapshotListener{}
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	})
	e.AddListener(snap)

	_, _ = e.Execute(context.Background(), engine.Request{Input: 42})

	if len(snap.Errored) != 1 {
		t.Fatalf("Errored: want 1 event after panic, got %d", len(snap.Errored))
	}

	var pe *inmemory.ActionPanicError
	if !errors.As(snap.Errored[0].Err, &pe) {
		t.Fatalf("Errored[0].Err: want *ActionPanicError, got %T", snap.Errored[0].Err)
	}

	if len(snap.Finished) != 1 {
		t.Fatalf("Finished: want 1 event even on panic, got %d", len(snap.Finished))
	}
}

func TestSnapshotListenerResetEnablesReuse(t *testing.T) {
	snap := &observability.SnapshotListener{}
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})
	e.AddListener(snap)

	_, _ = e.Execute(context.Background(), engine.Request{Input: 1})
	snap.Reset()
	_, _ = e.Execute(context.Background(), engine.Request{Input: 2})

	if len(snap.Started) != 1 || len(snap.Matches) != 1 || len(snap.Finished) != 1 {
		t.Fatalf("post-Reset capture: Started=%d Matches=%d Finished=%d",
			len(snap.Started), len(snap.Matches), len(snap.Finished))
	}
}
