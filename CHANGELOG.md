# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project aims to follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
once a first tagged release is cut.

## [Unreleased]

### Added

- ADR-0001 Bounded Goals, ADR-0002 Go as the language, ADR-0003 the engine port.
- Makefile and CI workflow running lint + vet + test + coverage threshold from commit one (gates are vacuously green while the codebase is empty).
- `engine` package: `Engine` interface, `Context`, `Result` value types, contract test scaffold.

