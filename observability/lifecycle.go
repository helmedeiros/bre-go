package observability

import "time"

// ExecutionStartedListener observes the start of an engine execution.
// Implemented in addition to ExecutionListener when a listener needs a
// pre-rule hook (tracing span open, latency start).
type ExecutionStartedListener interface {
	OnExecutionStarted(input interface{})
}

// ExecutionFinishedListener observes the end of an engine execution.
// Implemented in addition to ExecutionListener when a listener needs
// the per-execution summary (duration, output, matched names).
type ExecutionFinishedListener interface {
	OnExecutionFinished(input interface{}, output interface{}, matched []string, duration time.Duration)
}

// TimingListener records the duration of the most recent execution.
// Safe for one execution at a time; the zero value is usable.
type TimingListener struct {
	last     time.Duration
	matches  int
	executed bool
}

// OnRuleMatched counts a rule match for the in-flight execution.
func (t *TimingListener) OnRuleMatched(Match) {
	t.matches++
}

// OnExecutionStarted resets the match counter for a new execution.
func (t *TimingListener) OnExecutionStarted(interface{}) {
	t.matches = 0
}

// OnExecutionFinished records the elapsed duration.
func (t *TimingListener) OnExecutionFinished(_ interface{}, _ interface{}, _ []string, duration time.Duration) {
	t.last = duration
	t.executed = true
}

// LastDuration returns the duration of the most recent Execute, or 0
// before any execution has been observed.
func (t *TimingListener) LastDuration() time.Duration {
	return t.last
}

// MatchesInLastExecution returns the count of OnRuleMatched callbacks
// observed during the most recent execution.
func (t *TimingListener) MatchesInLastExecution() int {
	return t.matches
}

// HasObservedExecution reports whether OnExecutionFinished has fired
// at least once.
func (t *TimingListener) HasObservedExecution() bool {
	return t.executed
}
