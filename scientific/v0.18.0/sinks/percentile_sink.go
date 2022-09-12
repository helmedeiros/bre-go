package sinks

import (
	"sort"
	"sync"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

// PercentileSink tracks a fixed-size sliding window of durations and
// exposes p50/p95/p99 reads. Older samples roll off as the window
// fills. Useful when you don't have a downstream metrics backend with
// histogram support and want a quick latency dashboard from inside
// the process.
type PercentileSink struct {
	mu      sync.Mutex
	samples []time.Duration
	head    int
	size    int
	cap     int
}

// NewPercentileSink returns a sink with the given window capacity. A
// capacity of zero is treated as 1024 by default.
func NewPercentileSink(capacity int) *PercentileSink {
	if capacity <= 0 {
		capacity = 1024
	}
	return &PercentileSink{samples: make([]time.Duration, capacity), cap: capacity}
}

// RecordExecution appends m.Duration to the sliding window. Safe for
// concurrent calls; reads serialize on the same mutex.
func (s *PercentileSink) RecordExecution(m observability.ExecutionMetric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples[s.head] = m.Duration
	s.head = (s.head + 1) % s.cap
	if s.size < s.cap {
		s.size++
	}
}

// Percentiles returns p50/p95/p99 of the samples currently in the
// window. Returns zeros if the window is empty.
func (s *PercentileSink) Percentiles() (p50, p95, p99 time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size == 0 {
		return 0, 0, 0
	}
	sorted := make([]time.Duration, s.size)
	copy(sorted, s.samples[:s.size])
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[pct(s.size, 0.50)], sorted[pct(s.size, 0.95)], sorted[pct(s.size, 0.99)]
}

func pct(n int, p float64) int {
	i := int(float64(n) * p)
	if i >= n {
		return n - 1
	}
	return i
}
