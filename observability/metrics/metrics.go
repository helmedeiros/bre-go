// Package metrics decorates engine.Engine with aggregate metric
// emission via the observability.ExecutionMetricSink port.
//
// bre-go owns the port; backends (OTel, Prometheus, custom) implement
// the sink. The decorator depends only on the port -- this is the
// hexagonal architecture's Dependency Inversion principle realized in
// code. See ADR-0043 for the design rationale.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

// Wrap returns inner decorated to emit one ExecutionMetric per
// Execute via sink. The returned value satisfies engine.Engine.
// Optional capability interfaces (RuleLister, RuleInfoLister,
// ListenerHost) forward to inner via the standard type-assertion
// idiom on the returned value.
func Wrap(inner engine.Engine, sink observability.ExecutionMetricSink) engine.Engine {
	return &meteredEngine{inner: inner, sink: sink}
}

type meteredEngine struct {
	inner engine.Engine
	sink  observability.ExecutionMetricSink
}

// Execute calls inner.Execute, builds an ExecutionMetric from the
// outcome, and hands it to the sink before returning to the caller.
// Caller-visible behavior is identical to inner; only the side-effect
// changes.
func (m *meteredEngine) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
	start := time.Now()
	res, err := m.inner.Execute(ctx, req)
	duration := time.Since(start)

	metric := observability.ExecutionMetric{
		Adapter:      fmt.Sprintf("%T", m.inner),
		MatchedCount: len(res.Matched),
		MatchedNames: res.Matched,
		Duration:     duration,
	}
	switch {
	case err == nil:
		// success
	case errors.Is(err, context.Canceled):
		metric.Canceled = true
		metric.CancelReason = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		metric.Canceled = true
		metric.CancelReason = "deadline_exceeded"
	default:
		metric.Err = err
	}
	m.sink.RecordExecution(metric)
	return res, err
}

// RuleNames forwards to inner when it implements engine.RuleLister.
// Returns nil if inner does not.
func (m *meteredEngine) RuleNames() []string {
	if rl, ok := m.inner.(engine.RuleLister); ok {
		return rl.RuleNames()
	}
	return nil
}

// RuleInfos forwards to inner when it implements engine.RuleInfoLister.
// Returns nil if inner does not.
func (m *meteredEngine) RuleInfos() []engine.RuleInfo {
	if rl, ok := m.inner.(engine.RuleInfoLister); ok {
		return rl.RuleInfos()
	}
	return nil
}

// AddListener forwards to inner when it implements engine.ListenerHost.
// No-op if inner does not.
func (m *meteredEngine) AddListener(l observability.ExecutionListener) {
	if lh, ok := m.inner.(engine.ListenerHost); ok {
		lh.AddListener(l)
	}
}

// Unwrap returns the inner engine the metrics decorator is wrapping.
// Useful for callers that need adapter-specific methods (e.g.,
// indexed.Engine.Build, indexed.Engine.Diagnose) and that have stacked
// multiple decorators.
func (m *meteredEngine) Unwrap() engine.Engine { return m.inner }

// RecordingSink is a thread-safe ExecutionMetricSink that appends every
// recorded metric to an internal slice. Useful for tests and for
// consumers who want a simple in-memory aggregation buffer.
//
// Concurrent calls to RecordExecution are serialized via a sync.Mutex;
// readers of Records must call the Records() accessor (which returns
// a defensive copy) rather than touching the field directly.
type RecordingSink struct {
	mu      sync.Mutex
	records []observability.ExecutionMetric
}

// RecordExecution stores m. Safe for concurrent calls.
func (s *RecordingSink) RecordExecution(m observability.ExecutionMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, m)
}

// Records returns a snapshot of every metric recorded so far. Safe for
// concurrent calls; the returned slice is a defensive copy.
func (s *RecordingSink) Records() []observability.ExecutionMetric {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]observability.ExecutionMetric, len(s.records))
	copy(out, s.records)
	return out
}

// Reset clears the recorded metrics. Safe for concurrent calls.
func (s *RecordingSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
}
