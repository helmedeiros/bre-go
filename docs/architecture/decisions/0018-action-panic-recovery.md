# 18. Action Panic Recovery And The Errored Lifecycle Event

## Status

Accepted

## Context

Today, if a `Rule.Action` panics during `Execute`, the panic propagates up through the adapter to the caller. A single buggy action -- a nil-pointer dereference, an out-of-bounds index, a `panic("not implemented")` left in by mistake -- crashes the host goroutine, which in a long-lived process usually crashes the host *process*.

This is the wrong default for a library that hosts caller-defined code. Real BREs run actions in a recovery boundary so a misbehaving rule cannot take down the engine. ADR-0017 already named the shape of the missing piece:

> When a future ADR adds error-producing actions (panic recovery, validation failures inside `Execute`), it brings `ExecutionErroredListener` with it. The shape will be `OnExecutionErrored(input interface{}, err error)`.

Three design choices to resolve:

**1. What does `Execute` return on a panicking action?** Three options:

- (a) Stop on first panic. Return the partial `Result` (matched names up to that point) and a non-nil error. Remaining rules do not evaluate.
- (b) Recover and continue. Skip the panicking action's output but evaluate later rules. Surface the panic only via the listener.
- (c) Recover and continue, but also return a non-nil error from `Execute` (aggregating all panics).

Pick (a). Actions in a rule set can have order dependencies (rule N's action mutates state rule N+1 reads). Continuing past a known-broken action invites worse downstream failures. The explicit error return forces callers to decide rather than silently degrading.

**2. What does the error look like?** A typed error carrying the rule name lets callers branch on it without parsing strings. Define `ActionPanicError` locally in each adapter package -- `engine` cannot reference adapter-specific Rule types, and a shared neutral type adds dependency for marginal gain. Both adapter types implement the same interface (`error`), and both expose `RuleName()` so callers can introspect uniformly.

**3. Does the lifecycle event include the rule name?** ADR-0017 promised `OnExecutionErrored(input interface{}, err error)`. Refining: the error carries the rule name via the typed `ActionPanicError`. Callers who need it call `errors.As(err, &panicErr)` and read `panicErr.RuleName()`. No need to widen the interface signature.

The "first match's action panics on firstmatch" subtlety: `firstmatch.Execute` ordinarily returns after the first match. If that match's action panics, the matched name still appears in `Result.Matched` (because the rule *did* match -- the action failed afterwards). The error tells the caller "the matching rule was X, but its action failed." This matches how a downstream tracer would expect the data to look.

## Decision

Add `observability.ExecutionErroredListener`:

```go
type ExecutionErroredListener interface {
    OnExecutionErrored(input interface{}, err error)
}
```

In each adapter, wrap the `r.Action(req.Input)` call in a deferred-recover. If the action panics:

1. The recovery captures the panic value.
2. Construct an `ActionPanicError` carrying the rule name and the recovered value.
3. Call `OnExecutionErrored` on every listener that implements the interface.
4. Call `OnExecutionFinished` (so the lifecycle pair stays balanced -- every Started has a matching Finished, regardless of error).
5. Return `(out, &ActionPanicError{...})` where `out` is the partial Result.

`ActionPanicError` lives in each adapter package:

```go
// engine/inmemory/errors.go
type ActionPanicError struct {
    Rule  string
    Value interface{}
}
func (e *ActionPanicError) Error() string  { ... }
func (e *ActionPanicError) RuleName() string { return e.Rule }
```

Same shape in `engine/firstmatch`. The duplication is intentional -- both adapters need their own typed error, and pulling it into a shared package would couple every adapter to the same struct layout, which is the opposite of what the port pattern is for. Callers who want to handle both can use `errors.As` against the local type or define their own `RuleNamer` interface (one method, `RuleName() string`) and assert against that.

`Condition` panics are out of scope for this ADR. A `Condition` is a pure predicate; if it panics, that is a programmer error that surfaces during tests, not a defensive concern in production. Adding recovery there would mask real bugs.

The contract suite gains a tenth case: any adapter satisfying `engine.ListenerHost` must surface action panics through `OnExecutionErrored` and return a non-nil error from `Execute`. Auto-skips for adapters without `ListenerHost` support.

## Consequences

The library's safety story improves materially: one buggy rule can no longer crash a process running the engine. Existing tests continue to pass -- no current rule's action panics, so the recovery path is dormant for them.

`Execute`'s return signature is unchanged (`(Result, error)`) so no caller code breaks. The error from a panicking action is the first scenario where `Execute` returns non-nil error in normal operation; prior `error` returns were reserved for future use. Adapter implementations that have been treating the error return as "always nil" need to start checking it.

`OnExecutionFinished` always fires, even on the error path, so listeners using duration metrics can still record the elapsed time of a failed execution. The order on the error path is: `OnRuleMatched` (for any rules that matched before the panic) → `OnExecutionErrored` → `OnExecutionFinished`. The contract case pins this order.

`TimingListener.MatchesInLastExecution()` continues to reflect only successful matches -- the panicking rule did match its `Condition`, but its `Action` did not complete, so it does not increment the post-panic counter. (Implementation detail: `OnRuleMatched` fires *before* `r.Action(req.Input)` in both adapters today, which means the panicking rule's name *is* counted. The contract case pins this and tests verify both adapters agree.)

ADR-0017's reference to `OnExecutionErrored(input interface{}, err error)` is retroactively confirmed; the rule name is carried via the typed `ActionPanicError` rather than widening the interface signature.
