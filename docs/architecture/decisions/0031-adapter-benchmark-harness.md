# 31. Adapter Performance Benchmark Harness

## Status

Proposed — target v0.7.1. A small, additive patch release that lands
before v0.8.0's indexed matcher work so the baseline numbers exist
**before** the implementation being evaluated, not alongside it.

## Context

The library has shipped seven minors. Each adapter has a handful of
its own benchmarks (`engine/inmemory` pins per-call cost at 10 rules;
`engine/csv` and `engine/json` pin loader cost at 10 and 100 rows /
items; `engine/parser` pins parse + eval cost). Those benchmarks are
useful for catching per-package regressions but they do not let you
compare adapters apples-to-apples, and they do not scale across rule
counts.

v0.8.0 is the indexed matcher. Its entire value proposition is
sub-linear lookup at large N over multi-dimensional composite keys.
"Considerably faster" is the headline claim, but without a
pre-frozen baseline and a pre-committed success bar, that claim
reduces to "whatever the numbers turn out to be." Three problems with
generating the benchmark alongside the indexed matcher:

1. **No independent baseline.** Numbers for the linear adapters
   measured on the same machine in the same session as the indexed
   matcher land in the same delta cell as "Go version drift," "OS
   scheduler noise," and "I added a print statement." The comparison
   has no anchor.
2. **Benchmark-design bias.** A workload designed while the indexed
   matcher is being built tends to flatter what the indexed matcher
   does well. Designing it against only the linear adapters keeps the
   workload honest.
3. **No release-readiness checkpoint.** Without a pre-committed bar,
   v0.8.0 ships whenever the implementer thinks the numbers look
   good. With a bar, v0.8.0 either earns it or stays unreleased.

Three design questions:

**1. Where does the harness live?**

- (a) `engine/enginetest/bench` — sibling to the contract suite.
- (b) `engine/bench` — top-level under `engine/`.
- (c) `internal/bench` — module-private.

Pick (a). `engine/enginetest` already exists to "exercise any
`engine.Engine` in a standardized way." Benchmarking is the same
shape — hand it any `engine.Engine`, get a standardized perf
report — so it belongs at the same depth, with the same audience.
Custom-adapter authors who use `enginetest` to check correctness gain
a sibling sub-package to check performance.

Public access matters: the parity goal is a pluggable engine port.
Callers building their own adapter (a wrapper around a third-party
rule engine, an experimental implementation, a sharded variant) want
the same harness the library uses on its own built-ins. Hiding it in
`internal/` would defeat that.

**2. What does the workload matrix look like?**

A `Workload` is a configurable struct combining:

- **Rule-set size.** 10 / 100 / 1k / 10k. Smaller sizes catch
  fixed-overhead regressions; larger sizes catch the asymptotic
  story.
- **Match position.** `First` / `Middle` / `Last` / `NoHit`. The
  linear adapters' worst case is `Last` and `NoHit` — both walk the
  full rule list. The indexed adapter's win is precisely there.
- **Rule dimensionality.** 1 / 3 / 5 equality conditions per rule.
  1-dim is the degenerate case where a hash lookup is barely
  different from a linear comparison; 5-dim is where the composite
  key shines.
- **Match selectivity.** `Unique` (exactly one matching rule) /
  `Sparse` (~1% match) / `Dense` (~50% match). Indexing wins biggest
  at unique / sparse; dense matches narrow the gap because the linear
  adapter exits early.

Not every Cartesian combination ships. The matrix is **curated** to a
small set of cells that exercise representative shapes. One canonical
workload, `BasicMatcher`, encodes the shape we expect the most: 5-dim
equality conditions, sparse selectivity, configurable size.

**3. What's the success bar for v0.8.0?**

Pre-commit numbers in `BENCHMARKS.md` against `firstmatch` (the
fairest linear competitor — both adapters return on first match):

