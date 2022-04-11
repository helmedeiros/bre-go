package observability_test

import (
	"errors"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

func TestSnapshotListenerSatisfiesExecutionListener(t *testing.T) {
	var _ observability.ExecutionListener = &observability.SnapshotListener{}
}

func TestSnapshotListenerSatisfiesExecutionStartedListener(t *testing.T) {
	var _ observability.ExecutionStartedListener = &observability.SnapshotListener{}
}

func TestSnapshotListenerSatisfiesExecutionFinishedListener(t *testing.T) {
	var _ observability.ExecutionFinishedListener = &observability.SnapshotListener{}
}

func TestSnapshotListenerSatisfiesExecutionErroredListener(t *testing.T) {
	var _ observability.ExecutionErroredListener = &observability.SnapshotListener{}
}

func TestSnapshotListenerCapturesMatches(t *testing.T) {
	s := &observability.SnapshotListener{}

	s.OnRuleMatched(observability.Match{Rule: "alpha"})
	s.OnRuleMatched(observability.Match{Rule: "beta"})

	if len(s.Matches) != 2 {
		t.Fatalf("Matches: want 2, got %d", len(s.Matches))
	}
}

func TestSnapshotListenerPreservesMatchOrder(t *testing.T) {
	s := &observability.SnapshotListener{}

	s.OnRuleMatched(observability.Match{Rule: "first"})
	s.OnRuleMatched(observability.Match{Rule: "second"})

	if s.Matches[0].Rule != "first" || s.Matches[1].Rule != "second" {
		t.Fatalf("Matches order: want [first second], got [%s %s]",
			s.Matches[0].Rule, s.Matches[1].Rule)
	}
}

func TestSnapshotListenerCapturesStartedInput(t *testing.T) {
	s := &observability.SnapshotListener{}

	s.OnExecutionStarted("hello")

	if len(s.Started) != 1 || s.Started[0] != "hello" {
		t.Fatalf("Started: want [hello], got %v", s.Started)
	}
}

func TestSnapshotListenerCapturesFinishedFields(t *testing.T) {
	s := &observability.SnapshotListener{}

	s.OnExecutionFinished("in", "out", []string{"r"}, 5*time.Millisecond)

	if len(s.Finished) != 1 {
		t.Fatalf("Finished: want 1 event, got %d", len(s.Finished))
	}
	got := s.Finished[0]
	if got.Input != "in" || got.Output != "out" || got.Duration != 5*time.Millisecond {
		t.Fatalf("Finished[0] fields: got %+v", got)
	}
}

func TestSnapshotListenerCapturesErroredFields(t *testing.T) {
	s := &observability.SnapshotListener{}
	boom := errors.New("boom")

	s.OnExecutionErrored("in", boom)

	if len(s.Errored) != 1 {
		t.Fatalf("Errored: want 1 event, got %d", len(s.Errored))
	}
	if s.Errored[0].Input != "in" || !errors.Is(s.Errored[0].Err, boom) {
		t.Fatalf("Errored[0] fields: got %+v", s.Errored[0])
	}
}

func TestSnapshotListenerResetClearsAllSlices(t *testing.T) {
	s := &observability.SnapshotListener{}
	s.OnRuleMatched(observability.Match{Rule: "x"})
	s.OnExecutionStarted("in")
	s.OnExecutionFinished("in", nil, nil, 0)
	s.OnExecutionErrored("in", errors.New("e"))

	s.Reset()

	if len(s.Matches) != 0 || len(s.Started) != 0 || len(s.Finished) != 0 || len(s.Errored) != 0 {
		t.Fatalf("Reset did not clear all slices: M=%d S=%d F=%d E=%d",
			len(s.Matches), len(s.Started), len(s.Finished), len(s.Errored))
	}
}

func TestSnapshotListenerZeroValueIsUsable(t *testing.T) {
	var s observability.SnapshotListener

	s.OnRuleMatched(observability.Match{Rule: "x"})

	if len(s.Matches) != 1 {
		t.Fatalf("zero-value listener did not capture: %v", s.Matches)
	}
}
