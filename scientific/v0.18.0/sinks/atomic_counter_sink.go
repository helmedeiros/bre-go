package sinks

import (
	"sync/atomic"

	"github.com/helmedeiros/bre-go/observability"
)

// AtomicCounterSink is a lock-free counter for the cases where you
// only need totals. Every method is safe for concurrent calls without
// any mutex; reads are also lock-free. Useful for hot paths where the
// metrics decorator must add ~zero overhead.
type AtomicCounterSink struct {
	executions       uint64
	matched          uint64
	errored          uint64
	canceled         uint64
	deadlineExceeded uint64
}

// RecordExecution atomically updates the relevant counters.
func (s *AtomicCounterSink) RecordExecution(m observability.ExecutionMetric) {
	atomic.AddUint64(&s.executions, 1)
	if m.MatchedCount > 0 {
		atomic.AddUint64(&s.matched, 1)
	}
	switch {
	case m.Err != nil:
		atomic.AddUint64(&s.errored, 1)
	case m.Canceled && m.CancelReason == "deadline_exceeded":
		atomic.AddUint64(&s.deadlineExceeded, 1)
	case m.Canceled:
		atomic.AddUint64(&s.canceled, 1)
	}
}

// Snapshot returns a lock-free read of every counter. Values may have
// been incremented after the read began; this is the standard
// eventual-consistency trade for monotonic counters.
func (s *AtomicCounterSink) Snapshot() (executions, matched, errored, canceled, deadlineExceeded uint64) {
	return atomic.LoadUint64(&s.executions),
		atomic.LoadUint64(&s.matched),
		atomic.LoadUint64(&s.errored),
		atomic.LoadUint64(&s.canceled),
		atomic.LoadUint64(&s.deadlineExceeded)
}
