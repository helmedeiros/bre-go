# 44. Public API Stability Tiers

## Status

Proposed — defines the stability contract for every exported symbol bre-go currently ships. Documents what each tier commits the project to and classifies every package's surface. Honest snapshot of the current commitment, not a roadmap for a future version.

## Context

After 18 minor releases, bre-go's public surface has grown to ~130 exported symbols across 13 public packages. Some have been stable since v0.1.0 (the `engine.Engine` port); others shipped within the last release (the metrics port from v0.18.0). Consumers reading the README can see *what* is available but not *how strongly* the project commits to each piece.

Three concrete problems this leaves unsolved:

- A consumer building production code against bre-go has no way to tell which symbols they can depend on without expecting churn. "Stable since vN.M.0" sprinkled through the README is informative but inconsistent — some entries have it, some don't, the phrasing varies.
- A maintainer touching the code has no documented signal for "this symbol's shape is settled; don't change it without an ADR" vs. "this is recent, still finding its shape, can be reshaped in the next minor release."
- An external reviewer evaluating bre-go has no way to scope their audit. Symbols at different stability tiers warrant different scrutiny.

This ADR documents the tier system itself and classifies every currently-shipped public symbol into one of four tiers.

## Decision

Four tiers. Each tier carries a specific commitment about breaking changes.

### Tier 1 — Frozen

> The symbol's shape will not change. New methods may be added to interfaces only via additive type-extending in a parallel package. Any change to the existing surface requires a new ADR superseding ADR-0044 and a major-version bump.

Frozen symbols are the load-bearing parts of the codebase. Every adapter implements them; every consumer calls them. Their shape has survived 18 minor releases unchanged.

### Tier 2 — Stable

> The symbol's shape is settled. Additive changes (new optional fields, new methods on a struct, new error sentinels) are allowed in minor releases. Breaking changes require a deprecation notice in one minor release before removal in the next.

Stable symbols have shipped at least one release earlier than current. They have tests, they have documentation, they have production-shape usage demonstrated in the cookbook or scientific harnesses.

### Tier 3 — Evolving

> The symbol's shape is mostly settled but the maintainers reserve the right to reshape it in any minor release with a deprecation notice in the prior release. Use it, but watch for deprecation banners.

Evolving symbols shipped in the most recent release window or have known open questions that might still drive reshaping.

### Tier 4 — Experimental

> The symbol exists but is not committed to long-term. May be reshaped or removed in any release without a deprecation cycle. Use at your own risk.

Reserved for symbols introduced as proof-of-concept or with explicit documentation marking them as such. bre-go does not currently ship anything at this tier.

## Classification

### `engine` (the core port)

| Tier | Symbol |
|------|--------|
| 1 | `Engine` interface, `Request`, `Result` |
| 2 | `RuleInfo`, `RuleLister`, `RuleInfoLister`, `ListenerHost`, `WithCorrelationID`, `CorrelationIDFromContext`, `Load`, `RuleConfig`, `RuleConfigProvider`, `ChainProviders` |

The port + its value types (`Request`, `Result`) are frozen. Every adapter implements `Execute(ctx, req) (Result, error)` and has done so since v0.1.0. Any change to that signature cascades through every adapter and every consumer; it's the contract.

Optional capability interfaces (`RuleLister`, `RuleInfoLister`, `ListenerHost`) are Stable rather than Frozen because they're discovery contracts — adapters opt in via type-assertion. Adding new optional capabilities is additive; reshaping existing ones is not.

`WithCorrelationID` and `CorrelationIDFromContext` are Stable since v0.4.0 (ADR-0026). Used by both the OTel adapter and the metrics port; reshaping them now would ripple through both.

The provider abstraction (`RuleConfig`, `RuleConfigProvider`, `ChainProviders`, `Load`) is Stable since v0.3.0–v0.4.0 (ADR-0023). Two concrete providers ship in `engine/csv` and `engine/json`; consumers can write their own.

### `engine/conditions`

| Tier | Symbol |
|------|--------|
| 2 | `Always`, `Never`, `And`, `Or`, `Not` |

Combinator helpers for the `func(interface{}) bool` predicate shape used by the linear adapters. Stable since v0.4.0.

### `engine/exec`

| Tier | Symbol |
|------|--------|
| 2 | `Executor`, `New`, `OutputTypeMismatchError` |

Typed wrapper over any `engine.Engine`. Stable since v0.5.0; updated for context propagation in v0.2.0 (ADR-0022).

