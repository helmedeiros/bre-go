package priority_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
	"github.com/helmedeiros/bre-go/observability"
)

var _ engine.ListenerHost = (*priority.Engine)(nil)
var _ engine.RuleLister = (*priority.Engine)(nil)

type recordingListener struct {
	matches []observability.Match
}

func (r *recordingListener) OnRuleMatched(m observability.Match) {
	r.matches = append(r.matches, m)
}

func TestPriorityEngineSatisfiesListenerHost(t *testing.T) {
	var _ engine.ListenerHost = priority.New()
}

func TestPriorityEngineSatisfiesRuleLister(t *testing.T) {
	var _ engine.RuleLister = priority.New()
}

func TestExecuteFiresOnRuleMatchedForTheHighestPriorityRule(t *testing.T) {
	rec := &recordingListener{}
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "low",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "high",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
	})
	e.AddListener(rec)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if len(rec.matches) != 1 {
		t.Fatalf("matches: want 1, got %d", len(rec.matches))
	}
}

func TestExecuteListenerSeesTheHighestPriorityMatch(t *testing.T) {
	rec := &recordingListener{}
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "low",
		Priority:  1,
		Condition: func(interface{}) bool { return true },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "high",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
	})
	e.AddListener(rec)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if rec.matches[0].Rule != "high" {
		t.Fatalf("matches[0].Rule: want %q, got %q", "high", rec.matches[0].Rule)
	}
}

func TestExecuteFiresForEveryRegisteredListener(t *testing.T) {
	first := &recordingListener{}
	second := &recordingListener{}
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "match",
		Condition: func(interface{}) bool { return true },
	})
	e.AddListener(first)
	e.AddListener(second)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if len(second.matches) != 1 {
		t.Fatalf("second listener: want 1 match, got %d", len(second.matches))
	}
}

func TestExecuteFiresNoEventWhenNothingMatches(t *testing.T) {
	rec := &recordingListener{}
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "never",
		Priority:  100,
		Condition: func(interface{}) bool { return false },
	})
	e.AddListener(rec)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if len(rec.matches) != 0 {
		t.Fatalf("matches: want 0, got %d", len(rec.matches))
	}
}
