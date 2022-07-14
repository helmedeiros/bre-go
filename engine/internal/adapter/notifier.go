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

// Notifier wires observability.ExecutionListener callbacks. Embed
// it (by value) into an adapter's Engine struct; the methods are
// promoted to the embedding type so the adapter's public surface
// gains AddListener for free.
//
// The zero value of Notifier is usable. Notifier is safe for
// concurrent use across all four methods plus AddListener.
//
// Concurrency model (ADR-0037): the listener slice is held in an
// atomic.Value via copy-on-write. AddListener serializes via mu,
// loads the current slice, builds a fresh slice with one more
// element, stores. Notify* methods Load the slice once and iterate
// — lockless on the read path. Callers attaching listeners while
// Execute runs see the new listener on subsequent Execute calls
// (no synchronization point inside a single Execute).
type Notifier struct {
	mu        sync.Mutex   // serializes AddListener
	listeners atomic.Value // []observability.ExecutionListener
}

// AddListener registers l. Subsequent Notify* calls dispatch to l
// (and to every other registered listener) via type assertion at
// notify time, so a listener that only implements OnRuleMatched
// receives only matched events.
func (n *Notifier) AddListener(l observability.ExecutionListener) {
	n.mu.Lock()
	defer n.mu.Unlock()
	cur := n.load()
	next := make([]observability.ExecutionListener, len(cur)+1)
	copy(next, cur)
	next[len(cur)] = l
	n.listeners.Store(next)
}

// load returns the current listener slice via the atomic Value.
// Returns nil when no listener has ever been added (atomic.Value
// returns nil on first Load).
func (n *Notifier) load() []observability.ExecutionListener {
	v := n.listeners.Load()
	if v == nil {
		return nil
	}
	return v.([]observability.ExecutionListener)
}

// NotifyMatched fires OnRuleMatched on every registered listener.
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

// NotifyStarted fires OnExecutionStarted on every listener that
// implements observability.ExecutionStartedListener.
func (n *Notifier) NotifyStarted(input interface{}) {
	listeners := n.load()
	for _, l := range listeners {
		if started, ok := l.(observability.ExecutionStartedListener); ok {
			started.OnExecutionStarted(input)
		}
	}
}

// NotifyFinished fires OnExecutionFinished on every listener that
// implements observability.ExecutionFinishedListener.
func (n *Notifier) NotifyFinished(input, output interface{}, matched []string, duration time.Duration) {
	listeners := n.load()
	for _, l := range listeners {
		if finished, ok := l.(observability.ExecutionFinishedListener); ok {
			finished.OnExecutionFinished(input, output, matched, duration)
		}
	}
}

// NotifyErrored fires OnExecutionErrored on every listener that
// implements observability.ExecutionErroredListener.
func (n *Notifier) NotifyErrored(input interface{}, err error) {
	listeners := n.load()
	for _, l := range listeners {
		if errored, ok := l.(observability.ExecutionErroredListener); ok {
			errored.OnExecutionErrored(input, err)
		}
	}
}
