# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). See [ADR-0021](docs/architecture/decisions/0021-release-versioning-policy.md) for the 0.x interpretation.

## [Unreleased]

### Added

_Nothing yet. New entries land here._

## [0.10.0] - 2022-07-01

Tenth minor release. Second of five Phase-4 follow-ups. Widens `engine/indexed` to admit `OpNeq` / `OpNotIn` as post-filter terms (paired with at least one indexable term), and adds a value-expression mini-grammar in `engine/parser` for CSV-shaped callers. Additive (no breaking changes from v0.9.x).

### Added

- `engine/indexed` admits `parser.StringCondition{Op: OpNeq}` and `parser.SetCondition{Op: OpNotIn}` as runtime post-filters when paired with at least one indexable term (`OpEq` / `OpIn`). After a bucket hit the engine evaluates the rule's post-filter against the input fact and skips the rule if any term returns false. Rules without negation pay zero hot-path cost (Execute checks `len(postFilter) == 0` and skips the Eval).
- New typed `ErrNoIndexableTerms` sentinel for rules whose `Match` has no `OpEq` / `OpIn` term (pure-negation shapes). `ErrNonIndexableCondition` retains its v0.8.0 meaning of "shape the engine does not understand."
- `engine/parser` ships `ParseValueExpression(field, value)` and `ValueExpressionError`. Recognized shapes: plain value → `StringCondition{OpEq}`, `!value` → `StringCondition{OpNeq}`, `value1|value2` → `SetCondition{OpIn}`, `*` or empty → returns `nil`. Mixed forms (`!a|b`), empty negation operand, and empty alternatives all return `*ValueExpressionError` with the offending field / value / cause.
- `engine/enginetest/bench` gains `RuleSpec.NeqValues map[string]string` and `Workload.OpNeqDims int`. The structured generator emits `OpNeq` `StringCondition`s for the tail dims; the closure generator implements equivalent negation semantics for the linear baseline.
- Two new v0.10.0 success-bar tests in `engine/indexed/success_bar_test.go`:
  - 1k rules, 5 dims, 1 OpNeq post-filter, `Last` -- ≥ 5× faster. Live: ~111×.
  - 10k rules, same shape, `NoHit` -- ≥ 30× faster (bar relaxed from 50× to absorb post-filter overhead). Live: ~2 942×.
- ADR-0035 Accepted. Existing v0.8.0 + v0.9.0 success-bar cells continue to gate.

### Internal

- `extractIndexablePairs` returns both indexable terms (`[]fieldValueSet` for bucket-key construction) and post-filter terms (`[]parser.Condition` for runtime evaluation). New `classifyStringCondition` / `classifySetCondition` helpers route each Op to the right pile.
- `indexedRule` gains a `postFilter []parser.Condition` field. Empty / nil for pure-indexable rules.
- `Execute` lazily builds `map[string]interface{}` from the canonical `map[string]string` fact only when at least one candidate has a non-empty post-filter, so rules without negation skip the conversion.

## [0.9.1] - 2022-06-24

Patch release. Completes v0.9.0's perf picture by adding load-time benchmarks (the matrix + success-bar tests measure Execute only, with `b.ResetTimer()` excluding AddRule). No production-code change.

### Added

- `engine/indexed/load_bench_test.go` -- per-adapter load-time benchmarks across the same workloads the matrix uses. Compares `firstmatch.AddRule` (linear duplicate check) vs `indexed.AddRule` (Cartesian fan-out + hash-map duplicate check) at 1k and 10k rule counts, with and without OpIn shapes.
- `make bench-load` Makefile target.
- `BENCHMARKS.md` gains a "Load-time profile" section documenting the asymptotic crossover (indexed pays ~1.5× at small N for structural work, then wins ~4-6× at 10k+ because firstmatch is O(N²) while indexed is O(N)). Memory is always higher on indexed (~5× more) because of the bucket structures.

### Measured

Apple M4 / Go 1.18 / equality-only workloads:

