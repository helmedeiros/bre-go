package adapter_test

import (
	"errors"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine/internal/adapter"
	"github.com/helmedeiros/bre-go/observability"
)

func TestZeroValueNotifierIsUsable(t *testing.T) {
	var n adapter.Notifier
	// Should not panic.
	n.NotifyMatched("any", nil, nil)
}

func TestAddListenerRetainsTheListener(t *testing.T) {
	n := &adapter.Notifier{}
	snap := &observability.SnapshotListener{}

	n.AddListener(snap)
	n.NotifyMatched("alpha", "in", "out")

	if len(snap.Matches) != 1 {
		t.Fatalf("Matches: want 1, got %d", len(snap.Matches))
	}
}

func TestNotifyMatchedFiresOnEveryListener(t *testing.T) {
	first := &observability.SnapshotListener{}
	second := &observability.SnapshotListener{}
	n := &adapter.Notifier{}
	n.AddListener(first)
	n.AddListener(second)

	n.NotifyMatched("alpha", nil, nil)

	if len(first.Matches) != 1 || len(second.Matches) != 1 {
		t.Fatalf("both listeners should see the match")
	}
}

func TestNotifyMatchedCarriesRuleNameInputOutput(t *testing.T) {
	snap := &observability.SnapshotListener{}
	n := &adapter.Notifier{}
	n.AddListener(snap)

	n.NotifyMatched("the-rule", "the-input", "the-output")

	got := snap.Matches[0]
	if got.Rule != "the-rule" || got.Input != "the-input" || got.Output != "the-output" {
		t.Fatalf("Match: got %+v", got)
	}
}

func TestNotifyStartedFiresOnlyOnLifecycleListeners(t *testing.T) {
	snap := &observability.SnapshotListener{}
	plain := &plainListener{}
	n := &adapter.Notifier{}
	n.AddListener(snap)
	n.AddListener(plain)

	n.NotifyStarted("input")

	if len(snap.Started) != 1 {
		t.Fatalf("SnapshotListener should see Started")
	}
	if plain.startedCalls != 0 {
		t.Fatalf("plain listener should NOT receive Started (does not implement the interface)")
	}
}

func TestNotifyFinishedCarriesEventFields(t *testing.T) {
	snap := &observability.SnapshotListener{}
	n := &adapter.Notifier{}
	n.AddListener(snap)

	n.NotifyFinished("in", "out", []string{"r1", "r2"}, 5*time.Millisecond)

	got := snap.Finished[0]
	if got.Input != "in" || got.Output != "out" || got.Duration != 5*time.Millisecond {
		t.Fatalf("Finished: got %+v", got)
	}
	if len(got.Matched) != 2 || got.Matched[0] != "r1" {
		t.Fatalf("Finished.Matched: got %v", got.Matched)
	}
}

func TestNotifyErroredFiresOnlyOnErroredListeners(t *testing.T) {
	snap := &observability.SnapshotListener{}
	plain := &plainListener{}
	n := &adapter.Notifier{}
	n.AddListener(snap)
	n.AddListener(plain)

	boom := errors.New("boom")
	n.NotifyErrored("in", boom)

	if len(snap.Errored) != 1 {
		t.Fatalf("SnapshotListener should see Errored")
	}
	if plain.erroredCalls != 0 {
		t.Fatalf("plain listener should NOT receive Errored (does not implement the interface)")
	}
}

func TestNotifyErroredCarriesInputAndError(t *testing.T) {
	snap := &observability.SnapshotListener{}
	n := &adapter.Notifier{}
	n.AddListener(snap)

	boom := errors.New("boom")
	n.NotifyErrored("the-input", boom)

	got := snap.Errored[0]
	if got.Input != "the-input" || !errors.Is(got.Err, boom) {
		t.Fatalf("Errored: got %+v", got)
	}
}

func TestNotifyMatchedFiresEvenWithNoLifecycleSupport(t *testing.T) {
	plain := &plainListener{}
	n := &adapter.Notifier{}
	n.AddListener(plain)

	n.NotifyMatched("any", nil, nil)
	n.NotifyStarted(nil)
	n.NotifyFinished(nil, nil, nil, 0)
	n.NotifyErrored(nil, errors.New("e"))

	if plain.matchedCalls != 1 {
		t.Fatalf("plain listener should see exactly 1 OnRuleMatched, got %d", plain.matchedCalls)
	}
	// And the lifecycle methods should not have been called on the plain listener.
	if plain.startedCalls != 0 || plain.finishedCalls != 0 || plain.erroredCalls != 0 {
		t.Fatalf("plain listener received unexpected lifecycle callbacks: %+v", plain)
	}
}

// plainListener implements only ExecutionListener (the narrow one),
// not the lifecycle variants. Used to verify the Notifier's type-
// assertion dispatch.
type plainListener struct {
	matchedCalls  int
	startedCalls  int
	finishedCalls int
	erroredCalls  int
}

func (p *plainListener) OnRuleMatched(observability.Match) { p.matchedCalls++ }
