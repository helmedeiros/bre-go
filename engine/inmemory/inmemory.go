// Package inmemory is a tiny engine.Engine for tests and examples.
package inmemory

import (
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

// Rule is a named condition with an optional action.
// Description and Tags are optional metadata surfaced through
// engine.RuleInfoLister; they have no effect on Execute.
type Rule struct {
	Name        string
	Description string
	Tags        []string
	Condition   func(input interface{}) bool
	Action      func(input interface{}) interface{}
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

// AddRule registers r. Returns ErrEmptyRuleName when r.Name is empty,
// ErrNilCondition when r.Condition is nil, or ErrDuplicateRuleName when
// r.Name is already registered. Checks run shape-first (per-rule
// invariants) then engine-state-second (uniqueness).
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
// order. The returned slice is a fresh copy and safe to modify.
func (e *Engine) RuleNames() []string {
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name
	}
	return names
}

// RuleInfos returns every registered rule's metadata in insertion
// order. Description and Tags reflect the values set on each Rule;
// callers that did not set them get empty defaults. The returned
// slice and each RuleInfo's Tags are fresh copies; mutating either
// does not affect engine state.
func (e *Engine) RuleInfos() []engine.RuleInfo {
	infos := make([]engine.RuleInfo, len(e.rules))
	for i, r := range e.rules {
		infos[i] = engine.RuleInfo{
			Name:        r.Name,
			Description: r.Description,
			Tags:        copyTags(r.Tags),
		}
	}
	return infos
}

func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// Execute walks every rule and returns the result. Later matching
// actions overwrite earlier outputs. If a rule's Action panics, the
// panic is recovered and surfaced as an *ActionPanicError; remaining
// rules do not evaluate.
func (e *Engine) Execute(req engine.Request) (engine.Result, error) {
	start := time.Now()
	e.notifyStarted(req.Input)

	out := engine.Result{}
	var actionErr error
	for _, r := range e.rules {
		if r.Condition == nil || !r.Condition(req.Input) {
			continue
		}
		out.Matched = append(out.Matched, r.Name)
		if r.Action != nil {
			output, panicErr := runAction(r.Name, r.Action, req.Input)
			if panicErr != nil {
				actionErr = panicErr
				e.notifyErrored(req.Input, panicErr)
				break
			}
			out.Output = output
		}
		e.notify(r.Name, req.Input, out.Output)
	}

	e.notifyFinished(req.Input, out.Output, out.Matched, time.Since(start))
	return out, actionErr
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
