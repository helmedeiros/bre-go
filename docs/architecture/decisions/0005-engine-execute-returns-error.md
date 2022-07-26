# 5. engine.Engine.Execute Returns An Error

## Status

Accepted

## Context

ADR-0003 declared `engine.Engine.Execute(ctx Context) Result`. The signature has no failure channel: an engine that cannot evaluate the input (malformed rule, type assertion failure, transient backend error) has to either panic, return a zero `Result` indistinguishable from "no match", or smuggle an error inside `Result` via a side field.

All three options have failed elsewhere:

- Panic crosses a package boundary the language asks us to be explicit about. Library code that panics on bad input forces every caller to wrap calls in `recover` and reverse-engineer the panic shape.
- Zero `Result` from a failed evaluation looks identical to a successful "no rule matched" outcome. Production logs cannot distinguish the two.
- An `Error` field inside `Result` works in some languages but in Go the idiomatic shape is the second return value. Smuggling errors loses the `errors.Is`/`errors.As` plumbing the standard library is built around.

The pre-1.18 design also makes type assertions inside adapters very likely (ADR-0004). A failed assertion is a normal failure mode the port must be able to signal.

## Decision

Change the port signature to:

```go
type Engine interface {
    Execute(ctx Context) (Result, error)
}
```

Adapters return:

- `(Result{...}, nil)` for a successful evaluation (including "no rule matched" -- the result is well-defined, just empty).
- `(Result{}, err)` for any failure that the engine cannot internally handle. Adapters wrap engine-specific errors with `%w` so callers can inspect with `errors.Is` and `errors.As`.

The contract test grows two cases:

- An adapter that supports it returns an error when given an input it cannot evaluate.
- A successful evaluation with no match returns `(Result{}, nil)` -- not an error.

This is a breaking change to the port. It costs one commit; no external users exist yet to break.

## Consequences

The error channel is real for any backend adapter that wraps an external engine: such engines can return parse errors, evaluation errors, or resource-exhaustion errors. They surface through the port unmangled.

The in-memory adapter rarely has anything to fail on, but it returns `nil` errors consistently so callers writing engine-agnostic code can rely on the signature.

A future ADR may introduce a separate "validation error" sentinel set (engine-agnostic) so callers can distinguish "this input was bad" from "the engine itself failed". For now the single `error` return is enough; specialisation arrives when the first non-trivial adapter needs it.

> Note: the type named `Context` in this ADR was renamed to `Request` -- see [ADR-0006](0006-rename-context-to-request.md). The decision itself is unchanged; only the spelling.
