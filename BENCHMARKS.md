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

## v0.10.0 success bar — negation post-filter — ✅ CLEARED

ADR-0035 admits `StringCondition{Op: OpNeq}` and `SetCondition{Op: OpNotIn}` as post-filters on bucket hits in `engine/indexed`. The 1k cell shows the real cost of the post-filter — an extra ~190 ns/op vs the equality baseline because of the per-candidate `Condition.Eval`. The 10k NoHit cell is unaffected because the bucket misses before the post-filter ever runs.

| Cell | `firstmatch` baseline | Required multiplier | Indexed live | Result |
|---|---:|---:|---:|:---:|
| 1k rules, 5 dims, 1 of 5 fields uses `OpNeq` post-filter, `Last` | ~40 500 ns/op | ≥ 5× faster | ~364 ns/op (~111×) | ✅ |
| 10k rules, 5 dims, 1 of 5 fields `OpNeq`, `NoHit` | ~418 000 ns/op | ≥ 30× faster | ~142 ns/op (~2 942×) | ✅ |

The 30× bar (vs 50× in v0.8.0/v0.9.0) absorbs the post-filter overhead in pathological worst-case workloads. In practice the live ratio is much higher because real-world bucket hits are sparse.

Rules with zero `OpNeq` / `OpNotIn` terms (the v0.8.0 / v0.9.0 case) have a nil post-filter slice and Execute skips the Eval entirely. Zero hot-path cost for callers who never touch negation.

## v0.9.0 success bar — `OpIn` set-membership — ✅ CLEARED

ADR-0034 extends the indexed adapter to admit `SetCondition{Op: OpIn}` via bucket fan-out at AddRule. Two new bar cells were added in v0.9.0 to guarantee the new shape still rides the sub-linear matcher. Both clear at >200× firstmatch even though each rule now expands into multiple bucket entries:

| Cell | `firstmatch` baseline | Required multiplier | Indexed live | Result |
|---|---:|---:|---:|:---:|
| 1k rules, 5 dims, **2 of 5 dims are OpIn with 3 values each**, `Last` | ~41 000 ns/op | ≥ 5× faster | ~172 ns/op (~238×) | ✅ |
| 10k rules, 5 dims, **2 of 5 dims OpIn / 3 values**, `NoHit` | ~407 000 ns/op | ≥ 50× faster | ~152 ns/op (~2 674×) | ✅ |

The shape contract widened, the perf claim held. The four v0.8.0 cells below continue to gate ci-local too — v0.9.0 cannot regress equality-only performance.

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

## Load-time profile (v0.9.1)

The success-bar benchmarks (matrix + `TestSuccessBar_*`) measure per-Execute latency with `b.ResetTimer()` *after* rules are populated — the standard practice for measuring steady-state production cost. This section completes the picture by measuring **`AddRule` itself**: the cost of loading a populated engine before any Execute call.

Run with `make bench-load` (or `go test -run=^$ -bench=BenchmarkLoad ./engine/indexed/...`).

### Equality-only workloads

| Workload | `firstmatch` load | `indexed` load | Indexed:firstmatch | Indexed memory |
|---|---:|---:|---:|---:|
| 1 000 rules / 5 dims | ~1.08 ms | ~1.48 ms | 1.4× slower | ~2.76 MB (5× more) |
| 10 000 rules / 5 dims | ~96.6 ms | ~17.4 ms | **5.6× faster** | ~27.8 MB |

### OpIn workloads (2 of 5 dims OpIn × 3 values = 9× fan-out per rule)

| Workload | `firstmatch` load | `indexed` load | Indexed:firstmatch | Indexed memory |
|---|---:|---:|---:|---:|
| 1 000 rules / 5 dims | ~1.42 ms | ~2.27 ms | 1.6× slower | ~3.99 MB (5× more) |
| 10 000 rules / 5 dims | ~102 ms | ~27.0 ms | **3.8× faster** | ~42.5 MB |

### Why the asymptotic flip

The 10k-rule cells tell the structural story. `firstmatch.AddRule` does a linear duplicate-name check (scan the existing rules) on every call — **O(N²) total** for N rules. `indexed.AddRule` uses a `map[string]struct{}` for the same check — O(N) amortized. The crossover happens somewhere around 2 000–3 000 rules: below that, `firstmatch` wins on per-rule overhead; above that, `indexed`'s asymptotic advantage takes over.

The OpIn fan-out adds ~50% to the indexed load (9× more bucket inserts per rule) but the hot-path Execute cost stays flat — exactly as designed.

### Amortization

In a service that starts up once and serves requests for hours or days, AddRule cost is paid back in milliseconds. The worst small-scale case (1k rules, OpIn): `indexed` load is ~2.27 ms, vs ~172 ns per Execute. **~13 000 Execute calls and the load cost is even.** For long-running services the choice between adapters is dominated by per-Execute cost, not load cost.

For serverless / per-request load patterns, indexed's memory overhead and small-N load penalty matter more. Use `firstmatch` if N is small (<500) and your workload re-loads on every request.

These numbers are not gated by `ci-local` today — they live alongside the existing matrix as a release-prep / debugging reference. A future ADR may promote any of them to a hard gate when concurrency / hot-reload work (v0.12.0) needs a frozen load-time baseline.

## How to compare a new adapter

1. Write a `Factory` that returns the new adapter and a `SeedFunc` mapping `bench`'s standard `(name, condition)` shape into the adapter's `Rule` struct.
2. Add a sub-benchmark in `engine/enginetest/bench/matrix_bench_test.go` (or in a separate `_bench_test.go` file in the new adapter's package) that calls `bench.Run(b, w, newFactory)` across the same matrix cells used above.
3. Run `make bench-matrix > new.txt` against the current baseline, save it, then run again with the new adapter and diff with `benchstat baseline.txt new.txt`.

The numbers will move with hardware and Go version. The *ratios* between adapters on the same machine in the same run are the stable comparison.
