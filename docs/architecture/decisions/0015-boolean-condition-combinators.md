# 15. Boolean Condition Combinators

## Status

Accepted

## Context

Today every `Rule.Condition` is a single `func(interface{}) bool` the caller writes. Combining conditions ("amount > 100 AND currency == USD AND NOT blacklisted") requires the caller to hand-write a wrapper function for each rule. That is the right minimum for the engine port -- it gives every adapter the same shape -- but it pushes a tedious boilerplate onto rule authors and means each composition lives at a different call site.

The standard answer in rule engines is a small Boolean combinator library: `And`, `Or`, `Not`, plus the two sentinels `Always` and `Never`. Adding them to bre-go means rule definitions become declarative trees that any reader can scan top to bottom:

```go
// Today:
e.AddRule(inmemory.Rule{
    Name: "high-value-non-blacklisted",
    Condition: func(in interface{}) bool {
        d := in.(decision)
        return d.amount > 100 && d.currency == "USD" && !d.blacklisted
    },
})

// With combinators:
e.AddRule(inmemory.Rule{
    Name: "high-value-non-blacklisted",
    Condition: conditions.And(
        amountOver(100),
        currencyEquals("USD"),
        conditions.Not(blacklisted),
    ),
})
```

The Why: each leaf predicate (`amountOver`, `currencyEquals`, `blacklisted`) is now individually testable, individually reusable across rules, and named. The composition itself reads in English order.

The shape choice: combinators take `func(interface{}) bool` predicates and return `func(interface{}) bool`. That matches `Rule.Condition`'s type exactly, so the combinators drop into any adapter's `AddRule` call without conversion. No new interface, no `Predicate` named type that would force callers to wrap their existing funcs.

Variadic vs. binary: `And(a, b, c, d)` is more ergonomic than `And(And(And(a, b), c), d)`. We pick variadic. `Not` is unary. Zero-argument `And()` is the identity for conjunction (true); zero-argument `Or()` is the identity for disjunction (false). These edge cases are pinned by tests so the algebra is well-defined.

## Decision

Add `engine/conditions` as a sibling sub-package to the existing adapters. It exports:

```go
// Boolean combinators
func And(preds ...func(interface{}) bool) func(interface{}) bool
func Or(preds ...func(interface{}) bool) func(interface{}) bool
func Not(pred func(interface{}) bool) func(interface{}) bool

// Sentinel predicates
func Always() func(interface{}) bool   // returns the constant-true predicate
func Never() func(interface{}) bool    // returns the constant-false predicate
```

The package depends on nothing internal -- not `engine`, not the adapters. It is a pure utility that returns functions of the right shape. The two adapters and the contract suite stay unchanged; combinators are entirely additive and opt-in.

Short-circuit evaluation: `And` returns false on the first false predicate without calling later ones. `Or` returns true on the first true predicate. The order is the argument order. This matters when predicates have observable cost (a DB lookup) or side effects (a logger) -- though side-effectful predicates are an anti-pattern this ADR does not endorse.

## Consequences

Rule authors get a declarative composition tool without the engine port changing. Both `inmemory.Engine` and `firstmatch.Engine` accept combinator-produced conditions today, no per-adapter work needed -- the type matches `Rule.Condition` exactly.

The package surface is small: 3 combinators + 2 sentinels = 5 exported functions. Nothing here forecloses on the post-Go-1.18 generics design (ADR-0013): a future generic `conditions[T]` package can sit next to this one and offer typed variants without invalidating this one. The current package would then likely transition to **Superseded by** the generic version, or stay as the `interface{}` floor that the generic layer wraps.

A new test helper (`conditions/conditions_test.go`) pins each combinator's truth table and short-circuit behavior with one assertion per case.
