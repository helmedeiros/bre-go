package observability

import (
	"time"
)

// ExecutionMetric is the typed event the metrics decorator emits per
// Execute call. One value type covers success, error, and cancellation.
//
// Err and (Canceled, CancelReason) are mutually exclusive:
//   - On success: Err is nil; Canceled is false.
//   - On a true error: Err is set; Canceled is false.
//   - On context cancellation: Err is nil; Canceled is true and
//     CancelReason is "canceled" or "deadline_exceeded".
//
// This mirrors the v0.17 ADR-0042 lesson: cancellation is caller
// intent, not engine failure. Sinks that aggregate errors should
// gate on Err != nil; sinks that surface canceled rates should gate
// on Canceled.
type ExecutionMetric struct {
	Adapter      string
	MatchedCount int
	MatchedNames []string
	Duration     time.Duration
	Err          error
	Canceled     bool
	CancelReason string
}

// ExecutionMetricSink consumes typed metric events emitted by the
// metrics decorator. Implementations are the adapter half of the
// hexagonal port: bre-go owns this contract, backends (OTel,
// Prometheus, custom) adapt to it.
//
// RecordExecution must be safe for concurrent calls -- the decorator
// is called from inside Execute, which since v0.12.0 is concurrent-
// safe across the indexed adapter.
type ExecutionMetricSink interface {
	RecordExecution(ExecutionMetric)
}
