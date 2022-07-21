# 38. Structured Telemetry Listener

## Status

Accepted — landed in v0.13.0. `observability.TelemetryRecord` +
`observability.TelemetrySink` + `observability.StructuredTelemetryListener`
ship in the `observability` package. The listener implements all
four lifecycle interfaces and emits records via the caller-
supplied sink. Last of the five Phase-4 parity-closure releases;
Phase 4 ends here.

## Context

After v0.12.0, every shape gap relative to the parity target is
closed and `Execute` is concurrent-safe with documented hot reload.
What's left is the observability story.

The parity target's matcher emits a structured log per Execute
containing latency, matched-rule name, and a small handful of other
fields. Today's bre-go observability primitives (`CountingListener`,
`LoggingListener`, `TimingListener`, `SnapshotListener`) are
useful for tests and ad-hoc instrumentation but require callers to
build their own "per-Execute record" type if they want the
matcher-shaped emission a logging / metrics / tracing backend
expects.

v0.13.0 ships the missing built-in: a listener that captures the
lifecycle events the adapters already emit and bundles them into a
typed record. The sink is a `func(TelemetryRecord)` the caller
provides — typically a thin wrapper around the application's
logger, metrics client, or tracing exporter.

Three design questions.

### 1. What does a `TelemetryRecord` contain?

Today's lifecycle interface carries:

```go
OnExecutionStarted(input)
OnRuleMatched(Match{Rule, Input, Output})
OnExecutionFinished(input, output, matched, duration)
OnExecutionErrored(input, err)
```

A record built strictly from what those callbacks provide can carry:

- `Input interface{}` — request input (whatever the caller's Execute
  was given).
- `Output interface{}` — `Result.Output` for the matched rule (nil
  if no match or no action).
- `Matched []string` — the names of rules that matched. One entry
  for first-match adapters; potentially many for inmemory.
- `Duration time.Duration` — wall-clock from start to finish.
- `Err error` — non-nil if OnExecutionErrored fired (action panic
  or ctx cancellation).

What it cannot carry, with today's lifecycle:

- **Candidate count** (rules considered before post-filter
  rejection). This is an adapter-internal concept; surfacing it
  cross-adapter would require either (a) extending the listener
  interface (breaking) or (b) an adapter-specific listener
  (sprawl). Defer to a future ADR if a real consumer asks.
- **Correlation ID** from context. The lifecycle hooks don't
  receive `context.Context`. Callers who need correlation set
  the context via `engine.WithCorrelationID` (v0.4.0) and read it
  inside their `ConditionContext` / `ActionContext` — or wrap
  Execute with their own per-request context propagation. A
  future ADR could extend the lifecycle to carry ctx; out of
  v0.13.0 scope.

### 2. How does the listener handle the error path?

OnExecutionErrored fires on action panic or ctx cancellation;
OnExecutionFinished fires after, with the partial result. Two
possible designs:

- **(a) Per-Execute correlation.** The listener tracks per-call
  state (was Errored called?) keyed by some per-Execute identifier
  and emits one record per Execute via OnExecutionFinished, with
  Err set if applicable. Requires per-goroutine state.
- **(b) Per-event emission.** The listener emits one record per
  terminal event. Success Execute → one record from
  OnExecutionFinished. Error Execute → one record from
  OnExecutionErrored AND one from OnExecutionFinished
  (with the partial result). The sink correlates if it wants.

Pick **(b)**. Per-goroutine state in a listener is a leaky
abstraction (the listener doesn't know which goroutine an event
came from without runtime hacks; the lifecycle interface doesn't
provide a per-call context). Per-event emission is honest about
what the lifecycle interface promises: events are independent,
caller-side correlation is the caller's job.

For loggers and metrics counters this is fine. For distributed
tracing where one span per Execute matters, the caller's sink
deduplicates by stack-walking the events. Sample sink in the
cookbook documents the pattern.

### 3. Where does the listener live?

Three options:

- (a) `observability.StructuredTelemetryListener` — alongside
  the existing built-ins.
- (b) New sub-package `observability/structured` — separates
  newer / heavier types from the existing surface.
- (c) New top-level package `engine/telemetry` — emphasizes
  the integration role.

Pick **(a)**. The listener is one type and one function (the
constructor). It belongs next to `CountingListener` /
`LoggingListener` / `TimingListener` / `SnapshotListener` —
every existing built-in lives in `observability`. A sub-package
or top-level package is more ceremony than the addition warrants.

## Decision

Add to `observability/`:

```go
// TelemetryRecord captures the lifecycle of a single Execute call
// in a structured form. Emitted by StructuredTelemetryListener
// via a caller-supplied sink.
//
// One record per terminal lifecycle event:
//   - Success path: one record from OnExecutionFinished, Err nil.
//   - Error path: two records -- one from OnExecutionErrored
//     (Err non-nil, partial Output/Matched/Duration data), one
//     from OnExecutionFinished (Err nil, partial Output).
//     Sinks that want one record per Execute correlate by
//     timestamp + Input pointer.
type TelemetryRecord struct {
    Input    interface{}
    Output   interface{}
    Matched  []string
    Duration time.Duration
    Err      error
}

// TelemetrySink consumes a structured telemetry record. Called
// once per terminal lifecycle event by StructuredTelemetryListener.
// Sinks MUST be safe for concurrent calls when the engine is built
// and Execute runs from multiple goroutines.
type TelemetrySink func(TelemetryRecord)

// StructuredTelemetryListener implements all four lifecycle
// interfaces and emits TelemetryRecord values via sink. See
// ADR-0038 for the design rationale.
type StructuredTelemetryListener struct {
    sink TelemetrySink
}

// NewStructuredTelemetryListener wires a sink. The constructor
// is the only way to instantiate the listener so the sink is
// always non-nil.
func NewStructuredTelemetryListener(sink TelemetrySink) *StructuredTelemetryListener
```

