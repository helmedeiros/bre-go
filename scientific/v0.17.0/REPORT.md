# v0.17.0 OpenTelemetry Adapter — Telemetry Usability Review

> **TL;DR.** Eleven scenarios mirroring real production usage exercised the OTel adapter; the captured spans are committed at [`results/scenarios.json`](results/scenarios.json). Ten scenarios produce clear, operator-actionable telemetry on the first read. The eleventh (context cancellation) surfaced a design issue: cancellation was being marked as `codes.Error`, which would inflate error-rate dashboards even though cancellation is caller intent, not failure. The adapter was **adjusted before tagging**: `context.Canceled` and `context.DeadlineExceeded` now carry dedicated `rule.engine.canceled` + `rule.engine.cancel.reason` attributes with `status = Unset` (success), while every other error path remains `codes.Error`. Two new exported attribute key constants land in v0.17.0: `AttrCanceled` and `AttrCancelReason`.

## What this review is

After v0.17.0 wired up the OpenTelemetry adapter, the question wasn't "do tests pass" — they do — but "would a human operator looking at the span in Jaeger / Tempo / Honeycomb actually be able to do their job?" Eleven scenarios, each one written to mirror a real production situation an operator might face, run through the adapter. The resulting span trees + attributes get captured as JSON and audited side-by-side with the question "would I be able to answer X with this data?"

The harness is [`scientific/v0.17.0/cmd/otel-review/main.go`](cmd/otel-review/main.go). Output is [`results/scenarios.json`](results/scenarios.json). Anyone can re-run with `go run ./cmd/otel-review > results/scenarios.json` and re-audit.

## Scenarios and findings

### 1. happy-single-match — PASS

One input that matches one rule.

```
status_code: Unset
attributes:
  rule.engine.adapter        = "*indexed.Engine"
  rule.engine.matched.count  = 1
  rule.engine.matched.names  = ["br"]
duration_ns: 22458
```

Operator can answer: "Did the engine match? Yes. Which rule? `br`. How long did it take? 22µs." Clear.

### 2. happy-no-match — PASS

Input matches nothing.

```
status_code: Unset
attributes:
  rule.engine.matched.count  = 0
  rule.engine.matched.names  = []
```

Operator can answer: "Did the engine run? Yes (count is present). Did anything match? No (count = 0)." The empty `names` slice is explicit, not omitted — backend filters like `matched.count = 0` work cleanly.

### 3. happy-multi-match — INFORMATIONAL (not an OTel issue)

Setup: three rules with overlapping conditions. Input matches all three.

Result: `matched.count = 1, matched.names = ["br-equality"]`.

This is **bre-go's first-match semantics** showing through. The indexed adapter (and the other three) return on the first matching rule. The scenario's "expected three names" was an incorrect expectation on the harness author's part, not an adapter issue.

