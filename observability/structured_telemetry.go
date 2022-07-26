package observability

import (
	"time"
)

// TelemetryRecord captures one terminal lifecycle event of an Execute call.
// Error path emits two records: one with Err set, one with Err nil.
type TelemetryRecord struct {
	Input    interface{}
	Output   interface{}
	Matched  []string
	Duration time.Duration
	Err      error
}

// TelemetrySink consumes records. Must be safe for concurrent calls.
type TelemetrySink func(TelemetryRecord)

// StructuredTelemetryListener implements every lifecycle interface and
// emits records via sink.
type StructuredTelemetryListener struct {
	sink TelemetrySink
}

// NewStructuredTelemetryListener wires a sink. Panics on nil sink to surface
// the wiring bug rather than silently dropping telemetry.
func NewStructuredTelemetryListener(sink TelemetrySink) *StructuredTelemetryListener {
	if sink == nil {
		panic("observability: NewStructuredTelemetryListener called with nil sink")
	}
	return &StructuredTelemetryListener{sink: sink}
}

func (l *StructuredTelemetryListener) OnRuleMatched(Match) {}

func (l *StructuredTelemetryListener) OnExecutionStarted(interface{}) {}

func (l *StructuredTelemetryListener) OnExecutionFinished(input, output interface{}, matched []string, duration time.Duration) {
	l.sink(TelemetryRecord{
		Input:    input,
		Output:   output,
		Matched:  matched,
		Duration: duration,
	})
}

func (l *StructuredTelemetryListener) OnExecutionErrored(input interface{}, err error) {
	l.sink(TelemetryRecord{
		Input: input,
		Err:   err,
	})
}
