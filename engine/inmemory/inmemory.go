// Package inmemory provides a tiny engine.Engine implementation
// used by tests, examples, and any caller who wants rule execution
// without a third-party engine dependency.
//
// The package is intentionally small. Anything more sophisticated
// (compiled DSL, RETE-style matching, persistence) belongs in a
// separate adapter behind the same engine.Engine port.
package inmemory

import "github.com/helmedeiros/bre-go/engine"

// Rule describes a single rule held by an in-memory engine. The
// engine evaluates Condition against each Context.Input; when it
// returns true the rule's Name is appended to Result.Matched.
//
// A Rule with a nil Condition is treated as never matching --
// callers register such rules during construction and fill the
// Condition in before Execute. Naming is required; an empty Name
// is rejected at AddRule time so Matched output stays meaningful.
type Rule struct {
	Name      string
	Condition func(input interface{}) bool
}

// New constructs an empty engine. Rules are added with AddRule.
func New() *Engine {
	return &Engine{}
}

// Engine is an in-memory implementation of engine.Engine that
// holds rules in a slice and evaluates them in insertion order.
type Engine struct {
	rules []Rule
}

// AddRule registers a rule. Returns an error if the rule's Name
// is empty -- a nameless match would be invisible in Result.Matched.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return errEmptyRuleName
	}
	e.rules = append(e.rules, r)
	return nil
}

// Execute satisfies engine.Engine. It evaluates each rule's
// Condition against the Context.Input in insertion order. Every
// rule whose Condition returns true has its Name appended to
// Result.Matched. Output remains nil at this layer -- producing
// outputs is the job of rule Actions, added next.
func (e *Engine) Execute(ctx engine.Context) engine.Result {
	out := engine.Result{}
	for _, r := range e.rules {
		if r.Condition == nil {
			continue
		}
		if r.Condition(ctx.Input) {
			out.Matched = append(out.Matched, r.Name)
		}
	}
	return out
}
