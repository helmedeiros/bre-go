// Package firstmatch is an engine.Engine adapter that returns on the
// first matching rule. Use it for decision tables, content classifiers,
// and any policy where rule order encodes precedence.
package firstmatch

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

// Engine evaluates rules in insertion order and returns on the first
// matching rule.
type Engine struct {
	rules     []Rule
	listeners []observability.ExecutionListener
}

// AddRule registers r. Returns ErrEmptyRuleName when r.Name is empty,
// ErrNilCondition when r.Condition is nil, or ErrDuplicateRuleName when
// r.Name is already registered. Checks run shape-first, state-second.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	if r.Condition == nil {
		return ErrNilCondition
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

// Execute walks rules in insertion order and returns on the first one
// whose Condition is true. If no rule matches, returns an empty Result.
func (e *Engine) Execute(req engine.Request) (engine.Result, error) {
	for _, r := range e.rules {
		if !r.Condition(req.Input) {
			continue
		}
		out := engine.Result{Matched: []string{r.Name}}
		if r.Action != nil {
			out.Output = r.Action(req.Input)
		}
		e.notify(r.Name, req.Input, out.Output)
		return out, nil
	}
	return engine.Result{}, nil
}

func (e *Engine) notify(rule string, input, output interface{}) {
	m := observability.Match{Rule: rule, Input: input, Output: output}
	for _, l := range e.listeners {
		l.OnRuleMatched(m)
	}
}
