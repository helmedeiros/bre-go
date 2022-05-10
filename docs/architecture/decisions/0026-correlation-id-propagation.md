# 26. Correlation-ID Propagation Through Execute

## Status

Proposed — target release `v0.4.0`. Transitions to **Accepted** in the commit that lands the implementation. Closes the second half of Phase 2's observability extension.

## Context

`context.Context` has flowed through `Execute` since `v0.2.0` (ADR-0022). Callers can stuff request-scoped values into the context, and `ConditionContext` / `ActionContext` callbacks can read them. The Go convention for request-correlation IDs is exactly this: `context.WithValue(ctx, key, id)`, retrievable by anything downstream that knows the key.

What's missing: a **standard key** every caller uses. Today each application defines its own unexported context-key type for correlation IDs. Two apps can't share rule sets, predicates, or actions because they put the same logical value under different keys.

The library already publishes the observability vocabulary (`Match`, `Logger`, the lifecycle interfaces). Publishing a standard correlation-ID key is the same shape of decision: an opinion about how callers should structure cross-cutting concerns.

Three design questions:

**1. Where do the helpers live?**

- (a) `engine` package, since they manipulate the `context.Context` that flows through `engine.Engine.Execute`.
- (b) `observability` package, since correlation IDs are an observability concern.
- (c) New `engine/correlation` sub-package.

Pick (a). The helpers are 6 lines total. They wrap `context.Context` operations. They belong next to the `Engine` interface that consumes the context. `observability` would also work, but the audit (architectural review before this ADR) flagged that `engine` already imports `observability`, so adding `observability` import responsibilities to a wider portion of the engine surface is the wrong direction.

**2. What's the API shape?**

```go
package engine

func WithCorrelationID(ctx context.Context, id string) context.Context
func CorrelationIDFromContext(ctx context.Context) string
```

`WithCorrelationID` returns a derived context. `CorrelationIDFromContext` returns the stored ID or `""` if none is set. The key is an unexported `struct{}` type — callers cannot collide with the value or guess the key.

`""` as the "no ID set" return is deliberate. It's the natural Go zero value for `string`. Callers reading `if id := engine.CorrelationIDFromContext(ctx); id != "" { ... }` is the idiomatic pattern.

**3. How do listeners get the ID?**

Listeners today receive plain values (`Match`, `input interface{}`, etc.) — no `context.Context`. They can't read the ID directly.

Two approaches considered:

- **(a) Add new ctx-aware listener interfaces** (`RuleMatchedListenerCtx`, `ExecutionStartedListenerCtx`, ...). Adapters check via type assertion at notify time, paralleling the existing lifecycle role interfaces. Purely additive.
- **(b) Document the per-request listener pattern**: callers wanting per-request correlation construct a new listener that captures the per-request ID in its closure, attach it for the duration of the call. Less elegant but no new API surface.

**Pick (b) for `v0.4.0`.** The minimum useful thing is the ctx helpers themselves; `ConditionContext` and `ActionContext` callbacks already have ctx and can call `CorrelationIDFromContext`. Listener-side correlation is real but more invasive (8 new interfaces in observability) and deserves its own ADR with concrete caller feedback shaping it. Defer.

The cookbook gains a section showing the documented pattern: how to include the correlation ID in a structured log line from inside an `ActionContext`, and a sketch of the per-request-listener approach for cross-cutting cases.

## Decision

Add to the `engine` package:

```go
// WithCorrelationID returns ctx with id stored as the correlation
// identifier. Listeners and condition/action callbacks that have
// access to the context can recover it via CorrelationIDFromContext.
func WithCorrelationID(ctx context.Context, id string) context.Context

// CorrelationIDFromContext returns the correlation ID set on ctx via
// WithCorrelationID, or "" if none is set.
func CorrelationIDFromContext(ctx context.Context) string
```

Key type is unexported (`type correlationIDKey struct{}`), so callers cannot read or write the value without going through the helpers.

The Engine port itself is unchanged. Adapters do not need to know about correlation IDs; the context flows through unchanged, and any callback receiving ctx can read the ID.

A test file proves four behaviors with one assertion each: zero ID returns empty string from a fresh context, a set ID round-trips through Get/With, a nested WithCorrelationID call overwrites the outer ID, and ctx values set by other middleware on the same context don't collide with ours.

## Consequences

The `engine` port surface grows by two exported functions. No interface changes. No adapter changes.

The cookbook gains a "Propagate correlation IDs" section. The README Stability section adds `WithCorrelationID` and `CorrelationIDFromContext` as stable since `v0.4.0`.

Listener-side correlation (the ctx-aware listener interfaces in option (a) above) is deliberately deferred. When a future caller wants their `LoggingListener` to include the correlation ID in every log line, ADR-0027 introduces the ctx-aware listener interfaces — paralleling the existing `ExecutionStartedListener`, `ExecutionFinishedListener` shape — and `LoggingListener` becomes a `RuleMatchedListenerCtx` that calls `CorrelationIDFromContext` on the ctx it receives.

The two helpers shipped in this ADR are the minimum building blocks. Future patterns build on them.
