# 2. Go As The Implementation Language

## Status

Accepted

## Context

Picking the implementation language for a new business-rule library comes down to a small set of tradeoffs between runtime footprint, ecosystem, and developer ergonomics. The candidates considered:

- **JVM languages.** Mature library ecosystem for observability, configuration, and testing. Cold-start cost and memory footprint at runtime are real penalties for short-lived workloads.
- **Go.** Single static binary, small footprint, fast cold start. Standard library covers most needs without a dependency on a framework. Sub-second compilation gives the TDD loop a different character.
- **Rust.** Strongest safety story, steepest learning curve, smallest BRE library ecosystem at the time of writing.

The project's primary deployment target is small in-process libraries embedded in short-lived workers and serverless functions. Cold-start cost matters.

The Go ecosystem in January 2022 has one notable gap that influenced the design: generics arrive with Go 1.18 in mid-March 2022. Until then, the executor's input/output types are expressed via `interface{}` (boxed) with type-checked accessors. ADR-0003 (engine port) is written with this constraint in mind; ADR-0004 (generic executor) lands when 1.18 ships.

## Decision

Implement in Go. Pin the module to Go 1.17 today and bump to 1.18 with an explicit ADR when generics are stable.

Conventions adopted from the start:

- Module path is `github.com/helmedeiros/bre-go`.
- Code formatted by `gofmt` + `goimports`; enforced in CI through `golangci-lint`.
- Tests use the standard `testing` package and table-driven tests; no third-party assertion library.
- Errors are returned as `error` values; no panic in library code.
- Public types are documented with `// PublicName describes ...` package doc comments.

## Consequences

The pre-1.18 codebase pays a small ergonomic cost: rule input/output types travel as `interface{}` and adapters do the unboxing. The cost is intentional and short-lived; the project does not build a long-lived parallel API to work around it.

Go's lack of overloaded operators and inheritance pushes the public surface toward small interfaces and explicit composition. That matches the hexagonal port design coming in ADR-0003 -- the language nudges us toward the architecture we want.

Standard library + small dependency footprint means the security-update surface stays manageable. Every new dependency requires an ADR.
