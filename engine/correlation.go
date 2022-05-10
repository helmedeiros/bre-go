package engine

import "context"

type correlationIDKey struct{}

// WithCorrelationID returns a copy of ctx carrying id as the
// correlation identifier. Code holding the derived context can recover
// the id via CorrelationIDFromContext. The key used internally is an
// unexported type, so callers cannot read or overwrite the value
// without going through these helpers.
//
// Typical use: a request handler stamps the inbound request ID onto
// the context before calling Execute. ConditionContext / ActionContext
// callbacks running inside Execute read the id back when emitting
// structured logs or external calls.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, id)
}

// CorrelationIDFromContext returns the correlation id set on ctx via
// WithCorrelationID, or "" if none is set.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return id
	}
	return ""
}