| Rule count | firstmatch load | indexed load | Ratio |
|---:|---:|---:|---:|
| 1 000 | ~1.08 ms | ~1.48 ms | 1.4× slower |
| 10 000 | ~96.6 ms | ~17.4 ms | 5.6× faster |

Same shape with 9× OpIn fan-out per rule: indexed at 1k=2.27ms, 10k=27ms; still wins asymptotically at 10k.

Load is not gated by `ci-local` today -- the section is reference material for release prep and adapter-choice decisions. A future ADR may promote it to a hard gate when the v0.12.0 concurrency / hot-reload work needs a frozen load-time baseline.

## [0.9.0] - 2022-06-22

Ninth minor release. First of five Phase-4 follow-ups that incrementally widen what `engine/indexed` can match against. v0.9.0 admits `SetCondition{Op: OpIn}` (set membership) and documents wildcard semantics. Additive (no breaking changes from v0.8.0).

### Added

- `engine/indexed` now admits `parser.SetCondition{Op: OpIn, Values: [...]}` as part of a rule's `Match`. The expansion happens at `AddRule` time: the rule is inserted into one bucket entry per Cartesian-product combination of value sets. Empty value sets are rejected with `ErrNonIndexableCondition`; fan-outs above `maxFanout` (1024) are rejected with a new typed `*FanoutTooLargeError` carrying the rule name, computed cardinality, and limit. Single-value OpIn (`Values: ["x"]`) behaves identically to the equivalent `OpEq` shape. `OpNotIn` stays rejected -- that's v0.10.0's work.
- Wildcard semantics documented and locked in by tests: a rule whose `Match` omits a field is correctly handled by the existing key-set walker. Zero new production code; the property now has explicit test coverage so it cannot drift.
- `engine/enginetest/bench` gains `RuleSpec.InValues` (`map[string][]string`) and `Workload.OpInDims` / `Workload.OpInValuesPer` so the harness can generate OpIn-shaped rules across both linear and structured adapters for cross-adapter comparison. Existing equality-only workloads are unchanged.
- Two new v0.9.0 success-bar tests in `engine/indexed/success_bar_test.go`. Both run firstmatch and indexed live on OpIn-bearing workloads and assert the multiplier:
  - 1k rules, 5 dims, 2-of-5 OpIn with 3 values, `Last` position — ≥ 5× faster. Live: ~238×.
  - 10k rules, same shape, `NoHit` — ≥ 50× faster. Live: ~2 674×.
- ADR-0034 Accepted. Frozen BENCHMARKS.md cells extend; the four v0.8.0 cells continue to gate.

### Internal

- `extractEqualityPairs` → `extractIndexablePairs` returning `[]fieldValueSet` (one per constrained field; length-1 = OpEq, length-N = OpIn). Values are canonicalized (sorted + deduped) so two rules listing values in different order share a bucket.
- New helpers: `canonicalizeValues`, `cartesianFanout`, `enumerateCombinations`. The Cartesian-product walk uses a single reusable `[]fieldValuePair` slice and a closure-style `visit`; bucket inserts copy into a string key so the slice can be reused safely across iterations.

## [0.8.0] - 2022-06-17

Eighth minor release. Headline: the sub-linear `engine/indexed` adapter -- ADR-0001's eventual production path, finally landed. For rule sets expressible as conjunctions of equality predicates, `Execute` runs O(K) hash lookups where K is the number of distinct key-sets, independent of rule count. Additive (no breaking changes from v0.7.x).

### Added

