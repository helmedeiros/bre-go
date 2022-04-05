# bre-go

A Go business rule engine with a swappable engine port.

The public API is backend-agnostic. Today it ships with two small in-process engines for tests, examples, and lightweight production use; the long-term goal is to plug a mature open-source rule engine in behind the same interface so callers never have to change their code.

## Status

[![CI](https://github.com/helmedeiros/bre-go/actions/workflows/ci.yml/badge.svg)](https://github.com/helmedeiros/bre-go/actions/workflows/ci.yml)

`v0.1.0` -- first tagged release. Three concrete adapters, six public packages, twenty-one Architecture Decision Records on `main`. SemVer: pre-1.0 means breaking changes are still allowed but will land as a `0.x → 0.(x+1)` minor bump; see [ADR-0021](docs/architecture/decisions/0021-release-versioning-policy.md). The full design record and the current status of each ADR live in [`docs/architecture/decisions/`](docs/architecture/decisions/).

```sh
go get github.com/helmedeiros/bre-go@v0.1.0
```

## Stability

What you can build on today:

- **`engine.Engine` port and value types (`Request`, `Result`, `RuleInfo`).** Stable. Used by every adapter and the generic wrapper. Any change here would supersede ADR-0003.
- **Optional capability interfaces (`ListenerHost`, `RuleLister`, `RuleInfoLister`).** Stable. Adapters discover support through the standard Go type-assertion idiom.
- **Adapter registration validation (`ErrEmptyRuleName`, `ErrNilCondition`, `ErrDuplicateRuleName`).** Stable. Same sentinels, same shape-first-then-state-second check order on every adapter.
- **Listener lifecycle (`OnRuleMatched`, `OnExecutionStarted`, `OnExecutionFinished`, `OnExecutionErrored`).** Stable. Per-execution `OnRuleMatched` fires for successful matches only; `OnExecutionErrored` carries the typed `ActionPanicError`.
- **`engine/exec.Executor[In, Out]`.** Stable since Go 1.18 GA. Wraps any `engine.Engine`.

What may still change:

- **Adapter-internal `Rule` struct field order.** Today's adapters keep `Name` first by convention; a future ADR could land a builder pattern that hides the struct entirely.
- **Benchmark numbers.** No regression policy yet; numbers in `make bench` output are baselines for local comparison, not contract.
- **`docs/usage` tutorial layout.** Currently inline in README; may move to a dedicated `docs/` walkthrough as it grows.

## Quickstart

```go
package main

import (
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/conditions"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

type order struct {
	amount   int
	currency string
	flagged  bool
}

func main() {
	e := inmemory.New()
	counter := &observability.CountingListener{}
	e.AddListener(counter)

	_ = e.AddRule(inmemory.Rule{
		Name: "high-value-clean-usd",
		Condition: conditions.And(
			func(in interface{}) bool { return in.(order).amount > 100 },
			func(in interface{}) bool { return in.(order).currency == "USD" },
			conditions.Not(func(in interface{}) bool { return in.(order).flagged }),
		),
		Action: func(interface{}) interface{} { return "approve" },
	})

	res, _ := e.Execute(engine.Request{
		Input: order{amount: 250, currency: "USD"},
	})

	fmt.Println(res.Matched, res.Output, counter.Total())
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

## Toolkit

| Package | What it gives you |
|---------|-------------------|
| [`engine`](engine) | The `Engine` port, `Request`/`Result`/`RuleInfo` value types, the `ListenerHost`, `RuleLister`, and `RuleInfoLister` optional capability interfaces. |
| [`engine/inmemory`](engine/inmemory), [`engine/firstmatch`](engine/firstmatch), [`engine/priority`](engine/priority) | Three concrete adapters with different policies along two axes: ordering (insertion vs priority) and match policy (all vs first). |
| [`engine/conditions`](engine/conditions) | Boolean combinators (`And`, `Or`, `Not`) and sentinels (`Always`, `Never`) for declarative rule composition. |
| [`engine/exec`](engine/exec) | Generic `Executor[In, Out]` wrapper over any `engine.Engine`. Hides the `interface{}` cast at the call boundary; works with both shipped adapters and any future one. Requires Go 1.18. |
| [`engine/enginetest`](engine/enginetest) | Shared contract suite every adapter wires from a single test function. |
| [`observability`](observability) | `Logger` and `ExecutionListener` ports, three lifecycle role interfaces (`ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`), and the five built-ins: `NopLogger`, `NopExecutionListener`, `CountingListener`, `LoggingListener`, `TimingListener`. |

## License

[MIT](LICENSE).
