// Package inmemory is a tiny engine.Engine for tests and examples.
package inmemory

import "github.com/helmedeiros/bre-go/engine"

// Rule is a named condition with an optional action.
type Rule struct {
	Name      string
	Condition func(input interface{}) bool
	Action    func(input interface{}) interface{}
}

// New returns an empty engine.
func New() *Engine {
	return &Engine{}
}

// Engine evaluates rules in insertion order.
type Engine struct {
	rules []Rule
}

// AddRule registers r. Returns ErrEmptyRuleName when r.Name is empty.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	e.rules = append(e.rules, r)
	return nil
}

// Execute walks every rule and returns the result. Later matching
// actions overwrite earlier outputs.
func (e *Engine) Execute(req engine.Request) (engine.Result, error) {
	out := engine.Result{}
	for _, r := range e.rules {
		if r.Condition == nil || !r.Condition(req.Input) {
			continue
		}
		out.Matched = append(out.Matched, r.Name)
		if r.Action != nil {
			out.Output = r.Action(req.Input)
		}
	}
	return out, nil
}
