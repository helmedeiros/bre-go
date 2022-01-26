// Package inmemory is a tiny engine.Engine for tests and examples.
package inmemory

import (
	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

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
	rules     []Rule
	listeners []observability.ExecutionListener
}

// AddRule registers r. Returns ErrEmptyRuleName when r.Name is empty
// or ErrDuplicateRuleName when r.Name is already registered.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	for _, existing := range e.rules {
		if existing.Name == r.Name {
			return ErrDuplicateRuleName
		}
	}
	e.rules = append(e.rules, r)
	return nil
}

// AddListener registers l for OnRuleMatched events.
func (e *Engine) AddListener(l observability.ExecutionListener) {
	e.listeners = append(e.listeners, l)
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
		e.notify(r.Name, req.Input, out.Output)
	}
	return out, nil
}

func (e *Engine) notify(rule string, input, output interface{}) {
	m := observability.Match{Rule: rule, Input: input, Output: output}
	for _, l := range e.listeners {
		l.OnRuleMatched(m)
	}
}
