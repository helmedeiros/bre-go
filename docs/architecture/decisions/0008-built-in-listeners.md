# 8. Built-In Listener Implementations

## Status

Accepted

## Context

ADR-0007 defined `observability.ExecutionListener` and shipped `NopExecutionListener`. That is the right minimum for testing the port -- it proves the interface is implementable -- but it does not give a caller anything useful to attach. A real consumer wants at least two things on day one:

1. A way to count what fired, so dashboards and tests can assert on rule-level hit counts.
2. A way to log every match through the `Logger` port the library already exposes, so structured logs and rule-execution events flow through the same destination.

Without these in the library, every caller hand-rolls them. They diverge, naming drifts, and the contract test suite never gets to exercise the listener wiring against more than the nop.

A second design pressure: the `Logger` and `ExecutionListener` ports were designed independently in ADR-0007 to keep the surface minimal. But the two clearly compose -- "log every rule match" is the obvious bridge between them. The bridge belongs *in the library*, not in caller code, because (a) we want the contract tests to use it and (b) every adapter we ship should be reachable from `go doc` rather than buried in a downstream repo.

## Decision

Ship two listeners in the `observability` package:

```go
type CountingListener struct { /* per-rule counters, safe for one execution at a time */ }
func (c *CountingListener) OnRuleMatched(m Match)
func (c *CountingListener) Count(rule string) int
func (c *CountingListener) Total() int

type LoggingListener struct { /* wraps a Logger */ }
func NewLoggingListener(l Logger) *LoggingListener
func (l *LoggingListener) OnRuleMatched(m Match)
```

Both live next to `NopExecutionListener` in `observability/listener.go` (small enough not to warrant a new file each). `CountingListener` keeps counts in a map keyed by rule name; the zero value is usable. `LoggingListener` calls `Info` on its wrapped `Logger` with the rule name as a structured field; it never logs the `Input` or `Output` payloads, because they may carry PII the caller has not opted in to logging.

Concurrency: both listeners are documented as safe for *one execution at a time*. Concurrent `Execute` calls on the same engine instance are out of scope for the inmemory adapter today; when a future adapter changes that, we either add a mutex inside the listener or ship a separate `SyncCountingListener`. Premature locking right now would add overhead nobody needs.

## Consequences

The `observability` package grows by two small types but the *port surface* does not -- both are pure consumers of the existing `ExecutionListener` interface. Callers wire them with `engine.AddListener(...)`, no other API change.

`CountingListener` becomes the natural witness in tests that previously hand-rolled their own recording listener. Cleanup of those is a follow-up, not a prerequisite -- the recording listener in `engine/inmemory` stays as long as it remains the simplest thing that tests the *wiring* itself.

The decision to keep payloads out of `LoggingListener`'s log line is deliberate. Callers who do want them attach a custom listener; the built-in one stays safe-by-default.
