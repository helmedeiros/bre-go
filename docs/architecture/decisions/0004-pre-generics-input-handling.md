# 4. Pre-Generics Input Handling

## Status

Accepted (provisional; superseded by a follow-up ADR once Go 1.18 ships)

## Context

ADR-0003 declared the engine port with `engine.Context.Input` typed as `interface{}`. The reason was simple: Go 1.17 has no generics. The reason is also temporary: Go 1.18 ships in mid-March 2022 and brings type parameters to interfaces and functions.

Designing the public API around the temporary constraint risks two failure modes:

1. **Lock-in.** If callers grow code that depends on `interface{}` accessors -- type switches, reflect, hand-written assertion helpers -- a later switch to a generic executor becomes a breaking change rather than an additive one.
2. **Surface drift.** If the surface evolves freely while we wait for generics, the eventual generic-aware layer will not fit cleanly on top.

Both failure modes are well-documented in long-lived library codebases: every untyped-input method tends to accumulate ad-hoc callers that block any later attempt to add type parameters, turning a transitional API into a permanent compatibility burden.

## Decision

For the pre-1.18 window the engine port stays as declared:

- `engine.Context.Input interface{}` and `engine.Result.Output interface{}`.
- Adapters that need a specific type use a Go type assertion inside their `Execute`. They fail fast (return an error in ADR-0005) when the assertion does not match.

The pre-1.18 surface ships with one constraint that pre-empts the failure modes above: **no public helper that introspects `interface{}`**. No `engine.MustString(input)`, no reflect-based decoding, no JSON-via-interface. Callers either know the concrete type (and assert it themselves) or they wait for the generic executor.

When Go 1.18 ships, a follow-up ADR will:

- Bump `go.mod` to `go 1.18`.
- Introduce a typed wrapper above the existing `engine.Engine`: `Executor[In, Out]` that owns the assertion and presents typed methods to callers.
- Leave `engine.Engine` itself untyped. The port stays simple; the ergonomics live one layer up.

## Consequences

The pre-1.18 codebase is honest about its rough edges. Each adapter writes one assertion at the entry of its `Execute`; nothing pretends to be type-safe that is not.

When generics arrive, the `Executor` layer slots in as an additive feature -- no breaking change for callers already using `engine.Engine` directly. The Executor is the ergonomic surface; the port remains the load-bearing one. That separation is the point.

The "no introspection helper" rule will get tested. The first time we want to log an `interface{}` value, someone will reach for reflect. The right answer is to push that responsibility outward to the adapter or the caller, not to add it to the public surface.
