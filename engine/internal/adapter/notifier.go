// Package adapter holds internal helpers shared by the
// engine.Engine adapters in bre-go. Not importable from outside the
// module (Go's internal/ convention).
package adapter

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

// Notifier provides listener fan-out. Embed by value into an
// adapter's Engine; method promotion gives the adapter AddListener
// and the Notify* methods for free. The zero value is usable and
// safe for concurrent calls: AddListener serializes via mu and
// copy-on-write stores the new slice; Notify* methods Load lockless.
type Notifier struct {
	mu        sync.Mutex   // serializes AddListener
	listeners atomic.Value // []observability.ExecutionListener
}

// AddListener registers l. Lifecycle dispatch is via type assertion
// at notify time, so listeners only opt into the events they implement.
func (n *Notifier) AddListener(l observability.ExecutionListener) {
	n.mu.Lock()
	defer n.mu.Unlock()
	cur := n.load()
	next := make([]observability.ExecutionListener, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = l
	n.listeners.Store(next)
}

func (n *Notifier) load() []observability.ExecutionListener {
	v := n.listeners.Load()
	if v == nil {
		return nil
	}
	return v.([]observability.ExecutionListener)
}

// NotifyMatched fires OnRuleMatched on every listener.
func (n *Notifier) NotifyMatched(rule string, input, output interface{}) {
	listeners := n.load()
	if len(listeners) == 0 {
		return
	}
	m := observability.Match{Rule: rule, Input: input, Output: output}
	for _, l := range listeners {
		l.OnRuleMatched(m)
	}
}

// NotifyStarted fires OnExecutionStarted on listeners that implement it.
func (n *Notifier) NotifyStarted(input interface{}) {
	listeners := n.load()
	for _, l := range listeners {
		if started, ok := l.(observability.ExecutionStartedListener); ok {
			started.OnExecutionStarted(input)
		}
	}
}

// NotifyFinished fires OnExecutionFinished on listeners that implement it.
func (n *Notifier) NotifyFinished(input, output interface{}, matched []string, duration time.Duration) {
	listeners := n.load()
	for _, l := range listeners {
		if finished, ok := l.(observability.ExecutionFinishedListener); ok {
			finished.OnExecutionFinished(input, output, matched, duration)
		}
	}
}

// NotifyErrored fires OnExecutionErrored on listeners that implement it.
func (n *Notifier) NotifyErrored(input interface{}, err error) {
	listeners := n.load()
	for _, l := range listeners {
		if errored, ok := l.(observability.ExecutionErroredListener); ok {
			errored.OnExecutionErrored(input, err)
		}
	}
}
