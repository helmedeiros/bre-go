# Contributing to bre-go

Thanks for considering a contribution. This document is intentionally short while the project is still being shaped; it will grow as conventions stabilise.

## Local setup

```sh
git clone https://github.com/helmedeiros/bre-go
cd bre-go
```

Required tools (the [`Makefile`](Makefile) will tell you what is missing once it lands later this week):

- Go 1.17 (or newer)
- [`golangci-lint`](https://golangci-lint.run/)

## Branching and commits

- Branch from `main`. Name the branch after the change or issue.
- Imperative, present-tense commit messages ("Add …", not "Added …").
- Small, atomic commits. Each one must leave the quality gates (lint + vet + tests + coverage threshold) green.
- The TDD discipline is: a failing-test commit followed by an implementation commit. Refactors are a separate commit.
- Every contribution follows the [Clean Code conventions](docs/clean-code-conventions.md). Name conflicts with idiomatic Go go through the PR; the rule wins by default.

## Architecture decisions

Significant design choices are captured as Architecture Decision Records under [`docs/architecture/decisions/`](docs/architecture/decisions/). Before adding a non-trivial cross-cutting feature, open an ADR.

## Adding a new engine adapter

The `engine.Engine` port lives in [`engine/`](engine/) and is the only thing callers depend on. Every adapter must:

1. Live in its own sub-package under `engine/`.
2. Expose a `New(...)` constructor that returns either `*Engine` (an adapter-specific type implementing `engine.Engine`) or the interface directly.
3. **Never** expose adapter-specific types (rule structs, sessions, fact handles) outside the package boundary.
4. Wire `enginetest.RunContractTests` from a `*_test.go` file with a Factory that builds a fresh engine + a SeedFunc that registers rules in the adapter's native shape. The contract suite drives the same behavioral assertions every adapter must satisfy.

See `engine/inmemory/contract_test.go` for the wiring template.

## Reporting issues

Open a GitHub issue. For security concerns, follow [SECURITY.md](SECURITY.md) and use the private advisory channel.
