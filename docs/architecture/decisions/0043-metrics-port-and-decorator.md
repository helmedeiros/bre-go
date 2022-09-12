# 43. Metrics Port and Decorator (Hexagonal)

## Status

Accepted — landed in v0.18.0. Adds the `observability.ExecutionMetric` typed event + `observability.ExecutionMetricSink` interface (the port) and `observability/metrics.Wrap` (the decorator). bre-go owns the contract; backends (OTel, Prometheus, custom in-house systems) adapt to it. v0.18.0 ships the port + decorator + a `RecordingSink` reference implementation; the OTel adapter follows in v0.19.0 when the upstream OTel metric SDK stabilizes. The pre-tag scientific review at [`scientific/v0.18.0/REPORT.md`](../../../scientific/v0.18.0/REPORT.md) implemented three independent sinks (channel-based, atomic-counter, sliding-window percentile) at ~50 LOC each — proves the hexagonal port enables others in practice, not just in theory.

## Context

v0.17.0 wired up the OpenTelemetry **span** adapter via `observability/otel.Wrap` — a direct decorator that depends on `go.opentelemetry.io/otel` API types. That choice was right for spans: OTel's tracing API has been stable since v1.0.0 (Feb 2022), Go-1.18-compatible since v1.11.0 (Nov 2022), and span semantics are unambiguous.

Metrics is different in three ways:

1. **OTel metric SDK was pre-1.0 in Aug–Dec 2022.** It didn't go GA until v1.16.0 (June 2023). If bre-go's metrics surface depends directly on a pre-1.0 SDK, every consumer using metrics gets dragged through breaking changes upstream.
2. **Real consumers split across multiple metrics backends.** OTel is the cloud-native default, but plenty of services emit straight to Prometheus, Datadog StatsD, or in-house systems that long predate OTel. A direct OTel decorator forces every metrics-curious consumer to add OTel deps even if they never use the OTel exporters.
3. **bre-go has its own semantic model that doesn't map 1:1 to OTel's.** Cancellation-is-not-error (from v0.17 ADR-0042), the typed adapter name, the optional per-rule cardinality, the bre-go-typed error kinds — these are bre-go's contract. Encoding them through OTel attribute conventions loses information.

The hexagonal/SOLID move is to invert the dependency. bre-go defines the metrics contract; backends adapt to it.

Three design alternatives were considered before this ADR.

### Alternatives

**Option A: OTel-direct decorator (mirrors v0.17 span adapter).**

```go
package otelmetric
func Wrap(inner engine.Engine, meter metric.Meter, opts ...Option) engine.Engine
```

- Pros: smallest LOC (~150). Familiar pattern.
- Cons: violates Dependency Inversion — bre-go depends on OTel's metric SDK directly. When that SDK churns (it did, 2022–2023), every metrics-using consumer is dragged along. Locks out non-OTel backends.

**Option B: bre-go-owned port + adapter (chosen).**

```go
package observability
type ExecutionMetric struct { ... }
type ExecutionMetricSink interface { RecordExecution(ExecutionMetric) }

package metrics
func Wrap(inner engine.Engine, sink observability.ExecutionMetricSink) engine.Engine
```

OTel adapter ships later as a separate sink:

```go
package otelmetric  // v0.19.0
func NewSink(meter metric.Meter, opts ...Option) observability.ExecutionMetricSink
```

- Pros: pure hexagonal, SOLID across all five principles, insulates consumers from OTel SDK churn, enables Prometheus / Datadog / custom in-house sinks without touching bre-go core.
- Cons: more LOC (~350 across port + decorator + adapter + tests). One more abstraction layer.

**Option C: Extend the existing v0.13 Listener model.**

Add a metrics-aware listener interface and ship an OTel implementation.

- Pros: reuses existing infra.
- Cons: listeners don't see `ctx`. The cancellation-is-not-error distinction we shipped in v0.17 can't be represented. Real semantic loss.

### Decision

Option B. bre-go owns the contract; OTel and friends are adapters.

Five SOLID principles, applied:

- **Single Responsibility.** The decorator's job: time Execute, build an ExecutionMetric, hand it to a sink. The sink's job: encode that metric in some backend-specific way. Neither does the other's work.
- **Open/Closed.** New backends plug in as new sink implementations without touching `observability/metrics`. New observability concerns (audit logs, custom dashboards) plug in as new decorators without touching anything that already works.
- **Liskov Substitution.** The decorator returns `engine.Engine`. It substitutes for the inner engine everywhere — including stacked under other decorators.
- **Interface Segregation.** The sink interface has one method, `RecordExecution(ExecutionMetric)`. Backends that just want totals implement that one method and ignore most fields. Backends that want fine-grained labels read more. Nothing forces them to depend on what they don't use.
- **Dependency Inversion.** bre-go core depends on its own abstraction (`ExecutionMetricSink`). OTel, Prometheus, custom backends depend on the abstraction too. The abstraction depends on nothing external.

Hexagonal architecture, applied: `engine.Engine` is the domain port at the center. The metrics decorator is an adapter. Each sink is an adapter to a specific external metrics backend. The metric flow is one-directional: domain → port → adapter → external system. bre-go core never imports OTel, Prometheus, or any other backend.

## Decision

### Port (in `observability/`)

