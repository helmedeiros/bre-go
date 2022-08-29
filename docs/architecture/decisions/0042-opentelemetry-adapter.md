# 42. OpenTelemetry Adapter for `engine.Engine`

## Status

Accepted — landed in v0.17.0. Adds `observability/otel.Wrap(inner engine.Engine, tracer trace.Tracer, opts ...Option) engine.Engine` as a decorator that emits one OpenTelemetry span per `Execute` call. Compatible with every existing adapter (inmemory, firstmatch, priority, indexed) through the `engine.Engine` port. Optional capability interfaces (`RuleLister`, `RuleInfoLister`, `ListenerHost`) forward to the inner engine when present. Lives in `observability/otel/` inside the main bre-go module; depends on `go.opentelemetry.io/otel` v1.11.x (the contemporary release compatible with Go 1.18). The pre-tag scientific review at [`scientific/v0.17.0/REPORT.md`](../../../scientific/v0.17.0/REPORT.md) audited 11 production-mirroring scenarios and forced one design change: cancellation is **not** marked as `codes.Error` — it uses dedicated `rule.engine.canceled` + `rule.engine.cancel.reason` attributes instead.

## Context

bre-go ships an observability surface from v0.5.0 (basic listeners) through v0.13.0 (`observability.StructuredTelemetryListener`) — typed `TelemetryRecord`s emitted per `Execute`. That model works for log-pipeline / metrics-counter consumers who don't care about distributed-tracing context propagation. It does not work for consumers who want their rule executions to appear as spans within the parent request's distributed trace.

The OpenTelemetry SDK is the de-facto standard for that use case. A bre-go user running a service with OTel tracing already wired up wants `Execute` calls to produce spans automatically: each span parented to the current request span, tagged with the matched rule names, marked with error status on failure, propagating correlation IDs as attributes.

The existing `Listener` interfaces (`ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`) are not enough: they receive `input`, `output`, `matched`, `duration`, `err` but **not** the `context.Context`. OTel spans depend on context for parent-span lookup. A listener-based design would either need to:

- Extend every listener method with a `ctx` variant (breaks API surface, requires every adapter to pass `ctx` through `Notifier.NotifyXxxCtx` plumbing), or
- Store per-invocation context out-of-band (fragile — input pointers aren't reliable keys under concurrent execution).

Both are unattractive. The cleanest design is a **decorator** on `engine.Engine` itself: a Tracer type that wraps an inner engine, implements the same interface, and adds tracing around each `Execute`. Context is already a first-class parameter of `engine.Execute(ctx, req)`, so the decorator gets it for free.

Three design questions.

### 1. Decorator vs Listener?

Two implementation models:

- **(a) Listener.** Implements `ExecutionStartedListener` + `ExecutionFinishedListener` + `ExecutionErroredListener`. Attached via `Engine.AddListener`. Needs context plumbing that the current Notifier doesn't provide.
- **(b) Decorator.** Wraps an `engine.Engine` and intercepts `Execute(ctx, req)`. Doesn't need any change to the listener model.

Pick **(b)**. The decorator pattern is idiomatic Go, requires no internal API changes, composes cleanly with `engine/exec.Executor[In, Out]` (which is also a wrapper), and lets the OTel adapter live entirely outside `engine/`. The listener-extension approach would touch every adapter's `Execute` method and the `internal/adapter.Notifier` plumbing — disproportionate cost for one feature.

### 2. Span model: one span per Execute, or per matched rule?

Three options:

- **(a) One Execute span.** Whole rule execution is one span. Matched rule names as a string-slice attribute.
- **(b) Per-matched-rule child spans.** Each matched rule gets its own child span under the Execute span.
- **(c) One Execute span + events per matched rule.** OTel events are lightweight: timestamps + attributes attached to a span without creating a child.

Pick **(a)**. Rule matches are typically sub-microsecond; per-match child spans would (1) clutter the trace UI for callers that match many rules and (2) cost more in OTel overhead than the matches themselves. Putting matched rule names as a span attribute gives operators what they need (which rules fired) without the trace-explosion cost. **(c)** is a reasonable compromise but adds wire weight without commensurate value; defer until a real consumer asks for per-match events.

### 3. What goes on the span?

Attributes:

- `rule.engine.adapter` (string): the inner engine's type name (e.g., `"indexed.Engine"`). Lets operators slice traces by adapter.
- `rule.engine.matched.count` (int): how many rules matched.
- `rule.engine.matched.names` (string slice): the matched rule names in order.
- `rule.engine.correlation_id` (string, optional): `engine.CorrelationIDFromContext(ctx)` if set. Connects the span to the request's correlation ID for log/trace cross-reference.

Span name: `rule.engine.execute` by default; configurable via `WithSpanName(name)` option for callers that want a more specific name (e.g., `"pricing.rules.execute"`).

On error: span gets `codes.Error` status with the error message; `span.RecordError(err)` records the typed error (including `*ActionPanicError`, `*FanoutTooLargeError`, etc. — OTel will capture their concrete types).

## Decision

Add a new sub-package `observability/otel/` inside the main bre-go module. Top-level API:

```go
package otel

// Wrap returns inner instrumented with OTel spans around every
// Execute. The returned value satisfies engine.Engine; optional
// capability interfaces forward to inner.
func Wrap(inner engine.Engine, tracer trace.Tracer, opts ...Option) engine.Engine

type Option func(*tracedEngine)

// WithSpanName overrides the default "rule.engine.execute" span name.
func WithSpanName(name string) Option
```

Attribute keys are exported as constants for callers that want to filter / aggregate on them in their backend:

```go
const (
    AttrAdapter       = "rule.engine.adapter"
    AttrMatchedCount  = "rule.engine.matched.count"
    AttrMatchedNames  = "rule.engine.matched.names"
    AttrCorrelationID = "rule.engine.correlation_id"

    // Cancellation: signal caller intent without polluting error-rate dashboards.
    AttrCanceled     = "rule.engine.canceled"
    AttrCancelReason = "rule.engine.cancel.reason"
)
```

### Cancellation is not an error

`context.Canceled` and `context.DeadlineExceeded` are caller intent (upstream timeout, graceful shutdown, etc.) — not engine failures. The pre-tag scientific review at [`scientific/v0.17.0/REPORT.md`](../../../scientific/v0.17.0/REPORT.md) §5 surfaced that marking these as `codes.Error` would inflate error-rate dashboards with what are really expected outcomes.

The adapter handles them on a dedicated branch:

- Sets `AttrCanceled = true` and `AttrCancelReason = "canceled"` / `"deadline_exceeded"`.
- Leaves `Status` as `Unset` (success-by-default in OTel semantics).
- Does NOT call `span.RecordError` — cancellation is not an exception.

Operators get a one-attribute filter (`canceled = true`) for "show me canceled executions" without those entries counting as failures. The Go return value is unchanged — the caller still receives the original `context.Canceled` / `context.DeadlineExceeded` sentinel.

### Dependency footprint

The OTel adapter lives in `observability/otel/` inside the main bre-go module. We pin `go.opentelemetry.io/otel` to v1.11.x — the contemporary release compatible with Go 1.18 — so the main module's go directive stays at `go 1.18` and consumers don't need to upgrade Go to pull in OTel-aware bre-go.

API packages added to `go.mod`:

- `go.opentelemetry.io/otel` v1.11.2 (API root)
- `go.opentelemetry.io/otel/trace` v1.11.2 (Tracer / Span / SpanContext)
- `go.opentelemetry.io/otel/sdk` v1.11.2 (SDK; pulled in transitively, only needed for tests)
- transitive: `go-logr/logr`, `go-logr/stdr`, `golang.org/x/sys`

Tests use `otel/sdk/trace` + `otel/sdk/trace/tracetest` for the in-memory `SpanRecorder` and `otel/trace.NewNoopTracerProvider()` for the no-op-tracer test. The SDK packages are imported only from `_test.go` files; production binaries pull only the API tree.

Consumers who don't want any OTel deps in their build graph can simply not import `github.com/helmedeiros/bre-go/observability/otel`. Go's build system handles transitive pruning correctly — only packages actually imported make it into the consumer's binary.

### Capability-forwarding contract

The decorator's `Wrap` returns `engine.Engine`. Optional capability interfaces:

```go
// Forwarded to inner if inner implements them:
type RuleLister      interface { RuleNames() []string }
type RuleInfoLister  interface { RuleInfos() []engine.RuleInfo }
type ListenerHost    interface { AddListener(observability.ExecutionListener) }
```

The wrapper struct embeds the inner via `engine.Engine`, so methods on `engine.Engine` are promoted by default. For non-Engine capabilities (`RuleNames`, `RuleInfos`, `AddListener`), the wrapper does explicit type-assertion forwarding in its own methods so callers can use the standard idiom:

```go
traced := otel.Wrap(inner, tracer)
if lister, ok := traced.(indexed.RuleLister); ok {
    names := lister.RuleNames()
}
```

The wrapper's struct exposes these methods only if they're present on `inner`. (Go has no compile-time way to express "I implement X iff my inner does"; the practical idiom is to expose them unconditionally and panic / no-op when inner doesn't support them, OR to expose them via an explicit `Unwrap()` method.)

Pick: **explicit methods on the wrapper** that delegate when inner supports them, return zero-value or panic when it doesn't. Document the contract.

### Trace propagation

The decorator does not propagate trace context across process boundaries — that's the consumer's job (OTel propagators, `otelhttp.Transport`, etc.). The decorator only **continues** an existing trace if one is on the ctx, or starts a new root span if not. Standard OTel behavior; no special handling.

### Sampling

The decorator does not influence sampling — that's the OTel SDK's job, configured at the application level. Every `Execute` calls `tracer.Start(ctx, name)`; the SDK decides whether to actually emit a span based on the configured sampler.

### Testing strategy

Tests use OTel's in-memory `tracetest.SpanRecorder` to capture spans without needing a real OTel collector. Cover:

- One span per `Execute`.
- Span name is `"rule.engine.execute"` by default; `WithSpanName` overrides correctly.
- `AttrAdapter` carries the inner type name (e.g., `"*indexed.Engine"`).
- `AttrMatchedCount` + `AttrMatchedNames` carry the matched rule names in order.
- `AttrCorrelationID` carries the value from `engine.WithCorrelationID(ctx, id)` when set; absent when not set.
- On error: span has `codes.Error` status and the recorded error.
- Parent span propagation: when called from within an existing span, the new span is a child.
- Forwarding: wrapped `*indexed.Engine` keeps `RuleNames()` / `RuleInfos()` / `AddListener` working.
- Compose with `engine/exec.Executor`: `Executor.Execute(ctx, in)` through a Wrap'd engine produces a single Execute span (no double-wrapping artifact).

### Cookbook section

A new "Trace Execute with OpenTelemetry" entry:

- Show the simplest setup: `otelapi.Tracer("my-service") → otel.Wrap(engine, tracer) → execute`.
- Show the `WithSpanName` option for callers using bre-go in multiple distinct rule contexts.
- Show the correlation-ID flow: `ctx = engine.WithCorrelationID(ctx, "req-123")` → span automatically tagged.
- Note that the listener-based `StructuredTelemetryListener` is the right tool for non-OTel consumers; OTel decorator and structured listener can coexist on the same inner engine.

## Consequences

### Closed by v0.17.0

- bre-go integrates with the standard observability ecosystem. Operators running OTel-instrumented services can now see rule executions as first-class spans, with the same trace propagation, sampling, and backend export that the rest of their service uses.
- The decorator pattern proven here is the model for any future `engine.Engine` instrumentation (e.g., metrics, structured-logging adapters).

### NOT closed by v0.17.0

- **Per-matched-rule span events** (option (c) above). Deferred until a consumer asks for per-match observability.
- **Metrics adapter.** Same `Wrap` decorator pattern with OTel metrics (counters, histograms for matched_count + duration). Probably a v0.18.0 ADR.
- **Custom attribute extraction.** Today the decorator emits a fixed attribute set. Future ADR could add `WithAttributeExtractor(func(ctx, req, res) []attribute.KeyValue)` for callers who want custom slices (e.g., `customer.id`, `tenant.id`).

### Performance impact

- Per-Execute overhead: one `tracer.Start` + one `span.End` + `AttrMatchedNames` slice copy. ~100ns–1μs depending on the SDK's sampler decision.
- When sampling drops the span: `tracer.Start` returns a no-op span; the cost is two function calls. Negligible.
- The inner engine's Execute is called unchanged.

### Validation strategy

- Unit tests with `tracetest.SpanRecorder` against every concrete adapter (indexed, firstmatch, priority, inmemory) to confirm cross-adapter behavior.
- A "trace propagation" test that starts a parent span manually, calls Wrap'd Execute, asserts the resulting span's parent matches.
- A "no-op tracer" test: passing `noop.NewTracerProvider().Tracer("")` produces no observable side effects (graceful degradation).