- `engine/indexed` -- fourth concrete `engine.Engine` adapter, and the first sub-linear one. Rules use `Match parser.Condition` (typed AST from ADR-0028) instead of an opaque closure; `AddRule` rejects anything that is not a pure conjunction of `OpEq` `StringCondition`s with `ErrNonIndexableCondition`. Rules are bucketed by `(key-set, value-tuple)`; first-match semantics; insertion order breaks ties. Input must be `map[string]string` or `map[string]interface{}` (other shapes return `ErrIncompatibleInput`). New sentinels: `ErrEmptyRuleName`, `ErrNilMatch`, `ErrDuplicateRuleName`, `ErrNonIndexableCondition`, `ErrIncompatibleInput`. Embeds `engine/internal/adapter.Notifier` per ADR-0029. Twelfth public package; 100% test coverage. ADR-0033.
- `engine/indexed/success_bar_test.go` -- four `TestSuccessBar_*` tests that run `firstmatch` and `indexed` live and assert the multipliers committed in `BENCHMARKS.md`. Gates `ci-local`: any future change that drops the indexed adapter below `≥10×` at 1k/5d/NoHit, `≥5×` at 1k/5d/Last, `≥50×` at 10k/5d/NoHit, or above `2×` slowdown at 10 rules turns the build red.
- `engine/indexed/allocs_test.go` and `engine/indexed/stress_test.go` -- the standard alloc-tripwire (frozen at 2 allocs/op) and `//go:build stress` 100k-iteration loops per ADR-0032's pattern.
- `engine/enginetest/bench` gains `RuleSpec`, `StructuredSeedFunc`, `StructuredFactory`, `PopulateStructured`, `RunStructured`. The existing closure-based surface stays unchanged; adapters that introspect rule shape (indexed, and future variants) wire through the structured surface. `matrix_bench_test.go` now runs the indexed adapter alongside the three linear adapters in every matrix cell.
- README Stability + adapter table + Toolkit list all gain `engine/indexed`; `BENCHMARKS.md` records the cleared success bar with the indexed adapter's flat-cost profile (~155-380 ns/op across all four rule-count tiers).

### Measured

The indexed adapter clears every `BENCHMARKS.md` success-bar row by orders of magnitude. Live multipliers vs `firstmatch` on Apple M4 / Go 1.18:

| Cell | Required | Measured | |
|---|---:|---:|:--|
| 1k / 5d / NoHit | ≥ 10× faster | 254× faster | ✅ |
| 1k / 5d / Last | ≥ 5× faster | 230× faster | ✅ |
| 10k / 5d / NoHit | ≥ 50× faster | 2 625× faster | ✅ |
| 10 rules / 5d | ≤ 2× slowdown | 0.26× (faster, not slower) | ✅ |

## [0.7.2] - 2022-06-15

Patch release. Adds three Go-native regression gates that complement v0.7.1's advisory bench matrix without overlapping it. ADR-0032 Accepted. No public-runtime-API changes.

### Added

- **Allocation tripwires** in `engine/inmemory`, `engine/firstmatch`, `engine/priority`, and `engine/parser`. `testing.AllocsPerRun` pins the exact allocation count of each adapter's hot path (and the parser's `Parse` / `ParseToCondition`) at a fixed small workload. Tripwire-style: intentional perf changes update the constant in the same commit; unintentional changes fire the test. Gates `ci-local`.
- **Fuzz targets**: `FuzzParse` in `engine/parser` and `FuzzRuleConfigsArrayShape` in `engine/json`. Compile under `make test` (catches API drift) but only fuzz under `make fuzz-quick` (30s per target by default; tunable via `FUZZ_SECONDS`). Seed corpora cover the grammar / JSON shapes plus known boundary inputs. Asserts no panic and only typed-error returns.
- **Stress tests** behind `//go:build stress`: `TestExecuteSurvivesHighVolume` (100k iterations) and `TestNoGoroutineLeakUnderListenerFanout` per adapter. Run via `make stress` with `-race -count=1`. Not in `ci-local` -- release-prep gate alongside `make bench-matrix`.
- `make fuzz-quick` and `make stress` targets. `make test` runtime increase: ~50ms total (the allocation tripwires).

## [0.7.1] - 2022-06-06

Patch release. Adds the cross-adapter performance benchmark harness ahead of v0.8.0's indexed-matcher work. No public-runtime-API changes; the new package is a test-only sibling of `engine/enginetest`.

### Added

