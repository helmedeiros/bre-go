# 25. Multi-Source Rule Composition

## Status

Accepted — landed as the additive engine.ChainProviders helper. Ships with v0.4.0 alongside ADR-0026 (correlation-ID propagation).

## Context

ADR-0023 defined `engine.RuleConfigProvider[RC]` as the single-source loader contract. One provider returns one source's worth of rule configs. Two real-world patterns immediately push past that:

1. **Defaults plus tenant overrides.** A baseline rule set ships with the application; per-tenant rules layer on top. Each source has its own provider; the engine sees the union.
2. **Modular composition.** A "compliance" rule file plus a "pricing" rule file plus an "experiments" rule file -- each owned by a different team, each loaded separately, all evaluated as one rule set.

Both want the same primitive: combine multiple `RuleConfigProvider[RC]` instances into one. The caller does not want to write a manual loop that calls each provider in turn, concatenates the slices, and threads errors back. That's repetitive plumbing that belongs in the library.

Two design questions:

**1. Should composition be a function or a type?**

- (a) Function: `func ChainProviders[RC RuleConfig](providers ...RuleConfigProvider[RC]) RuleConfigProvider[RC]`. Returns an anonymous provider that delegates.
- (b) Type: `type ChainProvider[RC RuleConfig] struct{ ... }` with a constructor and a method.

Pick (a). The composed provider has no state to mutate after construction; a function returning an inline-implementing struct is enough. Callers who want to chain dynamically (build providers, append, evaluate) can do that themselves in a slice and call `ChainProviders(slice...)` once. No reason to expose a builder API.

**2. How does the chain handle errors?**

Each provider can return `(nil, error)`. The chain has three options:

- (a) **First-error-wins**: short-circuit on the first provider error; return that error.
- (b) **Collect-all**: keep going, accumulate every provider's error into a multi-error, return the aggregate.
- (c) **Partial-success**: skip providers that error, return the successful concatenation.

Pick (a). The first-error semantic matches how `engine.Load` already short-circuits on `add` errors. Two providers erroring usually means two related problems (e.g., a directory unreadable, multiple files affected); the first one is enough signal. Callers wanting "collect all errors" can build their own combinator on top.

**3. What about composition operators beyond chain?** Filter, map, dedup, sort, group-by-tag... these are tempting. None are in this ADR. `ChainProviders` is the one composition operator the parity target uses (the others can be done in the caller's bridging closure inside `engine.Load`). When a real caller asks for more, follow-up ADRs add them as targeted helpers, not as a general "stream" abstraction.

## Decision

Add to the `engine` package:

```go
// ChainProviders combines multiple providers into one. The returned
// provider's RuleConfigs concatenates each source's output in the
// order given. The first provider error short-circuits the rest.
func ChainProviders[RC RuleConfig](providers ...RuleConfigProvider[RC]) RuleConfigProvider[RC]
```

Implementation is roughly 15 lines: a private type wrapping the slice, with a `RuleConfigs()` method that walks the slice and concatenates.

A `ChainProviders()` with zero arguments returns an empty provider (returns `nil, nil` from `RuleConfigs`). Same identity-element shape as `conditions.And()` and `conditions.Or()`.

A test file in the `engine` package proves four behaviors with one assertion each: zero providers returns empty, one provider returns its configs unchanged, two providers concatenate in order, a provider error short-circuits with the same error returned.

## Consequences

The `engine` port grows by one exported function. No interface changes. No adapter changes. Existing callers using a single provider see no API change; the chain is purely additive.

Common wiring becomes:

```go
defaults := csv.NewLoader[TierConfig]("defaults.csv", parseTier)
tenant   := csv.NewLoader[TierConfig](tenantPath, parseTier)
combined := engine.ChainProviders(defaults, tenant)

err := engine.Load[TierConfig](combined, func(c TierConfig) error {
    return eng.AddRule(toRule(c))
})
```

The cookbook gains a "Compose rules from multiple sources" section showing this exact pattern alongside a tenant-override variation.

A future ADR for filtering (e.g., "load only rules with tag X") or for hot-reloadable providers (e.g., "wrap a Loader so it re-reads on a signal") can add corresponding helpers without revisiting this ADR. The minimal compose primitive does not foreclose on richer operators.
