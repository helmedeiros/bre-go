package observability

import "time"

// FinishedEvent captures the arguments observed by OnExecutionFinished
// on a SnapshotListener. Stored in insertion order in
// SnapshotListener.Finished.
type FinishedEvent struct {
	Input    interface{}
	Output   interface{}
	Matched  []string
	Duration time.Duration
}

// ErroredEvent captures the arguments observed by OnExecutionErrored
// on a SnapshotListener. Stored in insertion order in
// SnapshotListener.Errored.
type ErroredEvent struct {
	Input interface{}
	Err   error
}

// SnapshotListener captures every lifecycle event for later assertion.
// Implements ExecutionListener, ExecutionStartedListener,
// ExecutionFinishedListener, and ExecutionErroredListener -- adapters
// see all four through one type assertion per role.
//
// Designed for tests: build the engine, attach a SnapshotListener,
// execute, then assert on the captured slices. The zero value is
// usable; the slices grow as events arrive. Safe for one execution
// at a time.
type SnapshotListener struct {
	Matches  []Match
	Started  []interface{}
	Finished []FinishedEvent
	Errored  []ErroredEvent
}

// OnRuleMatched appends m to Matches.
func (s *SnapshotListener) OnRuleMatched(m Match) {
	s.Matches = append(s.Matches, m)
}

// OnExecutionStarted appends input to Started.
func (s *SnapshotListener) OnExecutionStarted(input interface{}) {
	s.Started = append(s.Started, input)
}

// OnExecutionFinished appends a FinishedEvent to Finished.
func (s *SnapshotListener) OnExecutionFinished(input interface{}, output interface{}, matched []string, duration time.Duration) {
	s.Finished = append(s.Finished, FinishedEvent{
		Input:    input,
		Output:   output,
		Matched:  matched,
		Duration: duration,
	})
}

// OnExecutionErrored appends an ErroredEvent to Errored.
func (s *SnapshotListener) OnExecutionErrored(input interface{}, err error) {
	s.Errored = append(s.Errored, ErroredEvent{Input: input, Err: err})
}

// Reset clears every captured slice so the listener can be reused
// across executions in a single test.
func (s *SnapshotListener) Reset() {
	s.Matches = nil
	s.Started = nil
	s.Finished = nil
	s.Errored = nil
}
