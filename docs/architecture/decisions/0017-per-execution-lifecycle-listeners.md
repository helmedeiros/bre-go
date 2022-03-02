# 17. Per-Execution Lifecycle Listeners

## Status

Accepted

## Context

ADR-0007 introduced `observability.ExecutionListener` with a single `OnRuleMatched(Match)` method and explicitly deferred per-execution lifecycle events:

> Per-execution lifecycle events (`Started`, `Finished`, `Errored`) are deliberately **not** part of the first cut. Callers that need them today can wrap the `Engine` with their own decorator. If we discover real callers needing the lifecycle events, a follow-up ADR adds them as separate listener interfaces -- many small interfaces over one fat one, per Clean Code.

Eight weeks of usage later, two needs are clear:

1. **Execution timing.** Today there is no hook to measure how long an `Execute` call takes. Every caller wanting latency metrics has to wrap the engine themselves -- and per ADR-0007, a decorator cannot observe per-rule matches, so the wrapper either duplicates the listener machinery or loses information.
2. **Audit and tracing.** Distributed tracing needs a "start" point to open a span and an "end" point to close it. Without lifecycle events the span has to be opened by the caller before `Execute` and closed after, which scatters the instrumentation across every call site.

ADR-0007 already pointed at the shape: **many small interfaces over one fat one**. Adding `OnExecutionStarted` and `OnExecutionFinished` directly to the existing `ExecutionListener` interface would be a breaking change -- every existing implementation (`NopExecutionListener`, `CountingListener`, `LoggingListener`, and any caller-defined listener) would need new methods.

The Go-idiomatic answer is two new role interfaces in `observability/`. Adapters call them via type assertion at notify time, exactly the way `engine.ListenerHost` and `engine.RuleLister` are discovered. A listener can implement none, one, or all three of the listener interfaces.

A naming concern: `observability` cannot import `engine` (the dependency goes the other way -- adapters import observability). The lifecycle methods therefore cannot carry `engine.Request` / `engine.Result` value types directly. They carry the underlying primitive data (`input interface{}`, `output interface{}`, `matched []string`, `duration time.Duration`), mirroring how `Match` already does it for `OnRuleMatched`.

## Decision

Add two new role interfaces to the `observability` package, alongside the existing `ExecutionListener`:

```go
type ExecutionStartedListener interface {
    OnExecutionStarted(input interface{})
}

type ExecutionFinishedListener interface {
    OnExecutionFinished(input interface{}, output interface{}, matched []string, duration time.Duration)
}
```

`AddListener` on each adapter keeps its existing signature -- it takes `observability.ExecutionListener`, since that is the most-general "I want match events" shape. At `Execute` time, the adapter:

1. Records `time.Now()`.
2. Walks the listener slice. For each listener, if it also implements `ExecutionStartedListener`, call `OnExecutionStarted(req.Input)`.
3. Runs the rules, calling `OnRuleMatched(...)` on every listener as before.
4. After the loop, walks the listener slice again. For each listener that also implements `ExecutionFinishedListener`, call `OnExecutionFinished(req.Input, res.Output, res.Matched, time.Since(start))`.

Adapters always satisfy `engine.ListenerHost`; the type-assertion lives *inside* the adapter, not on the port. The port stays minimal.

A first concrete consumer ships in the same week: `observability.TimingListener`. It implements all three interfaces (`OnRuleMatched` recording the last match, `OnExecutionFinished` recording the duration) so a caller can ask "how long did the last Execute take?" with one accessor.

`Errored` is deliberately not part of this cut. Today the engine's only error path is registration; `Execute` itself returns `(Result, nil)` for every adapter. When a future ADR adds error-producing actions (panic recovery, validation failures inside `Execute`), it brings `ExecutionErroredListener` with it. The shape will be `OnExecutionErrored(input interface{}, err error)`.

## Consequences

The observability port grows by two interfaces and one built-in. The engine port and both adapters get small, well-bounded changes: `Execute` adds a start-time capture and two type-asserting loops. `AddListener` stays the same; existing callers (and existing listener implementations) keep working unchanged.

The contract test suite gains a new case: if the adapter satisfies `engine.ListenerHost`, a listener that implements `ExecutionStartedListener` + `ExecutionFinishedListener` must receive both calls exactly once per `Execute`. Auto-skips when `ListenerHost` is not satisfied, matching the pattern from ADR-0010 and ADR-0016.

`TimingListener` becomes the first multi-interface built-in. The pattern -- one type implementing several listener roles -- is intentional: it shows callers how to compose lifecycle hooks without writing three separate wiring methods.
