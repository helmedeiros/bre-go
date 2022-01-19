# 7. Execution Listener As An Observer Port

## Status

Accepted

## Context

ADR-0001 listed "strong observability of every execution" as the second goal. The first observability piece (the `Logger` port in `observability`) handles unstructured asks: "log this". What it cannot answer is "tell me, programmatically, when a rule matched" -- metrics, traces, custom audit trails, dry-run reporters all need that signal at structured event granularity.

Established rule engine implementations expose a listener interface that adapters call at well-defined points during execution. Callers attach one or many listeners; logging and metrics are the most common first implementations.

Two design choices to weigh:

1. **Where the listener lives.** The decorator pattern (`engine.WithListener(inner, l) Engine`) is clean on paper but cannot observe per-rule matches -- the wrapper only sees the final `Result`. Per-rule observation has to happen inside each adapter.
2. **Listener shape.** A single method (`Observe(event Event)`) needs a sum-type union; Go does not have that natively. Multiple methods on one interface (`Started`, `Matched`, `Finished`) inflate the surface but keep each method small.

## Decision

Define an `ExecutionListener` interface in the `observability` package with one method:

```go
type Match struct {
    Rule   string
    Input  interface{}
    Output interface{}
}

type ExecutionListener interface {
    OnRuleMatched(m Match)
}
```

Adapters implement the firing themselves: after each rule match in their `Execute`, they call `OnRuleMatched` on every registered listener. The `inmemory.Engine` gains a single `AddListener` method that parallels `AddRule`.

Per-execution lifecycle events (`Started`, `Finished`, `Errored`) are deliberately **not** part of the first cut. Callers that need them today can wrap the `Engine` with their own decorator. If we discover real callers needing the lifecycle events, a follow-up ADR adds them as separate listener interfaces (`StartListener`, `ErrorListener`) -- many small interfaces over one fat one, per Clean Code.

## Consequences

The library's observation surface stays tiny: one value type and one interface. Logging and metrics ship as independent listener implementations either inside `observability/` or in caller code; neither pollutes the engine port.

Adapters that don't fire `OnRuleMatched` still satisfy the port -- listeners are opt-in. The decorator pattern remains available for callers who only want pre/post hooks around `Execute`.

The `Match` value type is intentionally identical in shape to what a single rule firing produces. If the underlying engine batches matches or reorders them, the adapter is the place that smooths it out into a per-rule stream the listener can consume.
