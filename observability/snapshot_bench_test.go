package observability_test

import (
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

func BenchmarkSnapshotListenerOnRuleMatched(b *testing.B) {
	s := &observability.SnapshotListener{}
	m := observability.Match{Rule: "x", Input: 42, Output: 84}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.OnRuleMatched(m)
	}
}

func BenchmarkSnapshotListenerFullLifecycle(b *testing.B) {
	s := &observability.SnapshotListener{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.OnExecutionStarted(42)
		s.OnRuleMatched(observability.Match{Rule: "x"})
		s.OnExecutionFinished(42, 84, []string{"x"}, time.Microsecond)
		s.Reset()
	}
}

func BenchmarkNopExecutionListenerOnRuleMatched(b *testing.B) {
	var n observability.NopExecutionListener
	m := observability.Match{Rule: "x"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n.OnRuleMatched(m)
	}
}
