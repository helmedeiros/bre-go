# 13. Generic Executor Layer Over engine.Engine

## Status

Accepted

## Context

ADR-0004 pinned the engine port to `interface{}` because Go 1.17 has no generics. The trade-off was explicit: pay one type assertion per condition/action in exchange for an `Engine` interface every adapter can implement today.

Go 1.18 is the first stable release with generics (type parameters). Once it ships, two design questions reopen:

1. **Does the `engine.Engine` port itself become generic?**
2. **Or does a separate layer wrap the existing port with type-safe ergonomics?**

The answer is (2), for two reasons:

- A generic `engine.Engine[In, Out]` would force every adapter (gorules, file-loaded, future remote) to commit to a specific `In`/`Out` pair at the package level. That breaks the "one adapter, many rule shapes" use case: today a single `inmemory.Engine` instance can hold rules whose conditions read strings and rules whose conditions read ints. With generics on the port itself, the engine instance is locked to a single shape.
- A wrapper layer can be written *and tested* against the existing port without breaking any caller. Adapters do not change. The wrapper is opt-in.

The shape this ADR proposes:

```go
// engine/exec/exec.go (new sub-package)
type Executor[In, Out any] struct {
    inner engine.Engine
}

func New[In, Out any](inner engine.Engine) *Executor[In, Out]

func (e *Executor[In, Out]) Execute(in In) (Out, []string, error)
```

`Executor` wraps any `engine.Engine`, hides the `interface{}` cast at the boundary, and returns a typed result. The rule registration side stays on the underlying adapter -- `inmemory.AddRule` still takes `func(interface{}) bool`. A typed builder on top of that comes in a follow-up ADR if it earns its keep.

## Decision

Add an `engine/exec` sub-package with the wrapper shape above. The existing `engine.Engine` port and every adapter stay untyped. Tests for `Executor` live in `engine/exec` and re-use the inmemory adapter as the underlying engine.

`Execute` returns `(Out, []string, error)`. The error path covers two distinct cases the caller branches on:

- The underlying engine returned an error (panic recovery, future validation failures). Pass it through unchanged.
- The engine produced an `Output` that is not assignable to `Out`. Surface as `*OutputTypeMismatchError` carrying the expected and actual type names. This catches the realistic mistake where a caller wires up an `Executor[Input, string]` over an engine whose actions return ints.

A `nil` Output (no rule had an action, or no rule matched) returns the zero value of `Out` and a `nil` error -- this is not a mismatch, it is "no decision".

## Consequences

The pre-1.18 `interface{}` port keeps every adapter untyped, so adapters that need to host rules of mixed shapes (one rule reading strings, another reading ints) can continue to do so. Callers that want type safety for their *own* call sites wrap with `Executor[In, Out]` and pay one type assertion at the boundary, exactly as designed.

Companion commits land together with this ADR's implementation:

- `go.mod` bumps from `go 1.17` to `go 1.18`.
- CI's `setup-go` bumps to `1.18`.
- ADR-0004's status changes to `Superseded by ADR-0013` with a forward-link callout at the top of its body, per ADR-0011's three-edit discipline.

ADR-0004's underlying decision is not reversed: the engine port stays untyped. What is superseded is the implicit "callers carry the `interface{}` cost permanently" framing -- the typed escape hatch now exists.
