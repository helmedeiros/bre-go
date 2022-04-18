// Package engine declares the port every rule-engine adapter
// implements. Adapters live in sub-packages.
package engine

import "context"

// Request is what a caller asks the Engine to evaluate.
type Request struct {
	Input interface{}
}

// Result is what the Engine produces. Empty fields mean no decision.
type Result struct {
	Output  interface{}
	Matched []string
}

// Engine evaluates rules against a Request. The context governs
// cancellation and deadlines; adapters check ctx.Err() between rules
// and return early with the cancellation error if the context is
// canceled mid-execution. A nil ctx is treated as context.Background().
type Engine interface {
	Execute(ctx context.Context, req Request) (Result, error)
}
