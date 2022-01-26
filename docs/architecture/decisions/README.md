# Architecture Decision Records

Each file in this folder captures one architecture decision made on the bre-go codebase, following the standard ADR shape (Status / Context / Decision / Consequences).

New decisions get the next number and a short kebab-case slug:

```
NNNN-short-decision-name.md
```

A decision is "Accepted" when its implementation is on `main`. Older decisions can be marked "Superseded by ADR-MMMM" and kept in place so the history of the codebase reads as a sequence of choices.

## Index

- [ADR-0001 Bounded Goals For This Project](0001-bounded-goals.md)
- [ADR-0002 Go As The Implementation Language](0002-go-as-the-language.md)
- [ADR-0003 The Engine Port](0003-engine-port.md)
- [ADR-0004 Pre-Generics Input Handling](0004-pre-generics-input-handling.md)
- [ADR-0005 engine.Engine.Execute Returns An Error](0005-engine-execute-returns-error.md)
- [ADR-0006 Rename engine.Context To engine.Request](0006-rename-context-to-request.md)
- [ADR-0007 Execution Listener As An Observer Port](0007-execution-listener.md)
- [ADR-0008 Built-In Listener Implementations](0008-built-in-listeners.md)
- [ADR-0009 Reject Duplicate Rule Names On AddRule](0009-reject-duplicate-rule-names.md)
