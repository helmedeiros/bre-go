// Package sinks contains three alternative ExecutionMetricSink
// implementations written from scratch against the v0.18.0 metrics
// port. Each one is independent -- it doesn't import the others,
// doesn't depend on OTel, and serves as evidence that the port
// surface is small enough that arbitrary backends can be written
// quickly. See ../REPORT.md for the analysis.
package sinks

import "github.com/helmedeiros/bre-go/observability"

// ChannelSink fans every recorded metric out on a buffered channel.
// Useful for pipelining metrics into a downstream goroutine that
// pushes to a slow / blocking destination (an HTTP exporter, a
// Kafka producer) without blocking Execute. If the channel is full
// the metric is dropped; the dropped count is observable via Dropped().
type ChannelSink struct {
	C       chan observability.ExecutionMetric
	dropped int64
}

// NewChannelSink returns a sink backed by a buffered channel of size cap.
func NewChannelSink(capacity int) *ChannelSink {
	return &ChannelSink{C: make(chan observability.ExecutionMetric, capacity)}
}

// RecordExecution sends m to the channel if capacity allows; otherwise
// increments the dropped counter.
func (s *ChannelSink) RecordExecution(m observability.ExecutionMetric) {
	select {
	case s.C <- m:
	default:
		s.dropped++
	}
}

// Dropped returns how many metrics were dropped because the channel
// was full. Not safe under concurrent RecordExecution callers; intended
// for single-producer scenarios.
func (s *ChannelSink) Dropped() int64 { return s.dropped }
