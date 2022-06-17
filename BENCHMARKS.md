# Benchmarks

Frozen baseline numbers and pre-committed performance targets for adapter comparisons. See [ADR-0031](docs/architecture/decisions/0031-adapter-benchmark-harness.md) for the rationale.

Reproduce locally with `make bench-matrix`. Comparing two runs is best done with [`benchstat`](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

## Test environment for the frozen numbers

- **Hardware**: Apple M4
- **OS**: macOS (`darwin/arm64`)
- **Go**: 1.18 toolchain (project minimum)
- **Build**: `go test -run=^$ -bench=. -benchmem`
- **Date**: 2022-06-06

These numbers are recorded at the **v0.7.1 cut**, before any indexed-matcher work. They are advisory baselines — not enforced by CI — because benchmark numbers carry a noise floor that would produce false positives if gated per-commit.

## Frozen baselines (v0.7.1)

### `BasicMatcher` workload — 5-dimensional, Sparse selectivity, Last position

| Rules | `firstmatch` | `inmemory` | `priority` |
|---:|---:|---:|---:|
| 10 | 636 ns/op | 648 ns/op | 783 ns/op |
| 100 | 4 235 ns/op | 4 244 ns/op | 5 126 ns/op |
| 1 000 | 39 813 ns/op | 42 459 ns/op | 48 019 ns/op |
| 10 000 | 395 938 ns/op | 414 915 ns/op | 523 214 ns/op |

The growth is linear in rule count for all three adapters — none of them index, so each `Execute` walks the whole rule list.

### 1 000 rules, 5 dimensions, Unique selectivity

| Position | `firstmatch` | `inmemory` | `priority` |
|---|---:|---:|---:|
| `First` (best case) | 282 ns/op | 40 313 ns/op | 10 666 ns/op |
| `Last` | 40 665 ns/op | 40 175 ns/op | 49 444 ns/op |
| `NoHit` (worst case) | 39 516 ns/op | 39 845 ns/op | 52 123 ns/op |

`firstmatch` benefits massively from early-exit at `First` (282 ns). `inmemory` walks all rules regardless. `priority` walks all rules in priority order with extra per-rule allocation.

### 10 000 rules, 5 dimensions, Unique selectivity

| Position | `firstmatch` | `inmemory` | `priority` |
|---|---:|---:|---:|
| `Last` | 401 946 ns/op | 399 268 ns/op | 526 324 ns/op |
| `NoHit` | 397 804 ns/op | 399 706 ns/op | 523 017 ns/op |

### 10 rules, 5 dimensions, Unique selectivity, Last position (anti-regression cell)

| `firstmatch` | `inmemory` | `priority` |
|---:|---:|---:|
| 650 ns/op | 666 ns/op | 791 ns/op |

This is the cell the indexed adapter must **not** regress on — small rule sets pay the index's fixed overhead without amortizing it.

## v0.8.0 success bar — indexed matcher — ✅ CLEARED

The indexed adapter must clear every row below before it ships as `v0.8.0`. The four `TestSuccessBar_*` tests in `engine/indexed/success_bar_test.go` enforce this live, comparing firstmatch and indexed in the same run so the ratios are immune to hardware drift.

| Cell | `firstmatch` baseline | Required multiplier | Indexed live | Result |
|---|---:|---:|---:|:---:|
| 1k rules, 5 dims, `NoHit` | 39 516 ns/op | **≥ 10× faster** | ~155 ns/op (~254×) | ✅ |
| 1k rules, 5 dims, `Last` | 40 665 ns/op | ≥ 5× faster | ~176 ns/op (~230×) | ✅ |
| 10k rules, 5 dims, `NoHit` | 397 804 ns/op | **≥ 50× faster** | ~155 ns/op (~2 625×) | ✅ |
| 10 rules, 5 dims, `Last` | 650 ns/op | within 2× (anti-regression) | ~172 ns/op (0.26× slowdown — *faster*) | ✅ |

The indexed adapter is sub-linear: it runs at roughly the same time regardless of rule count (the `NoHit` rows are essentially constant), because the lookup is O(K) hash probes where K is the number of distinct key-sets — typically a small constant. At small rule counts it is also faster than `firstmatch` in absolute terms because both adapters do a similar amount of work but the indexed one avoids the linear scan altogether.

The `priority` and `inmemory` baselines are listed above for completeness; the bar compares only against `firstmatch` (the fairest competitor, since both adapters return on first match).

### Indexed-adapter baselines (v0.8.0)

| Workload | Indexed ns/op |
|---|---:|
| `BasicMatcher(10)` | ~335 ns |
| `BasicMatcher(100)` | ~350 ns |
| `BasicMatcher(1 000)` | ~366 ns |
| `BasicMatcher(10 000)` | ~382 ns |

Note the flat shape — the cost of `Execute` barely moves as the rule count grows by three orders of magnitude. That is the indexed-matcher win the design promised.

## How to compare a new adapter

1. Write a `Factory` that returns the new adapter and a `SeedFunc` mapping `bench`'s standard `(name, condition)` shape into the adapter's `Rule` struct.
2. Add a sub-benchmark in `engine/enginetest/bench/matrix_bench_test.go` (or in a separate `_bench_test.go` file in the new adapter's package) that calls `bench.Run(b, w, newFactory)` across the same matrix cells used above.
3. Run `make bench-matrix > new.txt` against the current baseline, save it, then run again with the new adapter and diff with `benchstat baseline.txt new.txt`.

The numbers will move with hardware and Go version. The *ratios* between adapters on the same machine in the same run are the stable comparison.