- `engine/enginetest/bench` -- shared performance benchmark harness. `Workload` struct combines rule count, dimensionality, match position, and selectivity; `BasicMatcher(rules)` is the canonical 5-dimensional sparse-selectivity workload. `Run(b, w, factory)` populates the engine outside the timed section, then drives `Execute` for `b.N` iterations. Custom-adapter authors call it the same way the built-ins do. ADR-0031.
- `BENCHMARKS.md` at the repo root: frozen baselines for `inmemory` / `firstmatch` / `priority` across the curated matrix cells, plus the pre-committed v0.8.0 success bar (≥ 10× firstmatch at 1k/5-dim NoHit; ≥ 50× at 10k/5-dim NoHit; within 2× at 10 rules) that the indexed adapter must clear to ship.
- `make bench-matrix` target -- runs the cross-adapter matrix in isolation; the existing `make bench` keeps its per-package scope.
- README Toolkit lists `engine/enginetest/bench`.

## [0.7.0] - 2022-06-03

Seventh minor release. Opens Phase 4's declarative-rule-loading second front: JSON joins CSV as a built-in rule source. Additive (no breaking changes from v0.6.0). Includes one internal-only refactor (ADR-0029) that does not affect the public surface.

### Added

- `engine/json` -- the second concrete `RuleConfigProvider`. `Loader[RC]` reads a top-level JSON array from a file or `io.Reader`; each element is handed to a caller-supplied `ItemParser[RC] func(item json.RawMessage) (RC, error)` so the wire-to-engine mapping stays in caller code. `LoadError.Index` is 0-indexed (natural JSON-array position); `-1` for document-level failures (file open, malformed JSON, top-level value not an array). Two constructors (`NewLoader`, `NewLoaderFromReader`) mirror `engine/csv`. Eleventh public package; 100% test coverage; ~80 LOC. ADR-0030.
- Cookbook gains a "Load rules from a JSON file" pattern with the canonical wiring (wire-shape unmarshal → engine-internal map → `engine.Load`).
- README Stability section adds the JSON loader; Toolkit listing names it alongside the CSV loader.

### Internal

