# bre-go

A Go business rule engine with a swappable engine port.

The public API is backend-agnostic. Today it ships with three in-process engines (insertion-order all-match, insertion-order first-match, priority-ordered first-match) and CSV + JSON rule loaders; the long-term goal is to plug a mature open-source rule engine in behind the same interface so callers never have to change their code.

## Status

[![CI](https://github.com/helmedeiros/bre-go/actions/workflows/ci.yml/badge.svg)](https://github.com/helmedeiros/bre-go/actions/workflows/ci.yml)

`v0.7.2` -- patch release on top of v0.7.1. Adds the three Go-native regression gates from ADR-0032: per-adapter allocation tripwires (gating `ci-local`), fuzz targets for the parser and JSON loader (`make fuzz-quick`), and build-tagged stress tests (`make stress`). Three concrete adapters, rule loading from CSV and JSON, eleven public packages plus one test-only sibling (`engine/enginetest/bench`), thirty-two Architecture Decision Records on `main`. SemVer: pre-1.0 means breaking changes are still allowed but land as a `0.x → 0.(x+1)` minor bump; see [ADR-0021](docs/architecture/decisions/0021-release-versioning-policy.md). The full design record and the current status of each ADR live in [`docs/architecture/decisions/`](docs/architecture/decisions/).

```sh
go get github.com/helmedeiros/bre-go@v0.7.2
```

### Upgrading from v0.1.0

`v0.2.0` is a breaking change: `engine.Engine.Execute` and `engine/exec.Executor.Execute` now take `context.Context` as their first parameter (ADR-0022). The migration is mechanical:

```go
// v0.1.0
res, err := eng.Execute(engine.Request{Input: x})
out, matched, err := ex.Execute(x)

// v0.2.0
res, err := eng.Execute(ctx, engine.Request{Input: x})
out, matched, err := ex.Execute(ctx, x)
```

`Rule` structs gain optional `ConditionContext` and `ActionContext` fields; existing rule definitions compile unchanged.

## Stability

What you can build on today:

- **`engine.Engine` port and value types (`Request`, `Result`, `RuleInfo`).** Stable. Used by every adapter and the generic wrapper. Any change here would supersede ADR-0003.
- **Optional capability interfaces (`ListenerHost`, `RuleLister`, `RuleInfoLister`).** Stable. Adapters discover support through the standard Go type-assertion idiom.
- **Adapter registration validation (`ErrEmptyRuleName`, `ErrNilCondition`, `ErrDuplicateRuleName`).** Stable. Same sentinels, same shape-first-then-state-second check order on every adapter.
- **Listener lifecycle (`OnRuleMatched`, `OnExecutionStarted`, `OnExecutionFinished`, `OnExecutionErrored`).** Stable. Per-execution `OnRuleMatched` fires for successful matches only; `OnExecutionErrored` carries the typed `ActionPanicError`.
- **`engine/exec.Executor[In, Out]`.** Stable since Go 1.18 GA. Wraps any `engine.Engine`. `Execute` takes `context.Context` since v0.2.0.
- **`context.Context` propagation through `Execute`.** Stable since v0.2.0. A nil ctx is treated as `context.Background()` for test ergonomics; production code passes the real ctx.
- **`engine.RuleConfig`, `engine.RuleConfigProvider[RC]`, `engine.Load[RC]`.** Stable since v0.3.0. The loader abstraction; any provider that returns `[]RC` works with any adapter via the `Load` helper.
- **`engine/csv.Loader[RC]`.** Stable since v0.3.0. Reads CSV from a file or `io.Reader`, applies a caller-supplied `LineParser` per row.
- **`engine/json.Loader[RC]`.** Stable since v0.7.0. Reads a top-level JSON array from a file or `io.Reader`, applies a caller-supplied `ItemParser` per element (the parser receives a `json.RawMessage` and does its own wire-to-engine mapping). `LoadError.Index` is 0-indexed; `-1` for document-level failures.
- **`engine.ChainProviders[RC](providers...)`.** Stable since v0.4.0. Combines multiple `RuleConfigProvider[RC]` into one; concatenates in order; first-error short-circuits.
- **`engine.WithCorrelationID(ctx, id)` and `engine.CorrelationIDFromContext(ctx)`.** Stable since v0.4.0. Standard context-key helpers for stamping a request-scoped identifier; `ConditionContext` / `ActionContext` callbacks read the ID inside `Execute`.
- **`engine/parser` package.** Stable since v0.5.0. Compiles expression strings (`==`, `!=`, `IN`, `NOT IN`, `AND`, `OR`, `NOT`) into `Predicate`s, with `AsCondition` bridging them to `Rule.Condition`. String literals only; numeric and boolean literals stay out of scope until a real caller asks.
- **Typed `parser.Condition` tree (`StringCondition`, `SetCondition`, `AndCondition`, `OrCondition`, `NotCondition`).** Stable since v0.6.0. `ParseToCondition` returns an inspectable / marshallable tree; `AsRuleCondition` bridges typed Conditions to `Rule.Condition`. Op constants (`OpEq`, `OpNeq`, `OpIn`, `OpNotIn`) for ergonomics.

What may still change:

- **Adapter-internal `Rule` struct field order.** Today's adapters keep `Name` first by convention; a future ADR could land a builder pattern that hides the struct entirely.
- **Benchmark numbers.** No regression policy yet; numbers in `make bench` output are baselines for local comparison, not contract.
- **Additional loader formats.** `engine/csv` and `engine/json` are the first two concrete providers; an `engine/yaml` is a likely follow-up when a real caller asks. The `engine.RuleConfigProvider` interface itself is stable. NDJSON / streaming variants stay deferred.

## Quickstart

```go
package main

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine/conditions"
	"github.com/helmedeiros/bre-go/engine/exec"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

type Order struct {
	Amount   int
	Currency string
	Flagged  bool
}

func main() {
	e := inmemory.New()
	counter := &observability.CountingListener{}
	e.AddListener(counter)

	_ = e.AddRule(inmemory.Rule{
		Name:        "high-value-clean-usd",
		Description: "approve high-value USD orders that are not flagged",
		Tags:        []string{"approval"},
		Condition: conditions.And(
			func(in interface{}) bool { return in.(Order).Amount > 100 },
			func(in interface{}) bool { return in.(Order).Currency == "USD" },
			conditions.Not(func(in interface{}) bool { return in.(Order).Flagged }),
		),
		Action: func(interface{}) interface{} { return "approve" },
	})

	// Wrap the engine for typed input/output; underlying adapter unchanged.
	ex := exec.New[Order, string](e)
	decision, matched, _ := ex.Execute(context.Background(), Order{Amount: 250, Currency: "USD"})

	fmt.Println(matched, decision, counter.Total())
	// Output: [high-value-clean-usd] approve 1
}
```

## Which adapter do I want?

| Adapter | Semantics | Pick it when |
|---------|-----------|--------------|
| [`engine/inmemory`](engine/inmemory) | Evaluate every rule in insertion order; last matching action wins on `Output`; every match appears in `Matched`. | You want all decisions a rule set produces, accumulate counts via a listener, or run a "every rule should fire if applicable" policy. |
| [`engine/firstmatch`](engine/firstmatch) | Evaluate in insertion order; return on the first matching rule. Later rules are never evaluated and their actions never run. | You have a decision table, a content classifier, or a "first applicable rate" policy where rule order encodes precedence positionally. |
| [`engine/priority`](engine/priority) | Evaluate in descending `Priority` order (ties broken by insertion); return on the first matching rule. | You load rules from a config file or compose them from multiple sources, and precedence belongs in the data (not in `AddRule` call order). |

All three adapters share the same `Rule`-shape skeleton (`Name`, optional `Description`, optional `Tags`, `Condition`, `Action`; `priority.Rule` additionally carries `Priority int`), the same registration validation (`ErrEmptyRuleName`, `ErrNilCondition`, `ErrDuplicateRuleName`), satisfy `engine.ListenerHost` (so the observability built-ins `CountingListener`, `LoggingListener`, and `TimingListener` attach to any of them with `e.AddListener(...)`) plus `engine.RuleLister` (cheap `RuleNames()` for "what's in here?") plus `engine.RuleInfoLister` (richer `RuleInfos()` returning `RuleInfo{Name, Description, Tags}` for catalog endpoints), and recover panicking `Action`s into a typed `ActionPanicError` (with `RuleName()` for diagnostics) plus an `OnExecutionErrored` callback on listeners that opt in -- one buggy rule cannot crash the host process.

The same `enginetest.RunContractTests` suite runs against all three -- port-level behavior is identical, only the multi-rule policy differs.

For more patterns (listener composition, error handling, typed `Executor`, debug endpoints, adapter-agnostic helpers), see the [Cookbook](docs/cookbook.md).

## Toolkit

| Package | What it gives you |
|---------|-------------------|
| [`engine`](engine) | The `Engine` port, `Request`/`Result`/`RuleInfo` value types, the `ListenerHost`/`RuleLister`/`RuleInfoLister` optional capability interfaces, plus loader (`RuleConfig`, `RuleConfigProvider`, `Load`, `ChainProviders`) and correlation (`WithCorrelationID`, `CorrelationIDFromContext`) helpers. |
| [`engine/inmemory`](engine/inmemory), [`engine/firstmatch`](engine/firstmatch), [`engine/priority`](engine/priority) | Three concrete adapters with different policies along two axes: ordering (insertion vs priority) and match policy (all vs first). |
| [`engine/conditions`](engine/conditions) | Boolean combinators (`And`, `Or`, `Not`) and sentinels (`Always`, `Never`) for declarative rule composition. |
| [`engine/exec`](engine/exec) | Generic `Executor[In, Out]` wrapper over any `engine.Engine`. Hides the `interface{}` cast at the call boundary; works with both shipped adapters and any future one. Requires Go 1.18. |
| [`engine/csv`](engine/csv) | CSV-backed `engine.RuleConfigProvider`. `Loader[RC]` reads rules from a file or `io.Reader`, calls a caller-supplied `LineParser` for each row. `LoadError` carries the row number for diagnostics. |
| [`engine/json`](engine/json) | JSON-backed `engine.RuleConfigProvider`. `Loader[RC]` reads a top-level array from a file or `io.Reader`, calls a caller-supplied `ItemParser` per element (`json.RawMessage` in, typed `RuleConfig` out). `LoadError` carries the 0-indexed array position. |
| [`engine/enginetest/bench`](engine/enginetest/bench) | Cross-adapter performance benchmark harness. `Workload` describes a matrix cell (rules × dimensions × position × selectivity); `Run` drives `engine.Engine` through `b.N` iterations after seeding rules outside the timed section. Test-only sibling to `engine/enginetest`. See [`BENCHMARKS.md`](BENCHMARKS.md) for frozen baselines. |
| [`engine/parser`](engine/parser) | Expression DSL. `Parse(expr)` compiles a condition string into a `Predicate`; `AsCondition(pred, factOf)` bridges it to `Rule.Condition`. Grammar: `==` / `!=` / `IN` / `NOT IN` / `AND` / `OR` / `NOT` over string literals. |
| [`engine/enginetest`](engine/enginetest) | Shared contract suite every adapter wires from a single test function. |
| [`observability`](observability) | `Logger` and `ExecutionListener` ports, three lifecycle role interfaces (`ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`), and six built-ins: `NopLogger`, `NopExecutionListener`, `CountingListener`, `LoggingListener`, `TimingListener`, `SnapshotListener` (test-helper that captures all four lifecycle events for later assertion). |

## License

[MIT](LICENSE).
