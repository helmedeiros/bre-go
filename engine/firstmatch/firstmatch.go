// Package firstmatch is an engine.Engine adapter that returns on the
// first matching rule. Use it for decision tables, content classifiers,
// and any policy where rule order encodes precedence.
package firstmatch

import (
	"time"

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

// RuleNames returns the names of every registered rule in insertion
// order, which is the precedence chain Execute walks. The returned
// slice is a fresh copy and safe to modify.
func (e *Engine) RuleNames() []string {
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name
	}
	return names
}

// Execute walks rules in insertion order and returns on the first one
// whose Condition is true. If that rule's Action panics, the panic is
// recovered and surfaced as an *ActionPanicError; the rule name still
// appears in Result.Matched (the rule did match -- its Action failed).
// If no rule matches, returns an empty Result.
func (e *Engine) Execute(req engine.Request) (engine.Result, error) {
	start := time.Now()
	e.notifyStarted(req.Input)

	for _, r := range e.rules {
		if !r.Condition(req.Input) {
			continue
		}
		out := engine.Result{Matched: []string{r.Name}}
		if r.Action != nil {
			output, panicErr := runAction(r.Name, r.Action, req.Input)
			if panicErr != nil {
				e.notifyErrored(req.Input, panicErr)
				e.notifyFinished(req.Input, out.Output, out.Matched, time.Since(start))
				return out, panicErr
			}
			out.Output = output
		}
		e.notify(r.Name, req.Input, out.Output)
		e.notifyFinished(req.Input, out.Output, out.Matched, time.Since(start))
		return out, nil
	}

	e.notifyFinished(req.Input, nil, nil, time.Since(start))
	return engine.Result{}, nil
}

func runAction(name string, action func(interface{}) interface{}, input interface{}) (output interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &ActionPanicError{Rule: name, Value: r}
		}
	}()
	output = action(input)
	return output, nil
}

func (e *Engine) notify(rule string, input, output interface{}) {
	m := observability.Match{Rule: rule, Input: input, Output: output}
	for _, l := range e.listeners {
		l.OnRuleMatched(m)
	}
}

func (e *Engine) notifyStarted(input interface{}) {
	for _, l := range e.listeners {
		if started, ok := l.(observability.ExecutionStartedListener); ok {
			started.OnExecutionStarted(input)
		}
	}
}

func (e *Engine) notifyFinished(input, output interface{}, matched []string, duration time.Duration) {
	for _, l := range e.listeners {
		if finished, ok := l.(observability.ExecutionFinishedListener); ok {
			finished.OnExecutionFinished(input, output, matched, duration)
		}
	}
}

func (e *Engine) notifyErrored(input interface{}, err error) {
	for _, l := range e.listeners {
		if errored, ok := l.(observability.ExecutionErroredListener); ok {
			errored.OnExecutionErrored(input, err)
		}
	}
}
