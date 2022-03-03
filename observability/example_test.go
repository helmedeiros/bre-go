package observability_test

import (
	"fmt"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

func ExampleCountingListener() {
	c := &observability.CountingListener{}
	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "alpha"})
	c.OnRuleMatched(observability.Match{Rule: "beta"})

	fmt.Printf("alpha=%d beta=%d total=%d\n", c.Count("alpha"), c.Count("beta"), c.Total())
	// Output: alpha=2 beta=1 total=3
}

type stdoutLogger struct{}

func (stdoutLogger) Info(msg string, fields ...observability.Field) {
	fmt.Printf("%s rule=%v\n", msg, fields[0].Value)
}

func (stdoutLogger) Error(string, ...observability.Field) {}

func ExampleLoggingListener() {
	l := observability.NewLoggingListener(stdoutLogger{})
	l.OnRuleMatched(observability.Match{Rule: "alpha"})
	// Output: rule matched rule=alpha
}

func ExampleTimingListener() {
	tl := &observability.TimingListener{}

	tl.OnExecutionStarted(nil)
	tl.OnRuleMatched(observability.Match{Rule: "a"})
	tl.OnRuleMatched(observability.Match{Rule: "b"})
	tl.OnExecutionFinished(nil, nil, nil, 5*time.Millisecond)

	fmt.Printf("matches=%d observed=%v\n", tl.MatchesInLastExecution(), tl.HasObservedExecution())
	// Output: matches=2 observed=true
}
