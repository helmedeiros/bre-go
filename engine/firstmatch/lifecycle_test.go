package firstmatch_test

import (
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/observability"
)

type lifecycleRecorder struct {
	started  []interface{}
	finished []finishedEvent
	matches  []observability.Match
}

type finishedEvent struct {
	input    interface{}
	output   interface{}
	matched  []string
	duration time.Duration
}

func (r *lifecycleRecorder) OnRuleMatched(m observability.Match) {
	r.matches = append(r.matches, m)
}

func (r *lifecycleRecorder) OnExecutionStarted(input interface{}) {
	r.started = append(r.started, input)
}

func (r *lifecycleRecorder) OnExecutionFinished(input interface{}, output interface{}, matched []string, duration time.Duration) {
	r.finished = append(r.finished, finishedEvent{input, output, matched, duration})
}

func TestExecuteFiresOnExecutionStartedOnce(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := newEngine(t, firstmatch.Rule{Name: "alpha", Condition: func(interface{}) bool { return true }})
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.started) != 1 {
		t.Fatalf("started: want 1 event, got %d", len(rec.started))
	}
}

func TestExecuteFiresOnExecutionFinishedOnceOnAMatch(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := newEngine(t, firstmatch.Rule{Name: "alpha", Condition: func(interface{}) bool { return true }})
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.finished) != 1 {
		t.Fatalf("finished: want 1 event, got %d", len(rec.finished))
	}
}

func TestExecuteFiresOnExecutionFinishedOnceWhenNothingMatches(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := newEngine(t, firstmatch.Rule{Name: "never", Condition: func(interface{}) bool { return false }})
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.finished) != 1 {
		t.Fatalf("finished: want 1 event even when no rule matched, got %d", len(rec.finished))
	}
}

func TestExecutionFinishedReportsTheFirstMatchedRule(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := newEngine(t,
		firstmatch.Rule{Name: "first", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "second", Condition: func(interface{}) bool { return true }},
	)
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.finished[0].matched) != 1 || rec.finished[0].matched[0] != "first" {
		t.Fatalf("finished[0].matched: want [first], got %v", rec.finished[0].matched)
	}
}

func TestExecutionFinishedReportsEmptyMatchedWhenNothingFires(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := newEngine(t, firstmatch.Rule{Name: "never", Condition: func(interface{}) bool { return false }})
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.finished[0].matched) != 0 {
		t.Fatalf("finished[0].matched: want empty, got %v", rec.finished[0].matched)
	}
}
