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

## Architecture decisions

Significant design choices are captured as Architecture Decision Records under [`docs/architecture/decisions/`](docs/architecture/decisions/). Before adding a non-trivial cross-cutting feature, open an ADR.

## Reporting issues

Open a GitHub issue. For security concerns, follow [SECURITY.md](SECURITY.md) and use the private advisory channel.
