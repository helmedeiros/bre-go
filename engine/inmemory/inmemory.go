// Package inmemory provides a tiny engine.Engine implementation
// used by tests, examples, and any caller who wants rule execution
// without a third-party engine dependency.
//
// The package is intentionally small. Anything more sophisticated
// (compiled DSL, RETE-style matching, persistence) belongs in a
// separate adapter behind the same engine.Engine port.
package inmemory

import "github.com/helmedeiros/bre-go/engine"

// New constructs an empty engine. Rules are added with AddRule.
func New() *Engine {
	return &Engine{}
}

// Engine is an in-memory implementation of engine.Engine that
// holds rules in a slice and evaluates them in insertion order.
type Engine struct{}

// Execute satisfies engine.Engine. The empty engine has no rules
// to evaluate, so the result is empty.
func (*Engine) Execute(_ engine.Context) engine.Result {
	return engine.Result{}
}
