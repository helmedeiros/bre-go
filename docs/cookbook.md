# Cookbook

Realistic patterns for using `bre-go`. The [Quickstart in the README](../README.md#quickstart) covers the smallest happy-path example; this document is for everything past that.

All examples target `v0.2.0` or later. `context.Context` is the first parameter of every `Execute` call.

## Contents

- [Patterns](#patterns)
  - [Pick the right adapter](#pick-the-right-adapter)
  - [Compose conditions declaratively](#compose-conditions-declaratively)
  - [Handle cancellation and timeouts](#handle-cancellation-and-timeouts)
  - [Compose multiple listeners on one engine](#compose-multiple-listeners-on-one-engine)
  - [Introspect at runtime](#introspect-at-runtime)
  - [Handle errors from Execute](#handle-errors-from-execute)
  - [Use the typed Executor](#use-the-typed-executor)
  - [Write adapter-agnostic helpers](#write-adapter-agnostic-helpers)

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

### Compose multiple listeners on one engine

Listeners stack -- one engine can have many. Each one observes its slice of the lifecycle.

```go
e := inmemory.New()

counter := &observability.CountingListener{}                // per-rule hit counts
timing  := &observability.TimingListener{}                  // duration of the last execute
logger  := observability.NewLoggingListener(structuredLog)  // bridges to your Logger

e.AddListener(counter)
e.AddListener(timing)
e.AddListener(logger)

_, _ = e.Execute(ctx, engine.Request{Input: order})

// After Execute:
counter.Count("approve-fast")           // how many times this rule fired in total
counter.Total()                          // total matches across all rules
timing.LastDuration()                    // how long the last Execute took
timing.MatchesInLastExecution()          // how many matches in the last Execute (resets on next Started)
```

Listeners discover their capabilities through type assertions at notify time, so a plain `ExecutionListener` (only `OnRuleMatched`) and a full four-method listener can coexist on the same engine without either knowing the other exists.

For testing, `SnapshotListener` captures every lifecycle event for later assertion -- no need to hand-roll a recorder type:

```go
snap := &observability.SnapshotListener{}
e.AddListener(snap)

_, _ = e.Execute(ctx, engine.Request{Input: x})

if len(snap.Matches) != 2     { t.Fatalf(...) }
if snap.Finished[0].Duration  > deadline { t.Fatalf(...) }
if len(snap.Errored) != 0     { t.Fatalf(...) }
```

### Introspect at runtime

Adapters expose their rule set through two optional capability interfaces. Callers ask via type assertion -- the engine port itself does not require introspection.

```go
var eng engine.Engine = inmemory.New()
// ... register rules ...

// Just names (cheap):
if lister, ok := eng.(engine.RuleLister); ok {
    for _, name := range lister.RuleNames() {
        log.Printf("rule registered: %s", name)
    }
}

// Names + Description + Tags (richer; one allocation per rule):
if lister, ok := eng.(engine.RuleInfoLister); ok {
    for _, info := range lister.RuleInfos() {
        log.Printf("%s [%s]: %s", info.Name, strings.Join(info.Tags, ","), info.Description)
    }
}
```

A `/rules` debug endpoint is two lines:

```go
http.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
    if lister, ok := eng.(engine.RuleInfoLister); ok {
        _ = json.NewEncoder(w).Encode(lister.RuleInfos())
    }
})
```

Both methods return fresh copies; the caller can mutate the returned slices (or `Tags` within them) without corrupting engine state.

### Handle errors from Execute

`Execute` returns three error categories. Use `errors.Is` for sentinel comparison, `errors.As` for typed errors carrying details:

```go
res, err := eng.Execute(ctx, engine.Request{Input: x})
switch {
case errors.Is(err, context.Canceled):
    // caller cancelled the request
case errors.Is(err, context.DeadlineExceeded):
    // request timed out
case isActionPanic(err):
    // a rule's Action panicked; the rule name is in the typed error
    var pe *inmemory.ActionPanicError
    _ = errors.As(err, &pe)
    log.Printf("rule %q action panicked: %v", pe.RuleName(), pe.Value)
case err != nil:
    // unexpected
    log.Printf("execute: %v", err)
}

func isActionPanic(err error) bool {
    var pe *inmemory.ActionPanicError
    return errors.As(err, &pe)
}
```

Note: each adapter has its own `ActionPanicError` type (`inmemory.ActionPanicError`, `firstmatch.ActionPanicError`, `priority.ActionPanicError`). They all expose `RuleName() string`, so callers that need to handle panics across adapters can define a small local interface:

```go
type ruleNamer interface{ RuleName() string }
if rn, ok := err.(ruleNamer); ok {
    log.Printf("rule %q failed", rn.RuleName())
}
```

`Result.Matched` still contains the panicking rule's name -- the rule *did* match its condition; the action is what failed. `OnExecutionErrored` fires for the panicking rule; `OnRuleMatched` does not (the match event is reserved for successful Action completion).

### Use the typed Executor

`engine/exec.Executor[In, Out]` wraps any `engine.Engine` and hides the `interface{}` cast at the call boundary. The underlying adapter does not change:

```go
import "github.com/helmedeiros/bre-go/engine/exec"

type Decision string
type Order struct { Amount int; Currency string }

eng := inmemory.New()
// ... register rules whose Action returns Decision ...

ex := exec.New[Order, Decision](eng)
decision, matched, err := ex.Execute(ctx, Order{Amount: 250, Currency: "USD"})
// decision is typed Decision -- no type assertion at the call site
```

If a rule's Action returns a value not assignable to `Out`, `Execute` returns an `*exec.OutputTypeMismatchError` carrying the expected and actual type names:

```go
var mismatch *exec.OutputTypeMismatchError
if errors.As(err, &mismatch) {
    log.Printf("expected %s, got %s", mismatch.Expected, mismatch.Got)
}
```

When no rule matches (or no matching rule had an action), the zero value of `Out` is returned with a nil error -- "no decision" is not a mismatch.

### Write adapter-agnostic helpers

Code that accepts `engine.Engine` works with every shipped adapter (and any future one) without modification. This is the whole point of the port abstraction from ADR-0003.

```go
// Backend-agnostic helper. Works with inmemory, firstmatch, priority,
// and any future adapter that satisfies engine.Engine.
func evaluateAll(ctx context.Context, eng engine.Engine, inputs []Order) (approved, rejected int, err error) {
    for _, in := range inputs {
        res, ferr := eng.Execute(ctx, engine.Request{Input: in})
        if ferr != nil {
            return approved, rejected, ferr
        }
        if res.Output == "approve" {
            approved++
            continue
        }
        rejected++
    }
    return approved, rejected, nil
}
```

To opt into optional capabilities, type-assert at call time:

```go
func describeEngine(eng engine.Engine) {
    if lister, ok := eng.(engine.RuleInfoLister); ok {
        log.Printf("loaded %d rule(s)", len(lister.RuleInfos()))
    }
    if _, ok := eng.(engine.ListenerHost); ok {
        log.Printf("supports listeners")
    }
}
```

This pattern lets a caller wire a single `evaluateAll` once and ship code that:

- Works with `engine/inmemory` for tests and dev.
- Works with `engine/priority` once rules are loaded from a config file.
- Will work with a future `engine/gorules` adapter when GoRules Zen ships -- no change to `evaluateAll`.

If your helper needs a richer capability than the bare `engine.Engine` port provides, take the capability interface in the signature instead:

```go
func auditRuleSet(lister engine.RuleInfoLister) error { ... }
```

Callers pass the engine (it satisfies the interface implicitly via the type assertion the compiler does at the call site).
