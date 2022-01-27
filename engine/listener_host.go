package engine

import "github.com/helmedeiros/bre-go/observability"

// ListenerHost is an optional interface adapters implement when they
// support attaching observability.ExecutionListener instances.
// Callers detect support with a type assertion.
type ListenerHost interface {
	AddListener(observability.ExecutionListener)
}
