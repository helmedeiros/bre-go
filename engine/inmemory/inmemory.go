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
// returns true the rule's Name is appended to Result.Matched and
// the rule's Action (if any) is invoked to produce Result.Output.
//
// A Rule with a nil Condition is treated as never matching. A Rule
// with a non-nil Condition but a nil Action contributes to Matched
// and leaves Output untouched -- useful for rules that only
// classify the input. Naming is required; an empty Name is
// rejected at AddRule time so Matched output stays meaningful.
//
// When several matching rules carry an Action, later rules
// overwrite the Output of earlier ones (insertion order); the
// final Output is whichever Action ran last.
type Rule struct {
	Name      string
	Condition func(input interface{}) bool
	Action    func(input interface{}) interface{}
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
// Condition against the Request.Input in insertion order. Every
// rule whose Condition returns true has its Name appended to
// Result.Matched and its Action (if any) is invoked to set
// Result.Output.
//
// The in-memory engine has no failure modes today, so the error
// return is always nil. The signature exists so the port stays
// uniform across adapters (see ADR-0005).
func (e *Engine) Execute(req engine.Request) (engine.Result, error) {
	out := engine.Result{}
	for _, r := range e.rules {
		if r.Condition == nil {
			continue
		}
		if r.Condition(req.Input) {
			out.Matched = append(out.Matched, r.Name)
			if r.Action != nil {
				out.Output = r.Action(req.Input)
			}
		}
	}
	return out, nil
}
