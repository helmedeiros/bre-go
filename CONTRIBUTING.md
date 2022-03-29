# Contributing to bre-go

Thanks for considering a contribution. This document is intentionally short while the project is still being shaped; it will grow as conventions stabilise.

## Local setup

```sh
git clone https://github.com/helmedeiros/bre-go
cd bre-go
```

Required tools (the [`Makefile`](Makefile) will tell you what is missing once it lands later this week):

- Go 1.18 (or newer; required since the `engine/exec` generic wrapper landed)
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

The `engine.Engine` port lives in [`engine/`](engine/) and is the only thing callers depend on. The repo ships two adapters today -- [`engine/inmemory`](engine/inmemory) (all-match, last-action-wins) and [`engine/firstmatch`](engine/firstmatch) (first-match) -- both produced by the same four-step recipe:

1. **Live in your own sub-package under `engine/`.** Pick a name that describes the *policy*, not the implementation (`inmemory` was the wrong precedent here; `firstmatch` is the right one. Names like `decisiontable` or `priorityqueue` would also be good).
2. **Expose a `New(...) *Engine` constructor and an adapter-local `Rule` type.** The `Rule` type lives in the adapter package, not in `engine/`. The port stays minimal (see ADR-0014's rationale).
3. **Validate at registration time, not at execution time.** Empty name, nil condition, duplicate name -- return distinct sentinels callers can branch on with `errors.Is`. The check order is shape-first (per-rule invariants) then state-second (uniqueness), so error returns stay deterministic regardless of registration order. See ADR-0009, ADR-0012.
4. **Wire `enginetest.RunContractTests`** from a `*_test.go` file with a Factory that builds a fresh engine + a SeedFunc that registers rules in the adapter's native shape. The contract suite drives the port-level behavioral assertions every adapter must satisfy.

**Optional but recommended:**

- **Satisfy `engine.ListenerHost`** if your adapter can fire per-rule events. The two existing adapters both do; the `observability.CountingListener` and `LoggingListener` plug in automatically. A compile-time witness (`var _ engine.ListenerHost = (*Engine)(nil)`) catches signature drift.
- **Satisfy `engine.RuleLister`** if your adapter can enumerate its rule set. Return a fresh `[]string` in insertion order so mutating the slice does not affect engine state. A compile-time witness mirrors the `ListenerHost` one. See ADR-0016.
- **Satisfy `engine.RuleInfoLister`** if your `Rule` carries metadata callers might want to introspect. Return a fresh `[]engine.RuleInfo` in insertion order; mapping `Name`/`Description`/`Tags` from your adapter's local `Rule` struct is enough. See ADR-0020.
- **Recover panicking actions** in `Execute`. Surface them as a typed `ActionPanicError` local to your adapter (carrying `Rule` and `Value`, with a `RuleName()` accessor) and notify any `observability.ExecutionErroredListener` before returning the partial `Result` + non-nil error. See ADR-0018.

Adapters automatically work with the generic `engine/exec.Executor[In, Out]` wrapper -- the wrapper sits over `engine.Engine.Execute` and does not call into adapter internals. No extra wiring needed.
- **Add a runnable example** in `example_test.go` showing the adapter's headline use case. Compile-checked godoc beats prose every time.

See `engine/inmemory/contract_test.go` and `engine/firstmatch/contract_test.go` for the wiring template -- both files are deliberately near-identical.

## Reporting issues

Open a GitHub issue. For security concerns, follow [SECURITY.md](SECURITY.md) and use the private advisory channel.
