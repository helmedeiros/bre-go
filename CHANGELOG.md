# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
once a first tagged release is cut.

## [Unreleased]

### Added

- ADRs 0001–0015: bounded goals, Go as the language, the engine port, pre-generics input handling (provisional pending Go 1.18 GA), the `(Result, error)` return on `Execute`, the `Context`→`Request` rename, the Execution Listener observer port, built-in listener implementations, rejecting duplicate rule names on `AddRule`, `ListenerHost` as an optional capability interface, the ADR lifecycle/supersession convention, rejecting rules with a nil `Condition`, (Proposed) a generic `Executor[In, Out]` wrapper layer over `engine.Engine`, a first-match adapter alongside inmemory, and Boolean condition combinators.
- `Makefile` and CI workflow running lint + vet + test + coverage threshold from commit one. The cover target tolerates the empty-module and the no-statements case (vacuously passes) and filters `enginetest/` from the production-code coverage calculation. A `bench` target runs `go test -bench=. -benchmem` across every package.
- `engine` package: the `Engine` port, `Request` and `Result` value types, a witness `nilEngine` in tests proving the interface is implementable.
- `engine/inmemory`: the first concrete adapter. `Rule` holds a `Name`, a `Condition`, and an `Action`. `AddRule` rejects empty names with `ErrEmptyRuleName`, rules with a nil `Condition` with `ErrNilCondition`, and duplicate names with `ErrDuplicateRuleName` (shape-first, state-second check order, so error returns stay deterministic regardless of registration order). `Execute` walks rules in insertion order, appends matched names, and lets later actions overwrite earlier `Output` (last-match-wins). `AddListener` registers any number of `observability.ExecutionListener`s; `Execute` fires `OnRuleMatched` once per matching rule, after the rule's action runs so the listener sees the post-action `Output`. The adapter satisfies the new `engine.ListenerHost` capability interface.
- `engine.ListenerHost` optional interface: callers can detect listener support on any adapter through a single type assertion, instead of asserting the concrete adapter type.
- `engine/firstmatch`: second concrete adapter. Same `Rule` shape and registration validation as `inmemory` (empty name, nil condition, duplicate name -- shape-first, state-second order). Different `Execute` semantics: walk in insertion order, return on the first matching rule. Later rules never evaluate and their actions never run. Satisfies `engine.ListenerHost`; the listener sees the one matching rule. Picked when the policy is "decision table, content classifier, first applicable rate".
- `engine/enginetest`: shared contract suite (`RunContractTests`) that every adapter wires from a single test function. Seven behavioral cases pin the port's promises across implementations, including a duplicate-name rejection case and a ListenerHost-aware case that auto-skips for adapters without listener support.
- `engine/polymorphic_test.go`: table-driven tests that exercise both adapters through `engine.Engine` and `engine.ListenerHost` alone. Zero adapter-specific code in the assertion bodies -- the testimony for ADR-0003's port abstraction.
- `engine/firstmatch` benchmarks (first-rule-matches, last-rule-matches, with-listener) and runnable example (`ExampleEngine`, three-tier pricing scenario) mirror the inmemory shape.
- README "which adapter do I want?" table maps each adapter to its semantic and use case so callers do not have to read both package docs to pick. A new Quickstart code block at the top wires `inmemory` + `engine/conditions` + a `CountingListener` together. A new Toolkit table enumerates every public package on the project.
- `engine/conditions`: Boolean combinators (`And`, `Or`, `Not`) and sentinels (`Always`, `Never`) that produce `func(interface{}) bool` predicates of the same shape `Rule.Condition` expects. Short-circuit in argument order. Empty `And` is true, empty `Or` is false (algebraic identities). 100% coverage, zero-allocation benchmarks, three godoc examples (the third uses `Always` as a firstmatch catch-all). A polymorphic test seeds a nested combinator into both adapters to prove the package is genuinely adapter-agnostic.
- CONTRIBUTING adapter recipe refined with what writing `firstmatch` taught: name adapters after the policy not the storage; `Rule` belongs to the adapter package not the port; validation runs shape-first then state-second; satisfying `ListenerHost` and shipping an example are recommended.
- `observability` package: `Logger` interface (`Info` / `Error` with structured `Field` key/value pairs), constructors (`String`, `Int`, `Bool`, `Err`), and a `NopLogger` default that adapters use when the caller does not supply one. `ExecutionListener` interface (`OnRuleMatched(Match)`), the `Match` value type (`Rule`, `Input`, `Output`), and a `NopExecutionListener` default that discards every match. Built-in `CountingListener` (per-rule and total hit counts; zero value usable) and `LoggingListener` (bridges matches to the `Logger` port; logs the rule name only -- payloads stay off the wire to avoid accidental PII leaks).
- `engine/inmemory` benchmarks (`BenchmarkExecuteOverTenRules`, `BenchmarkExecuteWithListenerOverTenRules`) pin the per-call cost so later changes have a baseline to compare against.
- `engine/inmemory` runnable examples (`ExampleEngine`, `ExampleEngine_AddListener`) double as compile-checked godoc.
- `observability` runnable examples (`ExampleCountingListener`, `ExampleLoggingListener`) cover the built-in listener shapes.
- `CONTRIBUTING.md` documents the four-step adapter recipe and points at the inmemory wiring template.
- `.github/dependabot.yml` sweeps go modules and GitHub Actions weekly on Monday morning São Paulo time, capped at five open PRs per ecosystem.
- `docs/clean-code-conventions.md` collects the six Clean Code rule groups (Names, Functions, Comments, Tests, Structure, Data/Objects) the codebase commits to.
- `scripts/check-adrs.sh` (wired into `make all` / `ci-local`): verifies every ADR file is indexed in the ADR README, every README link points at a real file, and every ADR has one of the five allowed status values. Catches typos like `Acccepted` and a new ADR that forgets to update the index.
- ADR status table in `docs/architecture/decisions/README.md` shows the lifecycle marker (Proposed, Accepted, Accepted (provisional), Superseded by ADR-N, Deprecated) for every ADR at a glance.

### Changed

- `engine.Context` → `engine.Request`. The old name shadowed `context.Context` from the standard library; the new one names the role (a request to evaluate). ADR-0006 captures the call.
- Comments across the codebase reduced to one-line godoc. Multi-paragraph rationale lives in ADRs and PR descriptions, not in source files.
- `engine/inmemory` and `observability` test files split so every test asserts one behavior. Failures now name the missing property directly.

### Removed

- The duplicate unexported `errEmptyRuleName` sentinel in `engine/inmemory`. `ErrEmptyRuleName` remains and is the single point of comparison.
