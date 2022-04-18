// Package exec wraps an engine.Engine with a typed input/output
// surface that hides the interface{} cast at the boundary.
//
// Sits over the existing untyped engine.Engine port. Adapters do not
// change; one engine instance can still hold rules of mixed shapes
// because the typing lives in the wrapper, not in the port.
package exec

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
)

// Executor wraps an engine.Engine with typed input and output.
// In is the static type the wrapper hands to the underlying engine
// (via Request.Input). Out is the type Executor expects to recover
// from Result.Output via a type assertion.
type Executor[In, Out any] struct {
	inner engine.Engine
}

// New returns an Executor wrapping inner.
func New[In, Out any](inner engine.Engine) *Executor[In, Out] {
	return &Executor[In, Out]{inner: inner}
}

// Execute hands in to the underlying engine and casts the resulting
// Output to Out. Returns the zero value of Out when no rule produced
// an Output. Returns an *OutputTypeMismatchError when Output is set
// but not assignable to Out. ctx is propagated to the underlying
// engine.
func (e *Executor[In, Out]) Execute(ctx context.Context, in In) (Out, []string, error) {
	var zero Out
	res, err := e.inner.Execute(ctx, engine.Request{Input: in})
	if err != nil {
		return zero, res.Matched, err
	}
	if res.Output == nil {
		return zero, res.Matched, nil
	}
	out, ok := res.Output.(Out)
	if !ok {
		return zero, res.Matched, &OutputTypeMismatchError{
			Expected: fmt.Sprintf("%T", zero),
			Got:      fmt.Sprintf("%T", res.Output),
		}
	}
	return out, res.Matched, nil
}

// OutputTypeMismatchError is returned by Execute when an engine
// produces an Output that cannot be asserted to the Executor's Out
// type parameter.
type OutputTypeMismatchError struct {
	Expected string
	Got      string
}

// Error implements the error interface.
func (e *OutputTypeMismatchError) Error() string {
	return fmt.Sprintf("exec: output type mismatch: expected %s, got %s", e.Expected, e.Got)
}
