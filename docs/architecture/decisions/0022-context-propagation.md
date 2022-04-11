# 22. Propagate context.Context Through Execute

## Status

Proposed — target release `v0.2.0`. Transitions to **Accepted** in the commit that lands the implementation. This will be the first breaking change to the engine port since ADR-0003 declared it, and the first minor-version bump under ADR-0021's SemVer policy.

## Context

`engine.Engine.Execute` takes a single `engine.Request` argument today. The signature is:

```go
type Engine interface {
    Execute(req Request) (Result, error)
}
```

Three real-world needs are missing:

1. **Timeouts.** Rule sets loaded from external data sources may grow over time; a runaway evaluation should be cancellable from the caller. Today the only escape is goroutine-level `runtime.Goexit` or `time.AfterFunc`, both ugly.
2. **Request-scoped cancellation.** A caller serving HTTP requests wants to short-circuit `Execute` when the client disconnects. Without `context.Context`, the rule loop runs to completion regardless.
3. **Trace propagation.** Distributed tracing requires the span context to flow into work units. Conditions and Actions that themselves make network calls (a future DB-backed condition, a remote rule lookup) need to receive the request's context so they can attach to the parent span.

The Go-idiomatic answer is `context.Context` as the first parameter of any method that may block, perform I/O, or be cancelled. The standard library has held this convention since Go 1.7. Every Go HTTP handler, every database driver, every gRPC client follows it.

Three design choices:

**1. Where does `ctx` go?** Two shapes:

- (a) **First parameter on the method**: `Execute(ctx context.Context, req Request) (Result, error)`. Standard Go convention. Breaks every existing caller of `Execute`.
- (b) **Field on `Request`**: `Request{Ctx context.Context, Input interface{}}`. Non-breaking-ish (existing `Request{Input: x}` still compiles; `Ctx` defaults to nil). Departs from the Go convention.

Pick (a). The Go community treats `ctx` as a method parameter, not data on a struct. Linters (`contextcheck`, `noctx`) flag the struct-field pattern. Callers reading `eng.Execute(ctx, req)` immediately understand the cancellation contract; `eng.Execute(req)` with `req.Ctx` hides it.

**2. What does a nil ctx do?** Two options:

- (a) **Panic on nil**: matches `http.NewRequestWithContext` behavior.
- (b) **Treat nil as `context.Background()`**: convenience for tests.

Pick (b). Tests in this repo and downstream often pass `context.Background()` or `nil` interchangeably. The cost of one nil-check is trivial. The benefit -- test code stays terse -- is real. Document the behavior in the godoc.

**3. How do `Condition` and `Action` get the context?** The condition/action signatures are `func(input interface{}) bool` and `func(input interface{}) interface{}`. They have no `ctx`. Two options:

- (a) **Widen them**: `func(ctx context.Context, input interface{}) bool` and same for Action. Breaking change to every existing rule. Deep ripple.
- (b) **Keep the existing narrow signatures, add new optional `ConditionContext`/`ActionContext` fields**: `Rule.ConditionContext func(ctx context.Context, input interface{}) bool`. When set, the adapter prefers it. When not, the adapter calls the legacy `Condition`. Both compile-time-compatible with v0.1.0 callers.

Pick (b) for v0.2.0. Rule authors who need ctx in their predicates opt in by setting the `*Context` fields. Rule authors who don't need ctx keep the narrow signatures. This keeps the v0.2.0 migration mostly source-compatible: most callers only update their `Execute(req)` call to `Execute(ctx, req)`.

A later ADR (probably v0.3.0 or v0.4.0) can revisit unifying the signatures once the typed Executor layer (ADR-0013) has a clear path to thread context through to typed condition/action funcs as well.

## Decision

Land in `v0.2.0`. The engine port becomes:

```go
type Engine interface {
    Execute(ctx context.Context, req Request) (Result, error)
}
```

Each adapter's `Execute`:

1. If `ctx == nil`, treats as `context.Background()`.
2. Before the rule loop and between rules, checks `ctx.Err()`. On cancellation, calls `OnExecutionErrored` with the cancellation error and returns `(partialResult, ctx.Err())`.
3. For each rule, if `Rule.ConditionContext != nil`, calls it with `ctx`; otherwise falls back to `Rule.Condition`.
4. Same for `ActionContext` vs `Action`.

`Rule` structs gain two optional fields on every adapter (`inmemory`, `firstmatch`, `priority`):

```go
ConditionContext func(ctx context.Context, input interface{}) bool
ActionContext    func(ctx context.Context, input interface{}) interface{}
```

`AddRule` validation accepts a rule that sets either `Condition` or `ConditionContext` (but rejects when both are nil with `ErrNilCondition`). Same for Action (Action is already optional; ActionContext is also optional; setting both is allowed but the `*Context` variant wins).

The generic `exec.Executor[In, Out]` gains `ctx` as the first parameter to `Execute`:

```go
func (e *Executor[In, Out]) Execute(ctx context.Context, in In) (Out, []string, error)
```

`enginetest.RunContractTests` gains a twelfth case asserting that an adapter respects `ctx.Done()` -- a cancelled context produces a non-nil error from `Execute` even when rules would otherwise match.

## Consequences

Every caller of `Execute` updates their call sites. The breaking-change cost is paid once at v0.2.0; the resulting API matches Go convention forever. The `*Context` field pattern means most existing `Rule` definitions compile unchanged.

The contract suite, the polymorphic tests, the runnable examples, the README Quickstart, and the `check-quickstart.sh` expected output all update in the same minor bump. This is the kind of change ADR-0021 explicitly anticipated when it set the "minor = breaking" policy.

A cancelled context produces `OnExecutionErrored` and a non-nil `Execute` return -- listeners observing duration metrics still see `OnExecutionFinished` (the lifecycle pair stays balanced even on cancellation). This keeps the listener ordering contract from ADR-0017 + ADR-0018 intact.

`Condition`/`Action` widening (collapsing the narrow + Context variants back into one signature) waits for a later ADR. The rationale to defer: rule authors writing condition logic against `interface{}` rarely need ctx; rule authors making network calls almost always do. Letting both coexist for one minor cycle gives feedback on which pattern is more common.
