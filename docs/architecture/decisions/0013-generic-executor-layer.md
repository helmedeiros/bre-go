# 13. Generic Executor Layer Over engine.Engine

## Status

Proposed — pending Go 1.18 GA (currently scheduled for March 15, 2022). Will transition to **Accepted** in the same commit that bumps `go.mod` to 1.18 and lands the implementation. If 1.18 slips past Q2 2022, this ADR is revisited.

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

## Decision (provisional, pending 1.18 GA)

Add an `engine/exec` sub-package once Go 1.18 ships, with the wrapper shape above. The existing `engine.Engine` port and every adapter stay untyped. Tests for `Executor` live in `engine/exec` and re-use the inmemory adapter as the underlying engine.

When this lands:

- ADR-0004 transitions to **Superseded by ADR-0013** for the part that says "callers carry the cost permanently". The `interface{}` port stays, but the typed escape hatch exists.
- `go.mod` goes from `go 1.17` to `go 1.18`.
- CI's `setup-go` matrix grows to `[1.17, 1.18]` for one release cycle to catch regressions on the floor, then drops 1.17.

## Consequences

Two consequences worth flagging while this ADR sits in Proposed:

1. **Nothing in `main` should pre-empt the wrapper shape.** No `interface{}`-heavy helpers in `engine/inmemory` that would become awkward to retrofit a generic wrapper on top of. The existing `Rule` shape is fine; the wrapper sits *over* `Engine.Execute`, not inside the adapter.
2. **ADR-0004's `Superseded by ADR-0013` edit is deliberately deferred.** It moves in the same commit as the implementation, per ADR-0011's three-edit discipline. Until then, ADR-0004 stays Accepted (provisional) and ADR-0013 stays Proposed. The status table makes both visible.

If the 1.18 release slips or the wrapper turns out to be unnecessary in real caller code, this ADR transitions to **Deprecated** rather than Superseded -- no replacement, the `interface{}` port simply remains the answer.