### `engine/csv`, `engine/json`

| Tier | Symbol |
|------|--------|
| 2 | `Loader`, `LineParser` / `ItemParser`, `LoadError`, `NewLoader`, `NewLoaderFromReader` |

Provider implementations. Stable since v0.3.0 (csv) and v0.7.0 (json).

### `engine/inmemory`, `engine/firstmatch`, `engine/priority`

| Tier | Symbol |
|------|--------|
| 2 | `Engine`, `Rule`, `New`, `ActionPanicError`, `ErrDuplicateRuleName`, `ErrEmptyRuleName`, `ErrNilCondition` |

The three linear adapters. Each ships an `Engine` type implementing the port, a `Rule` struct, a `New` constructor, an `*ActionPanicError`, and three sentinel errors. The sentinel-error shape has been consistent across all three since v0.1.0–v0.2.0; reshaping it now would break every consumer that uses `errors.Is` against them.

### `engine/indexed`

| Tier | Symbol |
|------|--------|
| 2 | `Engine`, `Rule`, `New`, `Build`, `Built`, `RuleNames`, `RuleInfos`, `AddRule`, `Execute`, `AddListener`, `ActionPanicError`, `FanoutTooLargeError`, `ErrAlreadyBuilt`, `ErrDuplicateRuleName`, `ErrEmptyRuleName`, `ErrEngineBuilt`, `ErrIncompatibleInput`, `ErrNilMatch`, `ErrNoIndexableTerms`, `ErrNonIndexableCondition` |
| 2 | `DiagnoseReport`, `DeadRule`, `Diagnose` |
| 2 | `Snapshot`, `SnapshotRule`, `SnapshotCondition`, `SnapshotFormatVersion`, `RuleCallbacks`, `ExportSnapshot`, `LoadSnapshot`, `ErrSnapshotEmpty`, `ErrSnapshotFormatVersionMismatch`, `ErrSnapshotIncompatibleHook`, `ErrSnapshotMalformed` |
| 2 | `CompiledSnapshot`, `CompiledBucket`, `CompiledRuleRef`, `CompiledSnapshotFormatVersion`, `ExportCompiledSnapshot`, `LoadCompiledSnapshot`, `MarshalCompiledSnapshot`, `UnmarshalCompiledSnapshot`, `ErrCompiledSnapshotFormatVersionMismatch`, `ErrCompiledSnapshotMalformed` |
| 3 | `FieldValueSet`, `PreClassifiedRule`, `AddPreClassifiedRule`, `ExportPreClassifiedRules` |
| 3 | `PostFilterHook`, `WithPostFilterHook` |

The core indexed adapter API (Engine + Rule + lifecycle + Build/Built) is Stable; same shape since v0.8.0–v0.12.0, scientifically validated for both correctness and concurrency.

`Diagnose` ships since v0.14.0 (ADR-0039), conservative tier-1 dead-rule detection. Stable shape.

Both snapshot families are Stable. JSON snapshot (v0.15.0, ADR-0040) and binary compiled snapshot (v0.16.0, ADR-0041) ship as parallel paths. Each has its own format version with a refuse-on-mismatch contract; both have scientifically-audited cross-architecture portability.

The pre-classified rule path (`PreClassifiedRule` + `AddPreClassifiedRule` + `ExportPreClassifiedRules`) is Evolving — it shipped alongside the compiled snapshot in v0.16.0 but no scientific harness has audited its operator-facing semantics yet. The shape is plausible; the contract is not yet fully exercised.

`PostFilterHook` + `WithPostFilterHook` are Evolving for a different reason: they are the source of the "hook-bearing engines can't snapshot" friction documented in both ADR-0040 and ADR-0041. Both ADRs name this as an open question that would need a follow-up ADR with a real consumer driving the spec. Until that consumer arrives and that ADR ships, the hook contract is subject to change.

### `engine/parser`

| Tier | Symbol |
|------|--------|
| 2 | `Predicate`, `Parse`, `AsPredicate`, `AsCondition`, `AsRuleCondition`, `Condition`, `ParseToCondition`, `ParseValueExpression`, `StringCondition`, `SetCondition`, `RangeCondition`, `AndCondition`, `OrCondition`, `NotCondition`, `OpEq`, `OpNeq`, `OpIn`, `OpNotIn`, `ParseError`, `ValueExpressionError` |

The parser package. Stable since v0.5.0–v0.11.0; the typed `Condition` tree underpins both the indexed adapter and the snapshot wire formats. Reshaping the AST would ripple through every snapshot artifact ever written.

