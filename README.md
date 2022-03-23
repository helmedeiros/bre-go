# bre-go

A Go business rule engine with a swappable engine port.

The public API is backend-agnostic. Today it ships with two small in-process engines for tests, examples, and lightweight production use; the long-term goal is to plug a mature open-source rule engine in behind the same interface so callers never have to change their code.

## Status

Early. The architecture is being built first, the engine implementations follow. See [`docs/architecture/decisions/`](docs/architecture/decisions/) for the design record.

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

All three adapters share the same `Rule`-shape skeleton (`Name`, `Condition`, `Action`; `priority.Rule` additionally carries `Priority int`), the same registration validation (`ErrEmptyRuleName`, `ErrNilCondition`, `ErrDuplicateRuleName`), satisfy `engine.ListenerHost` (so the observability built-ins `CountingListener`, `LoggingListener`, and `TimingListener` attach to any of them with `e.AddListener(...)`) plus `engine.RuleLister` (so `RuleNames()` enumerates the registered rule set for debug endpoints and startup logging), and recover panicking `Action`s into a typed `ActionPanicError` (with `RuleName()` for diagnostics) plus an `OnExecutionErrored` callback on listeners that opt in -- one buggy rule cannot crash the host process.

The same `enginetest.RunContractTests` suite runs against all three -- port-level behavior is identical, only the multi-rule policy differs.

## Toolkit

| Package | What it gives you |
|---------|-------------------|
| [`engine`](engine) | The `Engine` port, `Request`/`Result` value types, the `ListenerHost` and `RuleLister` optional capability interfaces. |
| [`engine/inmemory`](engine/inmemory), [`engine/firstmatch`](engine/firstmatch), [`engine/priority`](engine/priority) | Three concrete adapters with different policies along two axes: ordering (insertion vs priority) and match policy (all vs first). |
| [`engine/conditions`](engine/conditions) | Boolean combinators (`And`, `Or`, `Not`) and sentinels (`Always`, `Never`) for declarative rule composition. |
| [`engine/exec`](engine/exec) | Generic `Executor[In, Out]` wrapper over any `engine.Engine`. Hides the `interface{}` cast at the call boundary; works with both shipped adapters and any future one. Requires Go 1.18. |
| [`engine/enginetest`](engine/enginetest) | Shared contract suite every adapter wires from a single test function. |
| [`observability`](observability) | `Logger` and `ExecutionListener` ports, three lifecycle role interfaces (`ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`), and the five built-ins: `NopLogger`, `NopExecutionListener`, `CountingListener`, `LoggingListener`, `TimingListener`. |

## License

[MIT](LICENSE).
