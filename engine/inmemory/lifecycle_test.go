package inmemory_test

import (
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
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
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: 42})

	if len(rec.started) != 1 {
		t.Fatalf("started: want 1 event, got %d", len(rec.started))
	}
}

func TestExecuteFiresOnExecutionFinishedOnce(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: 42})

	if len(rec.finished) != 1 {
		t.Fatalf("finished: want 1 event, got %d", len(rec.finished))
	}
}

func TestExecutionStartedReceivesTheRequestInput(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: "hello"})

	if rec.started[0] != "hello" {
		t.Fatalf("started[0]: want %q, got %v", "hello", rec.started[0])
	}
}

func TestExecutionFinishedReceivesTheMatchedNames(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: 42})

	if len(rec.finished[0].matched) != 1 || rec.finished[0].matched[0] != "alpha" {
		t.Fatalf("finished[0].matched: want [alpha], got %v", rec.finished[0].matched)
	}
}

func TestExecutionFinishedReportsANonZeroDuration(t *testing.T) {
	rec := &lifecycleRecorder{}
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: 42})

	if rec.finished[0].duration < 0 {
		t.Fatalf("finished[0].duration: want >= 0, got %v", rec.finished[0].duration)
	}
}

func TestExecutionLifecycleAcceptsAListenerWithoutLifecycleMethods(t *testing.T) {
	plain := &recordingListener{}
	e := engineWithMatchingRule(t, inmemory.New(), "alpha")
	e.AddListener(plain)

	_, _ = e.Execute(engine.Request{Input: 42})

	if len(plain.matches) != 1 {
		t.Fatalf("plain listener: want 1 match, got %d", len(plain.matches))
	}
}
