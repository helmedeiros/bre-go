package observability

import (
	"time"
)

// TelemetryRecord captures the lifecycle of a single Execute call
// in a structured form. Emitted by StructuredTelemetryListener via
// the caller-supplied TelemetrySink.
//
// Emission model: one record per terminal lifecycle event.
//   - Success path: one record from OnExecutionFinished, Err nil.
//   - Error path: two records -- one from OnExecutionErrored
//     (Err non-nil, partial Output/Matched data), then one from
//     OnExecutionFinished (Err nil, partial Output/Matched).
//
// Sinks that want exactly one record per Execute should
// deduplicate by timestamp + Input pointer. Most consumers
// (loggers, metrics counters) treat the events independently and
// don't need correlation. See ADR-0038 for the design rationale.
type TelemetryRecord struct {
	Input    interface{}
	Output   interface{}
	Matched  []string
	Duration time.Duration
	Err      error
}

// TelemetrySink consumes a structured telemetry record. Called
// once per terminal lifecycle event by StructuredTelemetryListener.
//
// Sinks MUST be safe for concurrent calls when the engine is
// built and Execute runs from multiple goroutines (the v0.12.0
// concurrency model). The library does not synchronize the sink.
type TelemetrySink func(TelemetryRecord)

// StructuredTelemetryListener implements every observability
// lifecycle interface and emits a TelemetryRecord per terminal
// event via the configured sink. Wire it on any engine via
// AddListener; the listener works identically across all four
// adapters.
//
// Added in v0.13.0 per ADR-0038. The last Phase-4 parity-closure
// piece.
type StructuredTelemetryListener struct {
	sink TelemetrySink
}

// NewStructuredTelemetryListener returns a listener that emits
// records via sink. Sink must not be nil; nil would silently
// swallow telemetry, which is a wiring bug the constructor
// surfaces immediately with panic.
func NewStructuredTelemetryListener(sink TelemetrySink) *StructuredTelemetryListener {
	if sink == nil {
		panic("observability: NewStructuredTelemetryListener called with nil sink")
	}
	return &StructuredTelemetryListener{sink: sink}
}

// OnRuleMatched implements ExecutionListener. No-op; per-match
// info is rolled up into TelemetryRecord.Matched at the terminal
// event.
func (l *StructuredTelemetryListener) OnRuleMatched(Match) {}

// OnExecutionStarted implements ExecutionStartedListener. No-op;
// the duration is delivered in the terminal event.
func (l *StructuredTelemetryListener) OnExecutionStarted(interface{}) {}

// OnExecutionFinished implements ExecutionFinishedListener.
// Emits a TelemetryRecord with Err nil; Matched and Duration
// reflect the values the engine passed.
func (l *StructuredTelemetryListener) OnExecutionFinished(input, output interface{}, matched []string, duration time.Duration) {
	l.sink(TelemetryRecord{
		Input:    input,
		Output:   output,
		Matched:  matched,
		Duration: duration,
	})
}

// OnExecutionErrored implements ExecutionErroredListener. Emits
// a TelemetryRecord with Err set. Note that
// OnExecutionFinished still fires after this; sinks see two
// records for one errored Execute call. See ADR-0038 §2.
func (l *StructuredTelemetryListener) OnExecutionErrored(input interface{}, err error) {
	l.sink(TelemetryRecord{
		Input: input,
		Err:   err,
	})
}
