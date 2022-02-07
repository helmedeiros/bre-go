package firstmatch_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/observability"
)

var _ engine.ListenerHost = (*firstmatch.Engine)(nil)

type recordingListener struct {
	matches []observability.Match
}

func (r *recordingListener) OnRuleMatched(m observability.Match) {
	r.matches = append(r.matches, m)
}

func TestFirstmatchEngineSatisfiesListenerHost(t *testing.T) {
	var _ engine.ListenerHost = firstmatch.New()
}

func TestExecuteFiresOnRuleMatchedForTheFirstMatch(t *testing.T) {
	rec := &recordingListener{}
	e := newEngine(t,
		firstmatch.Rule{Name: "first", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "second", Condition: func(interface{}) bool { return true }},
	)
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.matches) != 1 {
		t.Fatalf("matches: want 1, got %d", len(rec.matches))
	}
}

func TestExecuteListenerSeesTheFirstMatchingRule(t *testing.T) {
	rec := &recordingListener{}
	e := newEngine(t,
		firstmatch.Rule{Name: "first", Condition: func(interface{}) bool { return true }},
		firstmatch.Rule{Name: "second", Condition: func(interface{}) bool { return true }},
	)
	e.AddListener(rec)

	_ = execute(t, e)

	if rec.matches[0].Rule != "first" {
		t.Fatalf("matches[0].Rule: want %q, got %q", "first", rec.matches[0].Rule)
	}
}

func TestExecuteFiresNoListenerEventWhenNothingMatches(t *testing.T) {
	rec := &recordingListener{}
	e := newEngine(t,
		firstmatch.Rule{Name: "never", Condition: func(interface{}) bool { return false }},
	)
	e.AddListener(rec)

	_ = execute(t, e)

	if len(rec.matches) != 0 {
		t.Fatalf("matches: want 0, got %d", len(rec.matches))
	}
}

func TestExecuteFiresForEveryRegisteredListener(t *testing.T) {
	first := &recordingListener{}
	second := &recordingListener{}
	e := newEngine(t,
		firstmatch.Rule{Name: "match", Condition: func(interface{}) bool { return true }},
	)
	e.AddListener(first)
	e.AddListener(second)

	_ = execute(t, e)

	if len(second.matches) != 1 {
		t.Fatalf("second listener: want 1 match, got %d", len(second.matches))
	}
}