| Shape | Versus `firstmatch` |
|---|---|
| 1k rules, 5 dims, `NoHit` | ≥ 10× faster |
| 1k rules, 5 dims, `Last` | ≥ 5× faster |
| 10k rules, 5 dims, `NoHit` | ≥ 50× faster |
| 10 rules, 5 dims, any | within 2× (indexing overhead must not dominate small sets) |

The first three rows are the win the indexed matcher needs to earn.
The fourth row is the **anti-regression** guardrail — an indexed
adapter that beats firstmatch at 10k rules but is 20× slower at 10
rules is not a drop-in replacement; it's a sometimes-replacement, and
that needs a separate ADR with a selection story (which today's
roadmap defers to v0.10.0's provider selection).

If v0.8.0 misses any row, it does not ship as v0.8.0. The bar is the
contract.

## Decision

Add a new sub-package `engine/enginetest/bench` exporting:

```go
// MatchPosition tells the harness where in the rule list the
// matching rule sits (or whether nothing matches).
type MatchPosition int
const (
    First MatchPosition = iota
    Middle
    Last
    NoHit
)

// Selectivity tells the harness how many rules out of N match a
// given input.
type Selectivity int
const (
    Unique Selectivity = iota // exactly one rule matches
    Sparse                    // ~1% of rules match
    Dense                     // ~50% of rules match
)

// Workload encodes one matrix cell.
type Workload struct {
    Rules       int
    Dimensions  int
    Position    MatchPosition
    Selectivity Selectivity
}

// BasicMatcher is the canonical workload: 5-dim equality, sparse
// selectivity. The caller supplies Rules to scale it.
func BasicMatcher(rules int) Workload

// Run executes the workload against eng inside b.N iterations.
// The caller has already AddRule'd the rules; this just stamps the
// timer and runs Execute.
func Run(b *testing.B, w Workload, eng engine.Engine)

// Populate builds the rule set for w and registers it on eng.
// Returns the input value that Run should feed.
func Populate(eng engine.Engine, w Workload) (input interface{}, err error)
```

The package is generic over `engine.Engine` — no adapter knows or
cares it is being benchmarked. `Populate` builds rules whose
condition shape matches the dimensionality requested (1 / 3 / 5
equality conditions). The matching input value is positioned per
`MatchPosition`.

Two artifacts ship alongside the package:

1. **`BENCHMARKS.md`** at the repo root. Records frozen baseline
   numbers for `inmemory` / `firstmatch` / `priority` across the
   canonical matrix cells. Names the hardware (Apple M-series, Go
   version) the numbers come from. States the v0.8.0 success bar
   from the table above.
2. **`make bench-matrix`** target. Wires the matrix run for local
   execution. The existing `make bench` keeps its per-package scope.

The frozen baselines are advisory, not enforced by CI. `make
bench-matrix` is a developer tool; comparing against the frozen
numbers is a release-readiness check, not a per-commit gate
(noise floor too high). v0.8.0's release prep walks the matrix and
files the comparison in the v0.8.0 CHANGELOG entry.

## Consequences

The library gains a reproducible, adapter-agnostic way to measure
performance. The v0.8.0 indexed matcher starts with the bar already
published, so its release-readiness is checkable, not arguable. The
linear adapters get their numbers pinned at the v0.7.1 mark, which
also serves as a regression anchor for any future change to their
hot paths.

Cost is ~half a release of work: one new package (~150 LOC plus
tests), one `BENCHMARKS.md`, one Makefile target, ~20 baseline
benchmarks frozen against the three existing adapters. No public-API
churn — the `engine.Engine` surface is untouched.

Callers writing custom adapters gain a public benchmark harness. The
contract suite (`enginetest`) tells them their adapter is correct;
the bench suite tells them how it performs. The two together cover
the surface ADR-0009 promised when it locked the `engine.Engine`
port.

A future ADR refines the workload as real-world patterns surface
(non-equality conditions, range queries, mixed dimensionality per
rule). The harness shape stays; the matrix expands. If the v0.8.0
indexed matcher misses the bar, this ADR's table is what we point at
when explaining why v0.8.0 stays unreleased — not as a moving target,
but as the contract that was published before the implementation
existed.