- Extracted listener-wiring duplication out of `engine/inmemory`, `engine/firstmatch`, and `engine/priority` into a shared `engine/internal/adapter.Notifier` type. Each adapter `Engine` now embeds `adapter.Notifier` by value; method promotion preserves the public surface (`AddListener` is still defined on every adapter's `*Engine`). The other per-adapter helpers (`evaluateCondition`, `hasAction`, `runAction`, `copyTags`) stay duplicated on purpose -- they're typed over each adapter's local `Rule` struct. ADR-0029. No public-API change.

## [0.6.0] - 2022-05-27

Sixth minor release. Closes Phase 3 (Expression DSL). Additive (no breaking changes from v0.5.0).

### Added

- Typed `Condition` tree in `engine/parser`. New exported types: `Condition` interface (`Eval(fact) bool`), `StringCondition`, `SetCondition`, `AndCondition`, `OrCondition`, `NotCondition`, and op constants (`OpEq`, `OpNeq`, `OpIn`, `OpNotIn`).
- `parser.ParseToCondition(expr) (Condition, error)` returns the typed tree for inspection / marshalling / transformation.
- `parser.AsPredicate(c)` converts a typed Condition tree to a `Predicate`.
- `parser.AsRuleCondition(c, factOf) func(interface{}) bool` bridges a typed Condition tree directly to `Rule.Condition`.
- AND / OR chains flatten into N-ary `AndCondition` / `OrCondition` nodes (not nested binaries), making the tree easier to walk -- groundwork for Phase 4's indexed-matcher work.
- Cookbook gains "Inspect parsed conditions as typed trees" with a field-collector type-switch example.
- README Stability section adds the typed Condition tree.

### Internal

- `engine/parser` refactored so the parser builds the Condition tree directly. The existing `Parse(expr) (Predicate, error)` signature is preserved as a thin wrapper (`ParseToCondition` + `AsPredicate`). 18 new tests cover Eval semantics, unknown-op fallback, empty-children identities, error propagation, and both bridges. 100% coverage held.

## [0.5.0] - 2022-05-20

Fifth minor release. Opens Phase 3 (Expression DSL). Additive (no breaking changes from v0.4.0).

### Added

- `engine/parser` package -- expression DSL for rule conditions. `Parse(expr)` compiles a condition string into a `Predicate` (`func(map[string]interface{}) bool`); `AsCondition(pred, factOf)` bridges it to `Rule.Condition`. Grammar: `==` / `!=` / `IN` / `NOT IN` / `AND` / `OR` / `NOT` over string literals; parens; both single (`'...'`) and double (`"..."`) quotes (single-quote support added during pre-tag validation when CSV-cell-embedded conditions hit the obvious double-quote collision). `ParseError` carries the byte position of any syntax failure. Tenth public package; 100% test coverage; ~300 LOC. ADR-0027.
- Cookbook gains a "Write rule conditions as strings" section with the grammar table, the CSV-loaded wiring, and the single-quote-strings-in-CSV explainer.
- README Toolkit lists `engine/parser`; Stability section adds the parser surface as stable.

## [0.4.0] - 2022-05-13

Fourth minor release. Closes Phase 2's rule-loading + observability extensions. Additive (no breaking changes from v0.3.0).

### Added

- `engine.ChainProviders[RC](providers...)` -- multi-source rule composition. Combines multiple `RuleConfigProvider[RC]` into one; concatenates outputs in argument order; first-error short-circuits. Zero-arg call returns an empty provider (identity element). ADR-0025.
- `engine.WithCorrelationID(ctx, id)` and `engine.CorrelationIDFromContext(ctx)` -- standard helpers for stamping a request-scoped identifier on `context.Context`. Unexported key type prevents collisions. `ConditionContext` / `ActionContext` callbacks recover the ID inside `Execute`. ADR-0026.
- Cookbook gains two sections: "Compose rules from multiple sources" and "Propagate correlation IDs", with the canonical wiring patterns and the documented per-request-listener workaround until ctx-aware listener interfaces land in a future ADR.
- README Stability section adds the new four helpers; Toolkit row for engine names the loader and correlation helpers explicitly.

## [0.3.0] - 2022-05-06

Second minor release. Headline change: rule loading from external sources is now supported through `engine.RuleConfigProvider` + the first concrete provider `engine/csv`. Additive (no breaking changes from v0.2.0).

### Added

- `engine.RuleConfig` interface (single method `RuleName() string`) -- the minimum contract any loader's configuration type must satisfy.
- `engine.RuleConfigProvider[RC engine.RuleConfig]` interface -- the single-method (`RuleConfigs() ([]RC, error)`) shape every loader implements.
- `engine.Load[RC]` generic helper -- pulls configs from a provider and bridges to an adapter's `AddRule` via a caller-supplied closure.
- `engine/csv` package -- first concrete provider. `Loader[RC]` reads CSV from a file path (`NewLoader`) or `io.Reader` (`NewLoaderFromReader`); `SkipHeader(n)` and `Comma(c)` chainable for ergonomics. Errors wrap in `*LoadError` carrying `Path` and 1-indexed `Row`; `Unwrap()` exposes the underlying error.
- Runnable godoc example wiring `csv.NewLoaderFromReader` → `engine.Load` → `priority.Engine`.
- Benchmarks for `csv.Loader` at 10 and 100 rows (~1.4us and ~8.5us baselines).
- Cookbook section "Load rules from a CSV file" walking through the canonical pattern.
- CONTRIBUTING section "Adding a new rule loader" with the four-step recipe future loader contributors follow.

### Internal

- `docs/cookbook.md` landed (eight earlier sections from Week 17, plus the CSV section).
- README Toolkit + Stability sections refreshed for the new public surface.

## [0.2.0] - 2022-04-22

First minor release post-v0.1.0. Headline change: `context.Context` flows through `Execute` (breaking).

### Added

- `observability.SnapshotListener`: sixth built-in. Implements all four listener interfaces (`ExecutionListener`, `ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`) in one type. Captures every event into public slices (`Matches`, `Started`, `Finished`, `Errored`) for later assertion. Designed primarily for tests; benchmarked against `NopExecutionListener` so the production trade-off is visible. `Reset()` enables reuse across executions in one test.
- Optional `ConditionContext` and `ActionContext` fields on every adapter's `Rule` struct. The context-aware variants are preferred over the narrow `Condition`/`Action` when set; the narrow variants stay backward-compatible.
- `engine/enginetest.RunContractTests` gains a 12th case: a cancelled context produces a non-nil error from `Execute`.

### Changed

- **BREAKING**: `engine.Engine.Execute` now takes `(ctx context.Context, req Request)`. Every existing call site updates. Cancelled contexts produce `OnExecutionErrored` and a non-nil return; `OnExecutionFinished` still fires to keep the lifecycle pair balanced.
- **BREAKING**: `engine/exec.Executor[In, Out].Execute` now takes `(ctx context.Context, in In)`. Same shape as the underlying port.
- `AddRule`'s `ErrNilCondition` now triggers only when **both** `Condition` and `ConditionContext` are nil. Rules that set either compile.
- Contract suite + polymorphic tests + every adapter test + the README Quickstart updated to thread `context.Background()` (or a real ctx) through `Execute`.

### Internal

- Contract suite's `lifecycleRecorder` and `erroredRecorder` types replaced by `SnapshotListener` (dogfood -- the new built-in is the canonical recorder pattern).
- CONTRIBUTING adapter recipe points new test authors at `SnapshotListener` rather than hand-rolling per-test recorders.
- Godoc example + integration tests verify `SnapshotListener` captures events end-to-end against `inmemory.Engine`, including the panic path where `Errored` carries the typed `*ActionPanicError`.

## [0.1.0] - 2022-04-07

First tagged release. Three concrete adapters, six public packages, twenty-one Architecture Decision Records, 100% per-package test coverage.

### Added

- ADRs 0001–0021: bounded goals, Go as the language, the engine port, pre-generics input handling (now superseded by ADR-0013), the `(Result, error)` return on `Execute`, the `Context`→`Request` rename, the Execution Listener observer port, built-in listener implementations, rejecting duplicate rule names on `AddRule`, `ListenerHost` as an optional capability interface, the ADR lifecycle/supersession convention, rejecting rules with a nil `Condition`, the generic `Executor[In, Out]` wrapper layer over `engine.Engine`, a first-match adapter alongside inmemory, Boolean condition combinators, `RuleLister` as an optional introspection interface, per-execution lifecycle listeners, action panic recovery with the Errored lifecycle event, a priority-ordered adapter, `RuleInfoLister` for introspection beyond names, and the release versioning policy.
- `Makefile` and CI workflow running lint + vet + test + coverage threshold from commit one. The cover target tolerates the empty-module and the no-statements case (vacuously passes) and filters `enginetest/` from the production-code coverage calculation. A `bench` target runs `go test -bench=. -benchmem` across every package.
- `engine` package: the `Engine` port, `Request` and `Result` value types, a witness `nilEngine` in tests proving the interface is implementable.
- `engine/inmemory`: the first concrete adapter. `Rule` holds a `Name`, a `Condition`, and an `Action`. `AddRule` rejects empty names with `ErrEmptyRuleName`, rules with a nil `Condition` with `ErrNilCondition`, and duplicate names with `ErrDuplicateRuleName` (shape-first, state-second check order, so error returns stay deterministic regardless of registration order). `Execute` walks rules in insertion order, appends matched names, and lets later actions overwrite earlier `Output` (last-match-wins). A panicking `Action` is recovered and surfaced as an `*ActionPanicError` (carrying the rule name via `RuleName()`); `Execute` stops on the first panic and returns the partial `Result` + the error. `AddListener` registers any number of `observability.ExecutionListener`s; `Execute` fires `OnRuleMatched` once per matching rule after a successful action, with `OnExecutionErrored` firing for the panicking rule instead. The adapter satisfies the new `engine.ListenerHost` capability interface.
- `engine.ListenerHost` optional interface: callers can detect listener support on any adapter through a single type assertion, instead of asserting the concrete adapter type.
- `engine.RuleLister` optional interface: callers can enumerate the registered rule set on any adapter that supports introspection through a single type assertion. Returns a fresh `[]string` in insertion order; mutating the returned slice does not affect engine state. All three shipped adapters satisfy it.
- `engine.RuleInfoLister` optional interface + `engine.RuleInfo` value type: richer introspection returning `[]RuleInfo{Name, Description, Tags}` for catalog endpoints and rule auditing. Each adapter's `Rule` struct grows optional `Description` and `Tags` fields next to `Name` (additive, non-breaking). Returned slice and each `RuleInfo.Tags` are fresh copies; mutating either does not affect engine state. All three shipped adapters satisfy it.
- `engine/firstmatch`: second concrete adapter. Same `Rule` shape and registration validation as `inmemory` (empty name, nil condition, duplicate name -- shape-first, state-second order). Different `Execute` semantics: walk in insertion order, return on the first matching rule. Later rules never evaluate and their actions never run. Recovers panicking `Action`s with its own typed `*ActionPanicError`; the matched rule still appears in `Result.Matched` because the rule *did* match -- its Action failed. Satisfies `engine.ListenerHost`; the listener sees the one matching rule. Picked when the policy is "decision table, content classifier, first applicable rate".
- `engine/priority`: third concrete adapter. Same `Rule` shape plus a `Priority int` field. Same registration validation. `Execute` sorts a copy of the rule list per call by descending priority (ties broken by insertion order, stable sort) and returns on the first matching rule. `RuleNames()` returns insertion order so introspection reflects what was registered. Picked when rule precedence belongs in the data (config-driven decision tables, modular rule sets composed at runtime) rather than in `AddRule` call order. Same panic-recovery contract as the other adapters. Satisfies both `engine.ListenerHost` and `engine.RuleLister`. Runnable example in `ExampleEngine`. Benchmark pins the per-call sort cost (~590 ns/op at ten rules; the documented CPU/state trade-off from ADR-0019).
- `engine/enginetest`: shared contract suite (`RunContractTests`) that every adapter wires from a single test function. Eleven behavioral cases pin the port's promises across implementations, including a duplicate-name rejection case, capability-aware cases for `ListenerHost`, `RuleLister`, and `RuleInfoLister`, a lifecycle case asserting that any listener implementing `ExecutionStartedListener` + `ExecutionFinishedListener` receives exactly one of each per `Execute`, and a panic-recovery case asserting that a panicking `Action` surfaces through `OnExecutionErrored` and returns a non-nil error. All capability-aware cases auto-skip for adapters that do not implement the relevant interface.
- `engine/polymorphic_test.go`: table-driven tests that exercise both adapters through `engine.Engine` and `engine.ListenerHost` alone. Zero adapter-specific code in the assertion bodies -- the testimony for ADR-0003's port abstraction.
- `engine/firstmatch` benchmarks (first-rule-matches, last-rule-matches, with-listener) and runnable example (`ExampleEngine`, three-tier pricing scenario) mirror the inmemory shape.
- README "which adapter do I want?" table maps each adapter to its semantic and use case so callers do not have to read both package docs to pick. A new Quickstart code block at the top wires `inmemory` + `engine/conditions` + a `CountingListener` together. A new Toolkit table enumerates every public package on the project.
- `engine/conditions`: Boolean combinators (`And`, `Or`, `Not`) and sentinels (`Always`, `Never`) that produce `func(interface{}) bool` predicates of the same shape `Rule.Condition` expects. Short-circuit in argument order. Empty `And` is true, empty `Or` is false (algebraic identities). 100% coverage, zero-allocation benchmarks, three godoc examples (the third uses `Always` as a firstmatch catch-all). A polymorphic test seeds a nested combinator into both adapters to prove the package is genuinely adapter-agnostic.
- `engine/exec`: generic `Executor[In, Out]` wrapper over any `engine.Engine`. Hides the `interface{}` cast at the call boundary; returns `(Out, []string, error)`. A new `*OutputTypeMismatchError` is returned when the engine produces an `Output` that cannot be asserted to `Out` (carries the expected and actual type names via the standard `errors.As` idiom). A `nil` `Output` returns the zero value of `Out` and a `nil` error -- "no decision" is not a mismatch. The wrapper is engine-agnostic: a single `Executor[In, Out]` wraps both shipped adapters interchangeably, pinned by a polymorphic test. Requires Go 1.18.
- CONTRIBUTING adapter recipe refined with what writing `firstmatch` taught: name adapters after the policy not the storage; `Rule` belongs to the adapter package not the port; validation runs shape-first then state-second; satisfying `ListenerHost` and shipping an example are recommended.
- `observability` package: `Logger` interface (`Info` / `Error` with structured `Field` key/value pairs), constructors (`String`, `Int`, `Bool`, `Err`), and a `NopLogger` default that adapters use when the caller does not supply one. `ExecutionListener` interface (`OnRuleMatched(Match)`), the `Match` value type (`Rule`, `Input`, `Output`), and a `NopExecutionListener` default that discards every match. Three additional role interfaces -- `ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener` -- give listeners hooks for the start, end, and error path of an `Execute` call; adapters call them via type assertion at notify time, so plain `ExecutionListener` implementations still work unchanged. Built-in `CountingListener` (per-rule and total hit counts; zero value usable), `LoggingListener` (bridges matches to the `Logger` port; logs the rule name only -- payloads stay off the wire to avoid accidental PII leaks), and `TimingListener` (implements all three lifecycle interfaces in one type to expose `LastDuration()` and `MatchesInLastExecution()`).
- `engine/inmemory` benchmarks (`BenchmarkExecuteOverTenRules`, `BenchmarkExecuteWithListenerOverTenRules`) pin the per-call cost so later changes have a baseline to compare against.
- `engine/inmemory` runnable examples (`ExampleEngine`, `ExampleEngine_AddListener`) double as compile-checked godoc.
- `observability` runnable examples (`ExampleCountingListener`, `ExampleLoggingListener`) cover the built-in listener shapes.
- `CONTRIBUTING.md` documents the four-step adapter recipe and points at the inmemory wiring template.
- `.github/dependabot.yml` sweeps go modules and GitHub Actions weekly on Monday morning São Paulo time, capped at five open PRs per ecosystem.
- `docs/clean-code-conventions.md` collects the six Clean Code rule groups (Names, Functions, Comments, Tests, Structure, Data/Objects) the codebase commits to.
- `scripts/check-adrs.sh` (wired into `make all` / `ci-local`): verifies every ADR file is indexed in the ADR README, every README link points at a real file, every ADR has one of the five allowed status values, and every ADR carries the four standard sections (Status, Context, Decision, Consequences). Catches typos like `Acccepted`, a new ADR that forgets to update the index, and a Decision heading with inline qualifiers.
- ADR status table in `docs/architecture/decisions/README.md` shows the lifecycle marker (Proposed, Accepted, Accepted (provisional), Superseded by ADR-N, Deprecated) for every ADR at a glance.

### Changed

- `go.mod` from `go 1.17` to `go 1.18`, CI's `setup-go` and `golangci-lint`'s `run.go` to match. Go 1.18 GA landed Mar 15, 2022; the bump unlocks the `engine/exec` generic wrapper. Adapters and the engine port are unchanged.
- `engine.Context` → `engine.Request`. The old name shadowed `context.Context` from the standard library; the new one names the role (a request to evaluate). ADR-0006 captures the call.
- Comments across the codebase reduced to one-line godoc. Multi-paragraph rationale lives in ADRs and PR descriptions, not in source files.
- `engine/inmemory` and `observability` test files split so every test asserts one behavior. Failures now name the missing property directly.

### Removed

- The duplicate unexported `errEmptyRuleName` sentinel in `engine/inmemory`. `ErrEmptyRuleName` remains and is the single point of comparison.
