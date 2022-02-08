# bre-go

A Go business rule engine with a swappable engine port.

The public API is backend-agnostic. Today it ships with two small in-process engines for tests, examples, and lightweight production use; the long-term goal is to plug a mature open-source rule engine in behind the same interface so callers never have to change their code.

## Status

Early. The architecture is being built first, the engine implementations follow. See [`docs/architecture/decisions/`](docs/architecture/decisions/) for the design record.

## Which adapter do I want?

| Adapter | Semantics | Pick it when |
|---------|-----------|--------------|
| [`engine/inmemory`](engine/inmemory) | Evaluate every rule; last matching action wins on `Output`; every match appears in `Matched`. | You want all decisions a rule set produces, accumulate counts via a listener, or run a "every rule should fire if applicable" policy. |
| [`engine/firstmatch`](engine/firstmatch) | Evaluate in insertion order; return on the first matching rule. Later rules are never evaluated and their actions never run. | You have a decision table, a content classifier, or a "first applicable rate" policy where rule order encodes precedence. |

Both adapters share the same `Rule` shape (`Name`, `Condition`, `Action`), the same registration validation (`ErrEmptyRuleName`, `ErrNilCondition`, `ErrDuplicateRuleName`), and both satisfy `engine.ListenerHost` so the observability built-ins (`CountingListener`, `LoggingListener`) attach to either with `e.AddListener(...)`.

The same `enginetest.RunContractTests` suite runs against both -- port-level behavior is identical, only the multi-rule policy differs.

## License

[MIT](LICENSE).
