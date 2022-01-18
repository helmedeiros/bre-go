// Package engine declares the port every rule-engine adapter
// implements. Adapters live in sub-packages.
package engine

// Request is what a caller asks the Engine to evaluate.
type Request struct {
	Input interface{}
}

// Result is what the Engine produces. Empty fields mean no decision.
type Result struct {
	Output  interface{}
	Matched []string
}

// Engine evaluates rules against a Request.
type Engine interface {
	Execute(req Request) (Result, error)
}
