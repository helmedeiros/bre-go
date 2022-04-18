// Package firstmatch is an engine.Engine adapter that returns on the
// first matching rule. Use it for decision tables, content classifiers,
// and any policy where rule order encodes precedence.
package firstmatch

import (
	"context"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

// Rule is a named condition with an optional action.
// Description and Tags are optional metadata surfaced through
// engine.RuleInfoLister; they have no effect on Execute.
//
// Condition and Action are the narrow signatures; ConditionContext
// and ActionContext are context-aware variants. When the *Context
// variant is set, Execute prefers it.
type Rule struct {
	Name             string
	Description      string
	Tags             []string
	Condition        func(input interface{}) bool
	ConditionContext func(ctx context.Context, input interface{}) bool
	Action           func(input interface{}) interface{}
	ActionContext    func(ctx context.Context, input interface{}) interface{}
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
// ErrNilCondition when both Condition and ConditionContext are nil,
// or ErrDuplicateRuleName when r.Name is already registered. Checks
// run shape-first, state-second.
func (e *Engine) AddRule(r Rule) error {
	if r.Name == "" {
		return ErrEmptyRuleName
	}
	if r.Condition == nil && r.ConditionContext == nil {
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

// RuleInfos returns every registered rule's metadata in insertion
// order. Description and Tags reflect the values set on each Rule.
// The returned slice and each RuleInfo's Tags are fresh copies;
// mutating either does not affect engine state.
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

// Execute walks rules in insertion order and returns on the first one
// whose Condition is true. If that rule's Action panics, the panic is
// recovered and surfaced as an *ActionPanicError; the rule name still
// appears in Result.Matched (the rule did match -- its Action failed).
// If no rule matches, returns an empty Result.
//
// ctx governs cancellation; ctx.Err() is checked before the rule loop
// and between rules. A nil ctx is treated as context.Background().
func (e *Engine) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	e.notifyStarted(req.Input)

	for _, r := range e.rules {
		if err := ctx.Err(); err != nil {
			e.notifyErrored(req.Input, err)
			e.notifyFinished(req.Input, nil, nil, time.Since(start))
			return engine.Result{}, err
		}
		if !evaluateCondition(ctx, r, req.Input) {
			continue
		}
		out := engine.Result{Matched: []string{r.Name}}
		if hasAction(r) {
			output, panicErr := runAction(ctx, r, req.Input)
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

func evaluateCondition(ctx context.Context, r Rule, input interface{}) bool {
	if r.ConditionContext != nil {
		return r.ConditionContext(ctx, input)
	}
	// AddRule guarantees at least one of ConditionContext / Condition is set.
	return r.Condition(input)
}

func hasAction(r Rule) bool {
	return r.Action != nil || r.ActionContext != nil
}

func runAction(ctx context.Context, r Rule, input interface{}) (output interface{}, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = &ActionPanicError{Rule: r.Name, Value: rec}
		}
	}()
	if r.ActionContext != nil {
		output = r.ActionContext(ctx, input)
	} else {
		output = r.Action(input)
	}
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