The listener implements:

- `ExecutionListener.OnRuleMatched(Match)` — no-op (per-match info
  is rolled up into TelemetryRecord.Matched at the terminal event).
- `ExecutionStartedListener.OnExecutionStarted(input)` — no-op
  (duration is in the terminal event).
- `ExecutionFinishedListener.OnExecutionFinished(input, output, matched, duration)` —
  emits TelemetryRecord with Err nil.
- `ExecutionErroredListener.OnExecutionErrored(input, err)` — emits
  TelemetryRecord with Err set; other fields populated from the
  call signature.

`NewStructuredTelemetryListener(nil)` panics — empty sink would
silently swallow telemetry, hiding a wiring bug.

### Cookbook addition

```go
import (
    "log"
    "time"

    "github.com/helmedeiros/bre-go/engine/indexed"
    "github.com/helmedeiros/bre-go/observability"
)

func wireTelemetry(e *indexed.Engine) {
    sink := func(rec observability.TelemetryRecord) {
        if rec.Err != nil {
            log.Printf("rule-execute error: input=%v err=%v", rec.Input, rec.Err)
            return
        }
        log.Printf("rule-execute matched=%v duration=%s",
            rec.Matched, rec.Duration)
    }
    e.AddListener(observability.NewStructuredTelemetryListener(sink))
}
```

A second example wires the listener to a metrics counter, showing
the deduplication pattern when the sink wants one event per
Execute.

### Test additions

Standard battery in `observability/structured_telemetry_test.go`:

- One record per success Execute.
- Two records (Errored + Finished) on action panic.
- Two records on ctx cancellation.
- Concurrent Execute from many goroutines produces correct sink
  call count (one per Execute on the success path, two per Execute
  on the error path) without races.
- Constructor with nil sink panics.

The `engine/indexed/lifecycle_test.go` test suite gains a
concurrent-telemetry case verifying the listener works correctly
under the v0.12.0 concurrency model.

## Consequences

### Closed by v0.13.0

- Built-in structured emission for matching activity. No more
  hand-rolling a per-Execute record type — callers use the
  library's `TelemetryRecord` and wire whichever sink they need.
- The last parity-target observability gap. Phase 4 ends here.

### Still open after v0.13.0

- **Candidate count per Execute** (adapter-internal: how many
  rules were considered before post-filter rejection). Today's
  lifecycle interface doesn't carry this. A future ADR may add an
  `IndexedTelemetryListener` for the adapter-specific signal, or
  extend the cross-adapter lifecycle to include it.
- **Context propagation through lifecycle hooks.** OnExecution*
  doesn't take `context.Context`, so listeners can't read
  correlation IDs from the request context. A future ADR could
  add `ctx` to the callbacks; out of v0.13.0 scope.
- **Sampling.** Production telemetry often samples at <100% to
  control volume. v0.13.0 ships unsampled emission; sampling lives
  in the caller's sink (the standard place for it in the Go
  ecosystem).

### Performance impact

- Per Execute: one struct copy + one function call (the sink).
  ~10-50 ns on Apple silicon for a no-op sink, dominated by the
  sink's body.
- No allocations beyond `TelemetryRecord` itself (struct value).
  The existing alloc tripwires per adapter continue to pass.
- Sinks themselves are caller code; performance there depends on
  what the sink does (log emission, metrics, tracing).

### Validation strategy

- Unit tests cover the four lifecycle methods + nil-sink panic.
- A pre-tag external scientific test wires the listener to a
  test sink and verifies record contents for the four lifecycle
  paths (success, action panic, ctx cancellation, no-match).
- The structured listener also gets exercised in the parity test
  fixture (`/tmp/bre-go-scrooge-parity/`) to verify it integrates
  with the existing lifecycle without breaking any of the 11
  scenario cases.

### What this closes about Phase 4

Phase 4's five parity-closure releases ship in order:

- v0.9.0 — `OpIn` set-membership + wildcard semantics (ADR-0034).
- v0.10.0 — `OpNeq` post-filter + value-expression syntax (ADR-0035).
- v0.11.0 — `RangeCondition` + custom post-filter hook (ADR-0036).
- v0.12.0 — build-then-execute + concurrent Execute + hot reload (ADR-0037).
- v0.13.0 — structured telemetry listener (this ADR).

After v0.13.0 the indexed adapter has cleared every shape and
operational gap the parity target exposes (within the documented
intentional divergences: first-match vs last-match-wins,
per-rule key-sets vs fixed dimensions). The next release work is
Phase 5 (`engine/gorules` adapter, v0.14.0+), which still blocks
on the upstream library's mid-2023 launch.
