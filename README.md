# bre-go

A Go business rule engine with a swappable engine port.

The public API is backend-agnostic. Today it ships with a small in-memory engine for tests and examples; the long-term goal is to plug in a mature open-source rule engine behind the same interface so callers never have to change their code.

## Status

Early. The architecture is being built first, the engine implementations follow. See [`docs/architecture/decisions/`](docs/architecture/decisions/) for the design record.

## License

[MIT](LICENSE).
