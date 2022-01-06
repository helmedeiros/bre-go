# 1. Bounded Goals For This Project

## Status

Accepted

## Context

"Business Rule Engine" is a broad term. Existing tooling (GoRules Zen when it appears, OPA, JSON Logic, and others in adjacent ecosystems) covers a wide spectrum: from runtime pattern-matching engines with their own DSLs to embedded condition evaluators driven by JSON. The danger of starting another BRE without a constrained scope is producing a thin wrapper around someone else's engine that adds dependency without value.

A common operational lesson across BRE deployments shapes the goals below: the engine itself is the smallest part of the problem; the integration surface, observability, and ability to swap engines are the parts that hurt later.

## Decision

The project commits to these goals, in this order of priority:

1. **A backend-agnostic public API.** Callers depend on interfaces defined in `bre-go`, never on the underlying rule engine's types. Swapping engines is a wiring change, not a refactor.
2. **Strong observability of every execution.** Latency, matched rules, input/output snapshots, and structured logs travel together so a production issue is debuggable from the log line alone.
3. **TDD-first.** Every behaviour change ships as a failing-test commit followed by an implementation commit. The git history is the test plan.
4. **Quality gates from commit one.** Lint, vet, race-tested tests, coverage threshold, and vulnerability scanning are enforced in CI and in a pre-commit hook. No red commit reaches `main`.
5. **A small, opinionated public surface.** The library exposes the minimum needed to register rules, execute them against a typed input, and inspect the result. Sugar lives in extension packages or in the caller.

The project explicitly does **not** aim to:

- Reimplement a rule engine from scratch. We delegate to existing engines behind the port.
- Provide a DSL or grammar of our own. Each engine adapter speaks its native rule format.
- Solve rule authoring tooling (IDE plugins, web editors). That is a separate product.
- Run rules out-of-process. The library is in-process; clustering and hot-reload are out of scope.

## Consequences

The "swappable engine" claim is the load-bearing one. Every other decision is judged against it. If a future ADR proposes leaking an engine-specific type onto the public surface, this ADR has to be revisited first.

The "no DSL" decision means each engine adapter brings its own format. That trades elegance for honesty: rule authors learn the underlying engine, not an additional abstraction layer of ours.

The quality-gates-from-commit-one decision affects pace. The project deliberately moves slowly in the first few months -- there is no business pressure to ship -- to keep the gates non-negotiable while the codebase is small enough to refactor cheaply.