```go
// ExecutionMetric is the typed event the metrics decorator emits per
// Execute. One value type covers success, error, and cancellation.
type ExecutionMetric struct {
    Adapter      string         // e.g., "*indexed.Engine"
    MatchedCount int
    MatchedNames []string
    Duration     time.Duration
    Err          error          // nil on success or cancellation
    Canceled     bool
    CancelReason string         // "canceled" / "deadline_exceeded" / ""
}

// ExecutionMetricSink consumes typed metric events. Implementations
// are the adapter half of the hexagonal port.
type ExecutionMetricSink interface {
    RecordExecution(ExecutionMetric)
}
```

Err and (Canceled, CancelReason) are mutually exclusive — matches the v0.17 ADR-0042 lesson: cancellation is caller intent, not engine failure.

### Decorator (in `observability/metrics/`)

```go
func Wrap(inner engine.Engine, sink observability.ExecutionMetricSink) engine.Engine
```

Returns a `*meteredEngine` that:

- Times the inner Execute call.
- Classifies the outcome (success / error / cancel-canceled / cancel-deadline).
- Builds an `ExecutionMetric` and hands it to the sink.
- Returns the original `(result, error)` to the caller unchanged.
- Forwards `RuleNames`, `RuleInfos`, `AddListener`, `Unwrap` to inner via the same idiom as `observability/otel.Wrap`.

### Reference sink (in `observability/metrics/`)

```go
type RecordingSink struct { ... }
func (s *RecordingSink) RecordExecution(observability.ExecutionMetric)
func (s *RecordingSink) Records() []observability.ExecutionMetric  // defensive copy
func (s *RecordingSink) Reset()
```

Thread-safe append-buffer. Useful for tests and for consumers who want a simple in-memory aggregation without pulling in a backend SDK.

### Composition

Stacked decorators preserve `engine.Engine` cleanly:

```go
inner   := indexed.New() // ... AddRule, Build
traced  := otel.Wrap(inner, tracer)                          // v0.17
metered := metrics.Wrap(traced, sink)                        // v0.18
metered.Execute(ctx, req)                                    // spans + metrics
```

### What v0.19.0 will add

```go
package otelmetric  // future
func NewSink(meter metric.Meter, opts ...Option) observability.ExecutionMetricSink
```

The OTel adapter ships when the upstream metric SDK stabilizes (v1.16.0, June 2023 in upstream-Go-time). Until then, v0.18 consumers using the metrics port can:

- Use `RecordingSink` for tests.
- Wire their own sink to whatever metrics backend they already use.
- Wait for v0.19.0 if they want the OTel-native path.

## Consequences

### Closed by v0.18.0

- bre-go has a stable, owned metrics contract. The `ExecutionMetric` shape and the `ExecutionMetricSink` interface won't break in subsequent v0.x releases (they're additive-only — new optional fields land at the end of the struct).
- Anyone can write a sink in ~50 LOC. No coupling to OTel, no transitive dep weight if you don't want OTel.
- Cancellation semantics from v0.17 carry across: `Canceled` + `CancelReason` separated from `Err`.
- Stacking with the v0.17 span decorator works out of the box — both implement and consume `engine.Engine`.

### NOT closed by v0.18.0

- **The OTel metric adapter.** Lands in v0.19.0 when the upstream SDK is stable. v0.18 consumers wanting OTel-native metrics either implement their own sink against the pre-1.0 OTel metric SDK (with the upstream-stability caveat that drove this split) or wait.
- **Per-rule-name metrics.** The decorator records `MatchedNames` in every `ExecutionMetric`; whether the sink emits per-rule counters is the sink's call (cardinality is a backend-specific tradeoff). The OTel adapter in v0.19 will offer it as an opt-in `WithPerRuleMetrics(true)` option.
- **Error-kind classification.** v0.18 records the raw `Err` in the struct. Each sink classifies — backends with type-label conventions can switch on bre-go's typed errors (`*ActionPanicError`, `*FanoutTooLargeError`, etc.); backends with free-form labels can use `err.Error()`. v0.19's OTel adapter will ship a default classifier.

### Performance impact

- Per-Execute overhead: one `time.Now`/`time.Since` pair, one struct allocation, one interface call to the sink. Sub-microsecond on modern hardware.
- Inner engine's Execute is called unchanged.
- The sink controls its own cost. `RecordingSink` is a mutex-guarded append; a no-op sink is one virtual call.

### Validation strategy

- 18 unit tests in `observability/metrics/metrics_test.go` cover: every metric field on the success path, error path populates `Err` only, both cancellation sentinels populate `Canceled` + `CancelReason` only, caller-visible return value unchanged, RuleNames/RuleInfos/AddListener forwarding when inner supports them, nil/no-op when inner doesn't, Unwrap, defensive-copy semantics of `RecordingSink.Records`, `Reset`, concurrent safety under 16-goroutine × 100-call stress. 100% coverage.
- A pre-tag scientific review at `scientific/v0.18.0/` implements three alternative sinks (channel-based, atomic-counter, percentile-tracker) at ~50 LOC each — proves the port surface is genuinely small enough that backends can be written quickly and cleanly. The harness output is the evidence.

### What this validates for v0.19.0+

The split-release plan (port now, OTel adapter later) sets the precedent for any future observability decorator that depends on an unstable upstream API: define the bre-go-owned contract first, ship adapters as the upstream stabilizes. Same logic could apply to a future logging decorator (slog has been stable since Go 1.21, so a direct wrapper is fine) or audit decorator (no upstream standard, so port-first is right).

The principle: bre-go's own contract is the stable surface. Adapters to external systems are version-coupled to those systems and can ship on their own cadence.