### `observability` (core)

| Tier | Symbol |
|------|--------|
| 2 | `Match`, `ExecutionListener`, `ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`, `FinishedEvent`, `ErroredEvent`, `Logger`, `Field`, `Bool`, `Int`, `String`, `Err`, `NopLogger`, `NopExecutionListener`, `CountingListener`, `LoggingListener`, `SnapshotListener`, `TimingListener`, `StructuredTelemetryListener`, `NewStructuredTelemetryListener`, `TelemetryRecord`, `TelemetrySink` |
| 2 | `ExecutionMetric`, `ExecutionMetricSink` |

Listener interfaces (lifecycle + `Match`) are Stable since v0.4.0–v0.13.0. The structured telemetry surface from v0.13.0 (ADR-0038) is part of the same family.

The metrics port (`ExecutionMetric`, `ExecutionMetricSink`) is Stable as of v0.18.0 — the shape was the subject of the v0.18.0 scientific review that proved three independent backend adapters can implement it cleanly. The tier is Stable rather than Evolving because the contract is exactly what ADR-0043 committed to; reshaping it would invalidate the scientific evidence.

### `observability/metrics`

| Tier | Symbol |
|------|--------|
| 2 | `Wrap`, `RecordingSink` |

Decorator + reference sink. Stable since v0.18.0; the decorator pattern is shared with `observability/otel`.

### `observability/otel`

| Tier | Symbol |
|------|--------|
| 2 | `Wrap`, `WithSpanName`, `Option`, `AttrAdapter`, `AttrCanceled`, `AttrCancelReason`, `AttrCorrelationID`, `AttrMatchedCount`, `AttrMatchedNames` |

OTel span adapter. Stable since v0.17.0; the cancellation-vs-error semantics were the subject of the v0.17.0 scientific review.

## Symbols at risk before any contract freeze

These are surfaces where the current implementation works but the documented commitment is weaker than the others. Calling them out so a maintainer touching the code in the near future knows where the soft spots are.

- **`engine/indexed.PostFilterHook` + `WithPostFilterHook`.** Open per ADR-0040 §3 and ADR-0041 "not closed by v0.16.0." Hook-bearing engines refuse to snapshot, which is a known friction. Whatever ADR closes it may need to reshape the hook surface.
- **`engine/indexed.PreClassifiedRule` family.** Shipped in v0.16.0 alongside the compiled snapshot but has had no operator-facing audit. Useful for a consumer loading rules from a pre-processed source; semantics may need clarification.
- **Sentinel-error duplication across adapters.** `ErrEmptyRuleName` exists as a separate `var` in `engine/inmemory`, `engine/firstmatch`, `engine/priority`, and `engine/indexed`. Each is a distinct value. Consumers using `errors.Is` across adapters have to import each one or use the type's error string. A unified `engine.ErrEmptyRuleName` (and friends) at the port level would be cleaner — but the existing names are Stable, so this would be a deprecation cycle.

## Consequences

### Closed by this ADR

- Every currently-shipped public symbol has a documented tier. A consumer can read this ADR and know what commitment bre-go is making about each piece of the API they use.
- Maintainers have a documented criterion for "is it OK to reshape this." Tier 1 requires an ADR superseding this one; Tier 2 requires a deprecation cycle; Tier 3 can change with a deprecation notice in the prior release; Tier 4 can change without notice.
- External reviewers can scope their audit. Tier 1 + 2 should be most rigorous; Tier 3 carries explicit "may evolve" disclosure.

### NOT closed by this ADR

- No deprecation decisions. The "Symbols at risk" section names current soft spots but does not decide what to do about them. Each one would need its own ADR.
- No migration guide. That's a separate document, written when an actual breaking change is queued.
- No comparative benchmark vs. other Go business-rule engines. That's a separate scientific harness.

### Process

When a future ADR proposes a change to a Tier 1 or Tier 2 symbol, it MUST cite this ADR's classification and either supersede the relevant row or include the deprecation plan. Changes to Tier 3 symbols don't require this but the ADR should still announce the deprecation in the prior release.

### Re-audit cadence

This ADR is a snapshot. Re-audit when the public surface changes meaningfully — a new Tier 2 symbol lands, a Tier 3 graduates to Tier 2, a symbol is deprecated. The re-audit either edits this ADR's classification table in place (for minor adjustments) or files a superseding ADR (for tier-system changes).
