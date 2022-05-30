// Package adapter holds internal helpers shared by the
// engine.Engine adapters in bre-go. Not importable from outside the
// module (Go's internal/ convention).
package adapter

import (
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

// Notifier wires observability.ExecutionListener callbacks. Embed
// it (by value) into an adapter's Engine struct; the methods are
// promoted to the embedding type so the adapter's public surface
// gains AddListener for free.
//
// The zero value of Notifier is usable. Safe for one execution at a
// time (matching the engines that embed it).
type Notifier struct {
	listeners []observability.ExecutionListener
}

// AddListener registers l. Subsequent Notify* calls dispatch to l
// (and to every other registered listener) via type assertion at
// notify time, so a listener that only implements OnRuleMatched
// receives only matched events.
func (n *Notifier) AddListener(l observability.ExecutionListener) {
	n.listeners = append(n.listeners, l)
}

// NotifyMatched fires OnRuleMatched on every registered listener.
func (n *Notifier) NotifyMatched(rule string, input, output interface{}) {
	m := observability.Match{Rule: rule, Input: input, Output: output}
	for _, l := range n.listeners {
		l.OnRuleMatched(m)
	}
}

// NotifyStarted fires OnExecutionStarted on every listener that
// implements observability.ExecutionStartedListener.
func (n *Notifier) NotifyStarted(input interface{}) {
	for _, l := range n.listeners {
		if started, ok := l.(observability.ExecutionStartedListener); ok {
			started.OnExecutionStarted(input)
		}
	}
}

// NotifyFinished fires OnExecutionFinished on every listener that
// implements observability.ExecutionFinishedListener.
func (n *Notifier) NotifyFinished(input, output interface{}, matched []string, duration time.Duration) {
	for _, l := range n.listeners {
		if finished, ok := l.(observability.ExecutionFinishedListener); ok {
			finished.OnExecutionFinished(input, output, matched, duration)
		}
	}
}

// NotifyErrored fires OnExecutionErrored on every listener that
// implements observability.ExecutionErroredListener.
func (n *Notifier) NotifyErrored(input interface{}, err error) {
	for _, l := range n.listeners {
		if errored, ok := l.(observability.ExecutionErroredListener); ok {
			errored.OnExecutionErrored(input, err)
		}
	}
}
