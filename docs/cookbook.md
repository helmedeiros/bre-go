# Cookbook

Realistic patterns for using `bre-go`. The [Quickstart in the README](../README.md#quickstart) covers the smallest happy-path example; this document is for everything past that.

All examples target `v0.2.0` or later. `context.Context` is the first parameter of every `Execute` call.

## Contents

- [Patterns](#patterns)
  - [Pick the right adapter](#pick-the-right-adapter)
  - [Compose conditions declaratively](#compose-conditions-declaratively)
  - [Handle cancellation and timeouts](#handle-cancellation-and-timeouts)

## Patterns

### Pick the right adapter

The library ships three adapters with different semantics. Match the adapter to the policy your rule set encodes.

| Policy | Adapter |
|--------|---------|
| Evaluate every rule, every matching action runs, last action wins on `Output` | `engine/inmemory` |
| Walk in insertion order, return on the first matching rule | `engine/firstmatch` |
| Walk in descending `Priority` order, return on the first matching rule | `engine/priority` |

A useful starting heuristic: if your rule set comes from a config file where one row should always win for a given input, you want `firstmatch` or `priority` (decision-table semantics). If you're computing per-rule effects that *accumulate* (counters, audit trails), you want `inmemory`.

```go
// firstmatch: positional precedence -- order of AddRule wins
e := firstmatch.New()
_ = e.AddRule(firstmatch.Rule{Name: "premium", Condition: isPremium, Action: chargePremium})
_ = e.AddRule(firstmatch.Rule{Name: "standard", Condition: isStandard, Action: chargeStandard})
_ = e.AddRule(firstmatch.Rule{Name: "default", Condition: conditions.Always(), Action: chargeDefault})

// priority: explicit precedence in the data
p := priority.New()
_ = p.AddRule(priority.Rule{Name: "blocklist", Priority: 1000, Condition: onBlocklist, Action: deny})
_ = p.AddRule(priority.Rule{Name: "vip",       Priority: 100,  Condition: isVIP,        Action: approve})
_ = p.AddRule(priority.Rule{Name: "default",   Priority: 0,    Condition: conditions.Always(), Action: standard})
```

Both expressions of the same logic. Pick the one where the precedence lives where it belongs: in the call site (`firstmatch`) or in the data (`priority`).

### Compose conditions declaratively

`engine/conditions` lets you build predicates as trees instead of one fat closure. The same combinators (`And`, `Or`, `Not`, `Always`, `Never`) drop into any adapter's `Rule.Condition` field.

```go
import "github.com/helmedeiros/bre-go/engine/conditions"

approveCondition := conditions.And(
    conditions.Or(
        amountGreaterThan(100),
        currencyEquals("USD"),
    ),
    conditions.Not(flagged),
    conditions.Not(onBlocklist),
)

_ = e.AddRule(inmemory.Rule{
    Name:      "approve",
    Condition: approveCondition,
    Action:    approve,
})
```

Why this matters: each leaf predicate (`amountGreaterThan`, `currencyEquals`, `flagged`, `onBlocklist`) is independently testable, independently reusable across rules, and named. The composition itself reads in English order.

Short-circuit evaluation in argument order: `And` returns false on the first false, `Or` returns true on the first true. Place expensive predicates (DB lookups, remote calls) *after* cheap ones.

### Handle cancellation and timeouts

Since `v0.2.0`, `Execute` takes `context.Context` as the first parameter (ADR-0022). A cancelled context causes the engine to:

1. Fire `OnExecutionErrored(input, ctx.Err())` on any listener that supports it.
2. Fire `OnExecutionFinished(...)` so the lifecycle pair stays balanced.
3. Return the partial `Result` plus `ctx.Err()`.

The typical wiring in a request handler:

```go
func handle(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
    defer cancel()

    res, err := eng.Execute(ctx, engine.Request{Input: parseRequest(r)})
    if errors.Is(err, context.DeadlineExceeded) {
        http.Error(w, "rule evaluation timed out", http.StatusGatewayTimeout)
        return
    }
    if err != nil {
        // ActionPanicError or anything else
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    write(w, res)
}
```

For predicates or actions that themselves do I/O (a DB lookup, a remote call), use the `ConditionContext` / `ActionContext` fields on `Rule` so the ctx flows through:

```go
_ = e.AddRule(inmemory.Rule{
    Name: "remote-eligible",
    ConditionContext: func(ctx context.Context, in interface{}) bool {
        return remoteService.IsEligible(ctx, in.(Order).CustomerID)
    },
    Action: approve,
})
```

A nil ctx is treated as `context.Background()` for test ergonomics, but production callers should always pass a real ctx.
