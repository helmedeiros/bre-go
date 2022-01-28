# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
once a first tagged release is cut.

## [Unreleased]

### Added

- ADRs 0001–0010: bounded goals, Go as the language, the engine port, pre-generics input handling, the `(Result, error)` return on `Execute`, the `Context`→`Request` rename, the Execution Listener observer port, built-in listener implementations, rejecting duplicate rule names on `AddRule`, and `ListenerHost` as an optional capability interface.
- `Makefile` and CI workflow running lint + vet + test + coverage threshold from commit one. The cover target tolerates the empty-module and the no-statements case (vacuously passes) and filters `enginetest/` from the production-code coverage calculation. A `bench` target runs `go test -bench=. -benchmem` across every package.
- `engine` package: the `Engine` port, `Request` and `Result` value types, a witness `nilEngine` in tests proving the interface is implementable.
- `engine/inmemory`: the first concrete adapter. `Rule` holds a `Name`, a `Condition`, and an `Action`. `AddRule` rejects empty names with `ErrEmptyRuleName` and duplicate names with `ErrDuplicateRuleName`. `Execute` walks rules in insertion order, appends matched names, and lets later actions overwrite earlier `Output` (last-match-wins). `AddListener` registers any number of `observability.ExecutionListener`s; `Execute` fires `OnRuleMatched` once per matching rule, after the rule's action runs so the listener sees the post-action `Output`. The adapter satisfies the new `engine.ListenerHost` capability interface.
- `engine.ListenerHost` optional interface: callers can detect listener support on any adapter through a single type assertion, instead of asserting the concrete adapter type.
- `engine/enginetest`: shared contract suite (`RunContractTests`) that every adapter wires from a single test function. Six behavioral cases pin the port's promises across implementations, including a duplicate-name rejection case.
- `observability` package: `Logger` interface (`Info` / `Error` with structured `Field` key/value pairs), constructors (`String`, `Int`, `Bool`, `Err`), and a `NopLogger` default that adapters use when the caller does not supply one. `ExecutionListener` interface (`OnRuleMatched(Match)`), the `Match` value type (`Rule`, `Input`, `Output`), and a `NopExecutionListener` default that discards every match. Built-in `CountingListener` (per-rule and total hit counts; zero value usable) and `LoggingListener` (bridges matches to the `Logger` port; logs the rule name only -- payloads stay off the wire to avoid accidental PII leaks).
- `engine/inmemory` benchmarks (`BenchmarkExecuteOverTenRules`, `BenchmarkExecuteWithListenerOverTenRules`) pin the per-call cost so later changes have a baseline to compare against.
- `engine/inmemory` runnable examples (`ExampleEngine`, `ExampleEngine_AddListener`) double as compile-checked godoc.
- `observability` runnable examples (`ExampleCountingListener`, `ExampleLoggingListener`) cover the built-in listener shapes.
- `CONTRIBUTING.md` documents the four-step adapter recipe and points at the inmemory wiring template.
- `.github/dependabot.yml` sweeps go modules and GitHub Actions weekly on Monday morning São Paulo time, capped at five open PRs per ecosystem.
- `docs/clean-code-conventions.md` collects the six Clean Code rule groups (Names, Functions, Comments, Tests, Structure, Data/Objects) the codebase commits to.

### Changed

- `engine.Context` → `engine.Request`. The old name shadowed `context.Context` from the standard library; the new one names the role (a request to evaluate). ADR-0006 captures the call.
- Comments across the codebase reduced to one-line godoc. Multi-paragraph rationale lives in ADRs and PR descriptions, not in source files.
- `engine/inmemory` and `observability` test files split so every test asserts one behavior. Failures now name the missing property directly.

### Removed

- The duplicate unexported `errEmptyRuleName` sentinel in `engine/inmemory`. `ErrEmptyRuleName` remains and is the single point of comparison.
