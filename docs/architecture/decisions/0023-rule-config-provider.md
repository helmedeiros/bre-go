# 23. RuleConfigProvider: Decouple Matchers From Loading

## Status

Accepted — landed as the additive engine.RuleConfig + engine.RuleConfigProvider + engine.Load surface. Ships with v0.3.0 alongside the engine/csv concrete provider (ADR-0024).

## Context

Today every adapter accepts rules through `AddRule(Rule)` calls from in-code construction. That works for tests, examples, and the simplest production cases, but it does not work for the use cases this library is actually aimed at:

1. **Decision tables loaded from a config file** -- rules live in a CSV / JSON / YAML file, not in compiled Go code. Adding a new row to the file should not require a recompile.
2. **Modular rule sets composed from multiple sources** -- a base set of rules plus tenant-specific overrides plus environment guards, each loaded from its own source, composed at startup.
3. **Hot-reloadable rule sets** (future, not this ADR) -- the engine reads from a provider that can change underneath it.

All three need a layer between the matcher (the adapter) and the *origin* of the rule data. The matcher should not know whether rules came from a CSV file, a database, an HTTP service, or a hard-coded slice. It should ask a *provider* for the current list of rule configurations.

The shape that works:

```go
type RuleConfig interface {
    RuleName() string
}

type RuleConfigProvider[RC RuleConfig] interface {
    RuleConfigs() ([]RC, error)
}
```

`RuleConfig` is the minimum contract any rule's configuration must satisfy: a name. Concrete implementations carry whatever fields the rule needs (`Amount int`, `Currency string`, `Priority int`, etc.). The generic parameter lets each adapter / loader work with its own typed `RC` -- no `interface{}` boxing at this layer (we are post-1.18; ADR-0013).

`RuleConfigProvider` is the single-method interface every loader satisfies. A CSV loader, a JSON loader, an in-memory slice, an HTTP client -- all return `[]RC` (or an error if the underlying source is malformed). The interface is intentionally minimal: no `Reload()`, no `Watch()`, no `Close()`. Those land in later ADRs if they earn their keep.

Three design choices to weigh:

**1. Where do `RuleConfig` and `RuleConfigProvider` live?**

- (a) In the `engine` package, alongside `Engine` and `RuleInfo`. Promotes them as port-level concepts.
- (b) In a new `engine/load` (or `engine/loader`) sub-package. Keeps the `engine` package surface minimal.

Pick (a). `RuleConfig` is logically part of the port's vocabulary -- it describes what callers feed to adapters via loaders. A future `engine/csv`, `engine/json`, and any caller-defined loader all reference `engine.RuleConfigProvider`. Hiding it behind another package import path adds friction for marginal gain.

**2. Does the adapter use the provider directly, or does the caller mediate?**

- (a) Adapter has a `LoadFrom(provider engine.RuleConfigProvider[RC]) error` method that pulls rules from the provider and calls its own internal AddRule equivalent.
- (b) Caller calls `provider.RuleConfigs()`, then loops and calls `adapter.AddRule(...)` for each.

Pick (b). The adapter does not need to know about providers; it just needs `AddRule`. The caller (or a small wiring helper) bridges from provider to adapter. This keeps the engine port minimal -- no new method per adapter -- and lets callers compose providers (concat two providers' outputs before feeding the adapter) without the adapter being involved.

A small `engine.Load` helper function lives in the `engine` package:

```go
func Load[RC RuleConfig](provider RuleConfigProvider[RC], add func(RC) error) error {
    configs, err := provider.RuleConfigs()
    if err != nil {
        return err
    }
    for _, c := range configs {
        if err := add(c); err != nil {
            return err
        }
    }
    return nil
}
```

`add` is whatever the caller wires up (typically a closure over `adapter.AddRule`). The bridging stays explicit at the call site; nothing magical happens inside `Load`.

**3. How does the typed `RuleConfig` map to each adapter's `Rule` struct?**

- (a) `RuleConfig` IS the adapter's `Rule` -- unify them.
- (b) `RuleConfig` is a caller-defined struct; the caller writes a mapping function to convert each `RuleConfig` into an `adapter.Rule`.

Pick (b). The two types serve different purposes:

- `RuleConfig`: raw data from the source (CSV row, JSON object). May carry strings, ints, source-line numbers, comments.
- `adapter.Rule`: the engine-ready rule with `Condition` / `Action` funcs bound.

A mapping step (e.g., `func toRule(cfg MyConfig) inmemory.Rule`) lives in caller code and is where the source-data → engine-funcs translation happens. The mapping is testable in isolation.

## Decision

Add two new port-level types to the `engine` package:

```go
type RuleConfig interface {
    RuleName() string
}

type RuleConfigProvider[RC RuleConfig] interface {
    RuleConfigs() ([]RC, error)
}
```

Plus a small generic helper:

```go
func Load[RC RuleConfig](provider RuleConfigProvider[RC], add func(RC) error) error
```

No adapter package changes. No engine.Engine interface changes. The new types are additive and opt-in: callers using in-code `AddRule` continue to work unchanged.

A test in the `engine` package proves the shape: declare a tiny `testRuleConfig` struct implementing `RuleConfig`, an in-memory `sliceProvider` implementing `RuleConfigProvider`, and verify `Load(provider, add)` calls `add` once per config in insertion order.

## Consequences

The engine package gains two interfaces and one generic helper -- ~30 lines of code. The port surface grows to include the loader vocabulary. Callers building their first loader follow this shape:

```go
// Caller-defined config struct
type OrderRuleConfig struct {
    Name      string
    Amount    int
    Currency  string
    Action    string
}

func (c OrderRuleConfig) RuleName() string { return c.Name }

// Caller-defined provider (or use a future engine/csv loader)
type FileProvider struct { ... }
func (f *FileProvider) RuleConfigs() ([]OrderRuleConfig, error) { ... }

// Caller wires it
provider := &FileProvider{Path: "rules.csv"}
adapter := inmemory.New()
err := engine.Load(provider, func(c OrderRuleConfig) error {
    return adapter.AddRule(toInmemoryRule(c))
})
```

The future `engine/csv` adapter (ADR-0024) implements `RuleConfigProvider` for CSV files specifically. The future `engine/json` (ADR not yet written) does the same for JSON.

`RuleConfigProvider` not having a `Reload()` or `Watch()` method is deliberate. Hot-reload is a separate concern with its own complexity (rule-set diffing, transactional updates, listener notification). When a real caller needs it, a follow-up ADR adds `engine.WatchableProvider[RC]` as an optional capability interface, paralleling how `ListenerHost` and `RuleLister` work today.