**Implication for the OTel adapter:** `matched.names` is always either `[]` or `[<one_name>]`. The `count` attribute is technically redundant (it's always 0 or 1), but operationally useful — backends filter integers more naturally than slice-length predicates. Keep both.

### 4. error-action-panic — PASS

A rule whose `Action` panics.

```
status_code: Error
status_description: "indexed: action of rule \"explodes\" panicked: synthetic action panic"
attributes:
  rule.engine.matched.names = ["explodes"]
events:
  - name: "exception"
    attributes:
      exception.type:    "*indexed.ActionPanicError"
      exception.message: "indexed: action of rule \"explodes\" panicked: synthetic action panic"
```

Operator can answer: "Which rule panicked? `explodes`. What was the error type? `*indexed.ActionPanicError`. The status description even quotes the rule name." Triage-ready.

### 5. ctx-cancellation — **ADJUSTED** (originally a design issue)

**Before the adjustment** (initial implementation):

```
status_code: Error
status_description: "context canceled"
events:
  - name: "exception"
    attributes:
      exception.type:    "*errors.errorString"
      exception.message: "context canceled"
```

This was the design issue. Context cancellation is caller intent — an upstream RPC was canceled, a timeout fired, a graceful shutdown started. None of those are "the rule engine failed." Marking the span as `codes.Error` would mean:

- Error-rate dashboards count cancellations as failures, inflating noise.
- Operators triaging "why are errors up?" would chase ghosts caused by upstream timeouts they have no control over.
- The semantic of `exception.type = "*errors.errorString"` loses the original `context.Canceled` sentinel identity.

**After the adjustment** (what v0.17.0 actually ships):

```
status_code: Unset
attributes:
  rule.engine.canceled       = true
  rule.engine.cancel.reason  = "canceled"
  rule.engine.matched.count  = 0
  rule.engine.matched.names  = []
```

Now:

- Status stays `Unset` (success-by-default). Error-rate dashboards are not polluted.
- `rule.engine.canceled = true` makes "show me canceled executions" a one-attribute filter in any backend.
- `rule.engine.cancel.reason` distinguishes `"canceled"` (`context.Canceled`) from `"deadline_exceeded"` (`context.DeadlineExceeded`).
- No `exception` event — the cancellation isn't an exception.

The Go return value still carries `context.Canceled` / `context.DeadlineExceeded` unchanged; only the OTel surface representation changed.

**Two new exported attribute key constants** ship as part of this adjustment:

```go
const (
    AttrCanceled     = "rule.engine.canceled"
    AttrCancelReason = "rule.engine.cancel.reason"
)
```

### 6. correlation-id-set — PASS

```
attributes:
  rule.engine.correlation_id = "req-abc-123"
```

Operator can answer: "Which request was this? `req-abc-123`." The attribute flows from `engine.WithCorrelationID(ctx, id)` automatically — no caller-side wiring needed.

### 7. correlation-id-absent — PASS

```
attributes:
  // rule.engine.correlation_id NOT present
```

The attribute is absent (not present-with-empty-string). Backend queries like `correlation_id IS NOT NULL` work cleanly. The implementation does an empty-string check in `Execute` and only emits the attribute when a value is set.

### 8. nested-under-parent — PASS

The parent span:

```
name: "incoming-http-request"
trace_id: "62663411a423b3b5ad8defa12d42e1ee"
span_id:  "1dbcdc055aee2a56"
```

The rule.engine.execute span:

```
name: "rule.engine.execute"
trace_id:  "62663411a423b3b5ad8defa12d42e1ee"  // same trace
span_id:   "c6383f877907f087"
parent_id: "1dbcdc055aee2a56"                  // parent of incoming-http-request
```

Standard OTel parent-child relationship; backends render the rule execution as a child node under the request span. Clear.

### 9. concurrent-executes — PASS

Eight goroutines each start their own parent span and call `Execute`. Captured: eight `worker-N` spans and eight `rule.engine.execute` spans, each pair sharing a distinct trace ID with the rule span correctly parented to its goroutine's worker span. No cross-contamination, no shared state leaking through the adapter. Confirms the wrapper is safe under concurrent calls (which the inner indexed adapter has been since v0.12.0).

### 10. high-fanout-100-matches — INFORMATIONAL (not an OTel issue)

Setup: 100 rules with overlapping conditions. Result: still 1 match (first-match). `matched.names = ["rule-000"]`. The "high fanout" concern that motivated the scenario is moot for bre-go's first-match semantics — the slice is always 0 or 1 elements.

If a future adapter ever supported multi-match, this scenario would need a real ceiling discussion (Jaeger truncates string attributes at ~64KB; a slice of 10000 rule names would hit that). For v0.17.0 with first-match-only adapters, no action needed.

### 11. unicode-rule-name — PASS

```
attributes:
  rule.engine.matched.names = ["regra-brasileira-ção-☃-🇧🇷"]
```

Multi-byte UTF-8 + emoji preserved unchanged through OTel attribute encoding. No mojibake, no truncation, no escaping.

## Summary table

| ID | Scenario | Verdict | Notes |
|----|---|---|---|
| 1 | happy-single-match | PASS | |
| 2 | happy-no-match | PASS | |
| 3 | happy-multi-match | INFORMATIONAL | First-match semantics, not OTel issue |
| 4 | error-action-panic | PASS | rule name in status description AND in `matched.names` |
| 5 | **ctx-cancellation** | **ADJUSTED** | now uses `canceled` + `cancel.reason` attributes, status stays Unset |
| 6 | correlation-id-set | PASS | |
| 7 | correlation-id-absent | PASS | attribute correctly omitted |
| 8 | nested-under-parent | PASS | |
| 9 | concurrent-executes | PASS | no cross-contamination |
| 10 | high-fanout-100-matches | INFORMATIONAL | first-match makes the scenario moot |
| 11 | unicode-rule-name | PASS | bytes preserved |

10 PASS / 1 ADJUSTED / 2 INFORMATIONAL.

## Adjustments made before v0.17.0 tag

**Adjustment 1 (shipped):** Distinguish cancellation from failure. `context.Canceled` and `context.DeadlineExceeded` now go through a dedicated branch that:
- Sets `rule.engine.canceled = true` and `rule.engine.cancel.reason = "canceled"` / `"deadline_exceeded"`.
- Leaves `Status` as `Unset` (success-by-default in OTel semantics).
- Does NOT call `span.RecordError` (cancellation isn't an exception).

Every other error path still calls `span.RecordError` + `span.SetStatus(codes.Error, ...)`. Caller-side behavior unchanged — the Go function still returns the original `context.Canceled` / `context.DeadlineExceeded` error.

Two new exported constants land alongside: `AttrCanceled` and `AttrCancelReason`.

## Adjustments considered and rejected

- **Drop `AttrMatchedCount` as redundant with `len(AttrMatchedNames)`.** Some backends (Tempo, Datadog) make integer filtering easier than slice-cardinality predicates. Kept for ergonomics.
- **Strip the leading `*` from `AttrAdapter` value (`*indexed.Engine` → `indexed.Engine`).** Considered cosmetic; backends supporting wildcards filter `*Engine` cleanly. Left as the literal Go type name.
- **Document "Unset = success" in the cookbook.** This is OTel idiom; operators familiar with OTel know it. The cookbook entry already describes the success path implicitly. Adding a "Unset means success" note would be more confusing than helpful.
- **Truncate `AttrMatchedNames` above N elements.** Moot for first-match adapters. Revisit if a future multi-match adapter lands.

## Conclusion

The OpenTelemetry adapter as shipped in v0.17.0 produces telemetry that an operator can use to answer the questions they'd actually have in production:

- Did the engine match a rule? (`matched.count`)
- Which one? (`matched.names`)
- How long did it take? (span duration)
- Was it an error? (`status` + `exception` event with the typed error and the offending rule's name)
- Was it canceled by the caller? (`canceled` + `cancel.reason` attributes — separated from error path)
- Which request was this part of? (`correlation_id`)
- Where does it fit in the parent trace? (standard OTel parent-child)
- Is it tied to a specific adapter? (`adapter`)

No further blockers for v0.17.0 tag. The next item on the list is publishing the release artifacts and tagging.
