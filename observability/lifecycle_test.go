package observability_test

import (
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

func TestTimingListenerSatisfiesExecutionListener(t *testing.T) {
	var _ observability.ExecutionListener = &observability.TimingListener{}
}

func TestTimingListenerSatisfiesExecutionStartedListener(t *testing.T) {
	var _ observability.ExecutionStartedListener = &observability.TimingListener{}
}

func TestTimingListenerSatisfiesExecutionFinishedListener(t *testing.T) {
	var _ observability.ExecutionFinishedListener = &observability.TimingListener{}
}

func TestTimingListenerHasObservedExecutionStartsFalse(t *testing.T) {
	tl := &observability.TimingListener{}

	if tl.HasObservedExecution() {
		t.Fatalf("HasObservedExecution: want false on zero value, got true")
	}
}

func TestTimingListenerLastDurationStartsAtZero(t *testing.T) {
	tl := &observability.TimingListener{}

	if tl.LastDuration() != 0 {
		t.Fatalf("LastDuration: want 0, got %v", tl.LastDuration())
	}
}

func TestTimingListenerRecordsTheReportedDuration(t *testing.T) {
	tl := &observability.TimingListener{}

	tl.OnExecutionFinished(nil, nil, nil, 42*time.Millisecond)

	if tl.LastDuration() != 42*time.Millisecond {
		t.Fatalf("LastDuration: want 42ms, got %v", tl.LastDuration())
	}
}

func TestTimingListenerMarksAsObservedAfterFinish(t *testing.T) {
	tl := &observability.TimingListener{}

	tl.OnExecutionFinished(nil, nil, nil, time.Millisecond)

	if !tl.HasObservedExecution() {
		t.Fatalf("HasObservedExecution: want true after OnExecutionFinished, got false")
	}
}

func TestTimingListenerCountsMatchesInLastExecution(t *testing.T) {
	tl := &observability.TimingListener{}

	tl.OnExecutionStarted(nil)
	tl.OnRuleMatched(observability.Match{Rule: "a"})
	tl.OnRuleMatched(observability.Match{Rule: "b"})
	tl.OnExecutionFinished(nil, nil, nil, time.Millisecond)

	if tl.MatchesInLastExecution() != 2 {
		t.Fatalf("MatchesInLastExecution: want 2, got %d", tl.MatchesInLastExecution())
	}
}

func TestTimingListenerStartResetsMatchCount(t *testing.T) {
	tl := &observability.TimingListener{}
	tl.OnRuleMatched(observability.Match{Rule: "a"})

	tl.OnExecutionStarted(nil)

	if tl.MatchesInLastExecution() != 0 {
		t.Fatalf("MatchesInLastExecution after Started: want 0, got %d", tl.MatchesInLastExecution())
	}
}
