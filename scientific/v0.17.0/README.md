# OTel Telemetry Usability Review (v0.17.0)

Sibling directory to [`scientific/v0.15.0`](../v0.15.0/). Where v0.15.0 measured *whether the snapshot path worked* and *how fast it was*, this directory measures *whether the OpenTelemetry adapter's output is operator-actionable*. The headline finding is in [REPORT.md](REPORT.md); this README is the reproduction guide.

## What gets measured

Eleven scenarios mirroring real production usage exercise the OTel adapter. The harness captures the resulting span trees + attributes + events as JSON and audits whether an operator looking at them in a tracing backend (Jaeger / Tempo / Honeycomb / Datadog) could answer the questions they'd actually have:

- Did the engine match a rule? Which one?
- Was the execution an error, a cancellation, or a clean run?
- Is it tied to a correlation ID? An adapter type?
- Where does it fit under the parent trace?
- Does it survive concurrency, unicode, large match sets?

The full scenario list is in [REPORT.md](REPORT.md). Raw captured spans land at [`results/scenarios.json`](results/scenarios.json).

## Reproducing

```sh
go run ./cmd/otel-review > results/scenarios.json
```

(From inside this directory, which has its own go.mod that replaces `github.com/helmedeiros/bre-go` to the checkout.)

Or, from the repo root:

```sh
(cd scientific/v0.17.0 && go run ./cmd/otel-review > results/scenarios.json)
```

Roughly 1 second wall time. No Docker, no multi-arch — this is a pure-Go review of attribute semantics, not a performance harness.

## What changed because of this review

Before the review, the adapter marked `context.Canceled` and `context.DeadlineExceeded` as `codes.Error` and recorded them via `span.RecordError`. The review flagged that as misleading — cancellation is caller intent, not failure — and the adapter was adjusted to use dedicated `rule.engine.canceled` + `rule.engine.cancel.reason` attributes with the span status left `Unset` (success). Two new exported constants ship alongside (`AttrCanceled`, `AttrCancelReason`). See [REPORT.md §5](REPORT.md#5-ctx-cancellation--adjusted-originally-a-design-issue) for the before/after.

No other adjustments needed.
