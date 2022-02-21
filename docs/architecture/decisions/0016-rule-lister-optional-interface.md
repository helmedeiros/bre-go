# 16. RuleLister As An Optional Interface

## Status

Accepted

## Context

Today there is no port-level way to ask an engine "what rules are registered?". A caller holding an `engine.Engine` who wants to log the rule set at startup, expose a `/rules` debug endpoint, or sanity-check that a config-driven loader populated the engine correctly, has to type-assert to the concrete adapter (`*inmemory.Engine`, `*firstmatch.Engine`) and reach for unexported state.

That breaks ADR-0001's "swappable engine" promise. The whole point of the port is that callers can write code against the interface, not the adapter. Introspection should be available through the same shape.

Two design choices to weigh:

1. **Add `RuleNames()` to the `engine.Engine` port directly.** Every adapter must implement it. Friction for adapters that genuinely cannot introspect (a future remote/RPC-backed adapter where rules are owned by the server, for example).
2. **Add an optional capability interface.** Adapters that *can* introspect implement it. Callers discover the capability via a type assertion. Same pattern ADR-0010 used for `ListenerHost`.

Option (2) is the right precedent. Optional capability interfaces are the Go idiom for "supported by some adapters, not all" (cf. `io.Closer`, `http.Hijacker`, the existing `engine.ListenerHost`). The port stays minimal; the capability is discoverable; adapters opt in when it makes sense.

The shape choice for the method itself: return `[]string` (rule names) rather than `[]Rule` (full structs). Reasons:

- The full `Rule` struct contains funcs (`Condition`, `Action`) which carry closures the caller cannot meaningfully inspect.
- Different adapters have different `Rule` types (`inmemory.Rule`, `firstmatch.Rule`); the port cannot reference any of them without breaking the layering.
- Names are the identifier callers reason about (already established by ADR-0009's duplicate-name policy). They are exactly the introspection surface most tools want.
- A future ADR can add a richer introspection shape (e.g., `RuleMetadata{Name, Description, Tags}`) without breaking this one -- adapters would satisfy both interfaces in parallel.

## Decision

Add a one-method interface in the `engine` package, parallel to `engine.ListenerHost`:

```go
type RuleLister interface {
    RuleNames() []string
}
```

`inmemory.Engine` and `firstmatch.Engine` both implement it: each walks its `rules` slice and returns a fresh `[]string` of the names. Callers use the standard `eng.(engine.RuleLister)` type assertion to discover support.

The returned slice is a copy, not a view into the engine's internal state. Adapter contract: a `nil` or empty slice is a valid return (no rules registered); the order matches insertion order (so callers debugging a `firstmatch.Engine` see the precedence chain they configured).

A new contract case in `enginetest.RunContractTests` exercises the capability: if the engine satisfies `RuleLister`, seeding a rule named `"alpha"` should make `RuleNames()` return a slice containing `"alpha"`. Adapters that do not implement `RuleLister` auto-skip the case, same shape as the existing `ListenerHost`-aware case.

Compile-time witnesses live in each adapter's tests (`var _ engine.RuleLister = (*Engine)(nil)`), so a future signature drift breaks the build instead of silently downgrading the capability.

## Consequences

The `engine` package gains a small optional interface; the port itself is unchanged. Callers can write helpers that accept `engine.Engine` and decide at runtime whether the engine can be introspected. Adapters that cannot introspect (a future remote adapter, an adapter wrapping an opaque vendor session) are not forced into a method they cannot implement honestly.

The "names-only" return is deliberate. A richer introspection ADR can land later when a real caller needs more than names; until then, the surface stays minimal.

The pattern of "small optional capability interface + contract case that auto-skips when unsupported" is now established in two places (`ListenerHost`, `RuleLister`). Future capability additions follow the same template.
