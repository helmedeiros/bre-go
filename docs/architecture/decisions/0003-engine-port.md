# 3. The Engine Port

## Status

Accepted

## Context

ADR-0001 commits the project to a backend-agnostic public API. The most direct way to fail at that goal would be to model the public surface around whichever engine ships first, then discover later that swapping engines is a rewrite. The opposite mistake is to design an over-abstract interface that no real engine can implement cleanly.

A common anti-pattern in BRE codebases is exposing engine-shaped types directly on the public surface -- sessions, fact handles, base classes -- under the assumption that one engine will always be the implementation. That works until the team wants to evaluate an alternative (an external rule service, OPA, a JSON-Logic evaluator) and discovers the supposed abstraction is a thin name change over the original engine, not a real port. Every later attempt to swap engines becomes a rewrite of the integration surface.

The lesson: an engine port is only worth the name if it is defined first, **before** picking an implementation, and if every engine integration is treated as an adapter behind it.

## Decision

Adopt a hexagonal port-and-adapters layout. The public surface is a small interface in package `engine`:

```go
package engine

type Engine interface {
    Execute(ctx Context) Result
}
```

`Context` and `Result` are plain value types defined in this package. They carry whatever the caller and the engine agreed on -- inputs, outputs, matched-rule metadata -- but they are owned by us, not by any engine vendor.

Implementations of `Engine` live in sub-packages:

- `engine/inmemory` -- a trivial implementation used for tests and examples. The first one we will write.
- `engine/gorules` -- the production-target adapter, planned for adoption when GoRules Zen ships a public release (mid-2023 expected).
- `engine/<future>` -- any later backend that satisfies the same `Engine` interface.

Each adapter is a separate Go package importing `engine` and translating between our `Context`/`Result` and the underlying engine's types. **Adapter types never leak across the package boundary.** A caller importing `engine/gorules` gets a constructor that returns `engine.Engine`, not a GoRules-specific type.

Until Go generics arrive (1.18, ~March 2022), `Context` carries inputs as `interface{}` with type-checked accessors. ADR-0004 will retrofit a parameterised executor on top of the same `Engine` interface; the interface itself stays untyped, the executor layer above it gains generics.

## Consequences

The port being defined before any implementation forces the first commits of real code to be:

1. The interface declaration and value types in `engine/`.
2. The contract test suite that every implementation must satisfy.
3. The first trivial implementation (in-memory) used to prove the interface is implementable.

Only after that do we evaluate concrete backends. When GoRules Zen ships, the adapter is a single new package; nothing in the public API or in caller code needs to change. The same is true for any later backend.

The "no engine type on the public surface" rule is load-bearing. A future PR that adds a constructor returning `*gorules.Session` (instead of `engine.Engine`) breaks ADR-0001's promise and must be rejected, even if the diff looks small.

The cost of the port is one indirection per call. Measured against the value of being able to evaluate engines without rewriting callers, it pays for itself many times over.

> Note: the type named `Context` in this ADR was renamed to `Request` -- see [ADR-0006](0006-rename-context-to-request.md). The decision itself is unchanged; only the spelling.
