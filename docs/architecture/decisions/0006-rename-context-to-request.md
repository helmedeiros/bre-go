# 6. Rename engine.Context To engine.Request

## Status

Accepted

## Context

ADR-0003 introduced the engine port with `Execute(ctx Context) (Result, error)`. The type was named `Context` because it "carries the context of an execution". On its own that read fine.

Reading it next to `context.Context` from the standard library is a different story. Two distinct types named the same word, both alive at the same call site:

```go
import (
    stdctx "context"
    "github.com/helmedeiros/bre-go/engine"
)

func handle(ctx stdctx.Context, req engine.Context) (engine.Result, error) {
    // which ctx is which?
}
```

The Clean Code rule of "meaningful distinction" rejects this. Two different concepts (cancellation/deadline plumbing vs. rule-input payload) read identically here. The aliasing required to disambiguate (`stdctx`) is itself a smell.

## Decision

Rename `engine.Context` to `engine.Request`. The pairing reads naturally:

- `Request` carries what the caller wants evaluated.
- `Result` carries what the engine produced.

`Execute(req Request) (Result, error)`.

The public surface is otherwise unchanged. The field name `Input` stays as-is. The rename happens in one commit covering the port, the in-memory adapter, the contract suite, and the existing test fixtures.

This is a breaking change to the port. The repo has no external callers; the cost is one commit.

## Consequences

`engine.Request` and `stdlib.context.Context` are now meaningfully different at every call site. Imports no longer need aliases.

A future ADR that wants to wire `context.Context` into the port (cancellation, deadlines, tracing) can do so cleanly: `Execute(ctx context.Context, req Request)`. The name space no longer overlaps.

The reference to `Context` in ADRs 0003, 0004, and 0005 is preserved as historical text -- a footnote points at this ADR. The rule is: the architecture log reads as a sequence of choices, not a rewrite.
