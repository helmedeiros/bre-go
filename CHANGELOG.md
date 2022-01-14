# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
once a first tagged release is cut.

## [Unreleased]

### Added

- ADRs 0001–0005: bounded goals, Go as the language, the engine port, pre-generics input handling, and the `(Result, error)` return on `Execute`.
- `Makefile` and CI workflow running lint + vet + test + coverage threshold from commit one. The cover target tolerates the empty-module and the no-statements case (vacuously passes) and filters `enginetest/` from the production-code coverage calculation.
- `engine` package: the `Engine` port, `Context` and `Result` value types, a witness `nilEngine` in tests proving the interface is implementable.
- `engine/inmemory`: the first concrete adapter. `Rule` holds a `Name`, a `Condition`, and an `Action`. `AddRule` rejects empty names with `ErrEmptyRuleName`. `Execute` walks rules in insertion order, appends matched names, and lets later actions overwrite earlier `Output` (last-match-wins).
- `engine/enginetest`: shared contract suite (`RunContractTests`) that every adapter wires from a single test function. Five behavioral cases pin the port's promises across implementations.
- `observability` package: `Logger` interface (`Info` / `Error` with structured `Field` key/value pairs), constructors (`String`, `Int`, `Bool`, `Err`), and a `NopLogger` default that adapters use when the caller does not supply one.
- `CONTRIBUTING.md` documents the four-step adapter recipe and points at the inmemory wiring template.
