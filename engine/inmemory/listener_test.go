package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

type recordingListener struct {
	matches []observability.Match
}

func (r *recordingListener) OnRuleMatched(m observability.Match) {
	r.matches = append(r.matches, m)
}

func TestAddListenerStoresListener(t *testing.T) {
	e := inmemory.New()
	rec := &recordingListener{}

	e.AddListener(rec)
	_ = execute(t, engineWithMatchingRule(t, e, "any"), "x")

	if len(rec.matches) != 1 {
		t.Fatalf("matches: want 1, got %d", len(rec.matches))
	}
}

func TestExecuteFiresOnRuleMatched(t *testing.T) {
	rec := &recordingListener{}
	e := engineWithMatchingRule(t, inmemory.New(), "fire-me")
	e.AddListener(rec)

	_ = execute(t, e, "x")

	if rec.matches[0].Rule != "fire-me" {
		t.Fatalf("matches[0].Rule: want %q, got %q", "fire-me", rec.matches[0].Rule)
	}
}

func TestExecutePassesInputToListener(t *testing.T) {
	rec := &recordingListener{}
	e := engineWithMatchingRule(t, inmemory.New(), "any")
	e.AddListener(rec)

	_ = execute(t, e, 42)

	if rec.matches[0].Input != 42 {
		t.Fatalf("matches[0].Input: want 42, got %v", rec.matches[0].Input)
	}
}

func TestExecutePassesActionOutputToListener(t *testing.T) {
	rec := &recordingListener{}
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "doubler",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in.(int) * 2 },
	})
	e.AddListener(rec)

	_ = execute(t, e, 21)

	if rec.matches[0].Output != 42 {
		t.Fatalf("matches[0].Output: want 42, got %v", rec.matches[0].Output)
	}
}

func TestExecuteDoesNotFireForUnmatchedRules(t *testing.T) {
	rec := &recordingListener{}
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "never",
		Condition: func(interface{}) bool { return false },
	})
	e.AddListener(rec)

	_ = execute(t, e, "x")

	if len(rec.matches) != 0 {
		t.Fatalf("matches: want 0, got %d", len(rec.matches))
	}
}

func TestExecuteFiresForEveryRegisteredListener(t *testing.T) {
	first := &recordingListener{}
	second := &recordingListener{}
	e := engineWithMatchingRule(t, inmemory.New(), "any")
	e.AddListener(first)
	e.AddListener(second)

	_ = execute(t, e, "x")

	if len(second.matches) != 1 {
		t.Fatalf("second listener: want 1 match, got %d", len(second.matches))
	}
}

func engineWithMatchingRule(t *testing.T, e *inmemory.Engine, name string) *inmemory.Engine {
	t.Helper()
	if err := e.AddRule(inmemory.Rule{
		Name:      name,
		Condition: func(interface{}) bool { return true },
	}); err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
	return e
}
