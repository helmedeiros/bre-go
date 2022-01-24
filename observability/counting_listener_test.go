package observability_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/observability"
)

func TestCountingListenerSatisfiesExecutionListener(t *testing.T) {
	var _ observability.ExecutionListener = &observability.CountingListener{}
}

func TestCountingListenerCountStartsAtZero(t *testing.T) {
	c := &observability.CountingListener{}

	if got := c.Count("missing"); got != 0 {
		t.Fatalf("Count(missing): want 0, got %d", got)
	}
}

func TestCountingListenerTotalStartsAtZero(t *testing.T) {
	c := &observability.CountingListener{}

	if got := c.Total(); got != 0 {
		t.Fatalf("Total: want 0, got %d", got)
	}
}

func TestCountingListenerCountsOneMatchPerRule(t *testing.T) {
	c := &observability.CountingListener{}

	c.OnRuleMatched(observability.Match{Rule: "alpha"})

	if got := c.Count("alpha"); got != 1 {
		t.Fatalf("Count(alpha): want 1, got %d", got)
	}
}

func TestCountingListenerAccumulatesMatchesForSameRule(t *testing.T) {
	c := &observability.CountingListener{}

	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "alpha"})

	if got := c.Count("alpha"); got != 3 {
		t.Fatalf("Count(alpha): want 3, got %d", got)
	}
}

func TestCountingListenerCountsRulesIndependently(t *testing.T) {
	c := &observability.CountingListener{}

	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "beta"})

	if got := c.Count("beta"); got != 1 {
		t.Fatalf("Count(beta): want 1, got %d", got)
	}
}

func TestCountingListenerTotalSumsAllMatches(t *testing.T) {
	c := &observability.CountingListener{}

	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "beta"})

	if got := c.Total(); got != 3 {
		t.Fatalf("Total: want 3, got %d", got)
	}
}

func TestCountingListenerCountReturnsZeroForUnknownRule(t *testing.T) {
	c := &observability.CountingListener{}
	c.OnRuleMatched(observability.Match{Rule: "alpha"})

	if got := c.Count("never-seen"); got != 0 {
		t.Fatalf("Count(never-seen): want 0, got %d", got)
	}
}
