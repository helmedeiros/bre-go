# 10. ListenerHost As An Optional Interface

## Status

Accepted

## Context

ADR-0007 added `observability.ExecutionListener` and said adapters fire `OnRuleMatched` themselves. The `inmemory.Engine` exposes `AddListener(observability.ExecutionListener)`; future adapters (gorules, file-loaded, anything else) are free to expose the same method, but nothing in the type system enforces that, and callers who want to register a listener generically (against an `engine.Engine`-typed variable) have to type-assert to a specific adapter.

That is the wrong layering. The *port* should declare an opt-in extension that adapters with listener support implement, so callers can detect the capability with one type assertion instead of one per adapter.

The shape choices:

1. **Embed in `engine.Engine`**: every adapter must support listeners. Wrong -- a future read-only or remote adapter might not.
2. **Separate optional interface**: callers `if host, ok := eng.(engine.ListenerHost); ok { host.AddListener(...) }`. Standard Go idiom (cf. `io.Closer`, `http.Hijacker`).
3. **Reflection / magic**: rejected, opaque to readers.

We pick (2).

## Decision

Add a small interface in the `engine` package:

```go
type ListenerHost interface {
    AddListener(observability.ExecutionListener)
}
```

`inmemory.Engine` already satisfies it without any code change -- its `AddListener` signature matches. A compile-time witness in `engine/inmemory` (`var _ engine.ListenerHost = (*Engine)(nil)`) makes the assertion explicit so a future signature drift fails the build instead of silently breaking external callers.

Adapters that do not support listeners simply do not implement the method. Callers use the standard `eng.(engine.ListenerHost)` type assertion to discover the capability.

## Consequences

The `engine` package gains a one-method interface and a dependency on `observability`. That import direction is fine -- `engine` declares the ports, and `observability` declares the listener types those ports reference. The inverse import (observability importing engine) was never on the table.

Callers can now write helper functions that accept `engine.Engine` and decide at runtime whether to register a listener, without leaking the concrete adapter type. The contract test suite does not need a new case for this -- absence of `AddListener` is a valid adapter state, so the suite stays silent on it.

Two tests pin the witness: one asserts `*inmemory.Engine` satisfies `engine.ListenerHost`, another walks the assertion end-to-end (cast, register, fire, observe).
