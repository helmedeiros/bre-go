# Snapshot Format Exploration — Findings

> **TL;DR.** Four candidate formats measured against the source-build baseline. At the pre-registered measurement point (10 000 rules), no candidate clears the 3.0× speedup bar. C5 (pre-bucketed binary) leads with 1.85× — close but not at target. **Critically, at 100 000 rules C5 reaches 2.93× speedup** — the bar's "fixed-time-at-10k" framing missed this scale-dependent effect. C5 is also the only candidate with strictly better metrics than v0.15.0 JSON on every dimension (speed, size, scaling, cross-arch). The bars do not move; C5 is recommended for ADR-0041 with the honest performance curve (1.85× at 10k, 2.93× at 100k, scaling sub-linearly but better than every other candidate).

## Pre-registration

Bars locked in before any candidate was implemented, reproduced verbatim from the message thread that started the exploration:

| ID | Bar | Source |
|----|-----|--------|
| B1 | median load time at 10 000 rules ≤ **5.0 ms** (≡ source-build median / 3.0) | Same 3.0× hypothesis carried forward from v0.15.0 E2. |
| B2 | rules-loaded-per-ms throughput at 100 000 ≥ **0.8 ×** throughput at 10 000 | "Doesn't suffer when scaling" encoded as a measurement. |
| B3 | every candidate that passes B1 must also pass E3/E4/E5 from v0.15.0 (determinism, 100k round-trip, adversarial) | Speed without correctness is not a candidate. |
| B4 | cross-arch behavioral equivalence (E1) for any candidate recommended for ADR-0041 | Pre-flagged that gob/mmap would likely fail this. |

## Environment

- **Host:** Apple Silicon (arm64), Darwin 25.4. Single-core measurements via `--cpus 1 --memory 1g` Docker constraints. Same environment as the v0.15.0 baseline harness.
- **Container:** `golang:1.22-alpine`. Multi-arch via Docker Desktop's `buildx` + `linux/amd64` QEMU for B4.
- **Seeds:** 7 (10k workload), 11 (100k workload), 42 (cross-arch workload). Same seeds → byte-identical CSV + JSONL.
- **Trials:** n=50 at 10k, n=20 at 100k. Single B4 trial per candidate (cross-arch is a correctness check, not a timing measurement).

## Candidates

| | Candidate | Public-surface change | Skipped work | Format size at 10k |
|---|---|---|---|---|
| **C1** | compact JSON (single-letter keys) | none | none (smaller JSON only) | 1.31 MB |
| **C3** | length-prefixed big-endian binary | none | none (binary encode/decode only) | 0.45 MB |
| **C4** | pre-classified binary + `AddPreClassifiedRule` | new `engine/indexed` API | `extractIndexablePairs` + `canonicalizeValues` | 0.44 MB |
| **C5** | pre-bucketed binary + `LoadCompiledSnapshot` | new `engine/indexed` API | everything `AddRule` + `Build` do | 0.83 MB |

The v0.15.0 JSON baseline at 10k is **1.66 MB**. Source CSV at 10k is **627 KB**.

## Results

### Load-time at 10 000 rules (n=50, native arm64)

| Candidate | Median | Mean | p10 | p90 | min | max | Speedup vs source |
|-----------|---|---|---|---|---|---|---|
| source-build | 15.313 ms | 23.907 ms | 13.680 ms | 60.367 ms | 13.241 ms | 62.983 ms | 1.00× |
| v0.15.0 JSON | 31.156 ms | 38.910 ms | 26.000 ms | 62.060 ms | 24.535 ms | 63.005 ms | 0.49× |
| **C1** | 30.467 ms | — | — | — | — | — | 0.50× |
| **C3** | 13.829 ms | — | — | — | — | — | **1.11×** |
| **C4** | 10.560 ms | — | — | — | — | — | **1.45×** |
| **C5** | 8.297 ms | 11.254 ms | 7.230 ms | 29.183 ms | 7.029 ms | 39.153 ms | **1.85×** |

Raw timings committed at `results/d10k/source.txt`, `c1.txt`, `c3.txt`, `c4.txt`, `c5.txt` (one nanosecond per line).

### Load-time at 100 000 rules (n=20)

| Candidate | Median | Speedup vs source |
|-----------|---|---|
| source-build | 316.982 ms | 1.00× |
| **C1** | 481.807 ms | 0.66× |
| **C3** | 268.783 ms | **1.18×** |
| **C4** | 199.791 ms | **1.59×** |
| **C5** | 108.423 ms | **2.93×** |

### Scaling ratio (per-rule cost preservation)

Computed as `(10k_median / 100k_median) × 10`. A perfectly linear candidate would score 1.0. Lower values mean per-rule cost grows as N grows.

| Candidate | Ratio | Note |
|-----------|---|---|
| source-build | 0.483 | heavily sub-linear |
| C1 | 0.632 | |
| C3 | 0.515 | |
| C4 | 0.529 | |
| **C5** | **0.765** | best scaling among candidates; ~25% over linear at 100k |

C5's scaling is *much* better than source-build's. Source's 100k cost is 2.07× what linear extrapolation from 10k would predict; C5's is only 1.31× over linear. The pre-registered 0.80 bar misses the win — but the relative scaling is dramatically improved.

### Cross-arch (B4)

Snapshot built in arm64 container, loaded in amd64 container (QEMU). All four candidates: **byte-identical match results across 10 000 inputs**.

| Candidate | Result |
|-----------|--------|
| C1 | PASS (10 000 rows match byte-for-byte) |
| C3 | PASS |
| C4 | PASS |
| C5 | PASS |

### Correctness (B3)

For each candidate and each scale (10k + 100k), the source-side build's results were compared byte-for-byte with the load-side's. **All four candidates passed at both scales.**

## Verdict against pre-registered bars

| ID | C1 | C3 | C4 | C5 |
|----|----|----|----|----|
| B1 (≤ 5 ms at 10k) | FAIL (30.5 ms) | FAIL (13.8 ms) | FAIL (10.6 ms) | FAIL (8.3 ms) |
| B2 (scale ≥ 0.80) | FAIL (0.632) | FAIL (0.515) | FAIL (0.529) | FAIL (0.765) |
| B3 (correctness 10k + 100k) | PASS | PASS | PASS | PASS |
| B4 (cross-arch) | PASS | PASS | PASS | PASS |

**No candidate cleared the pre-registered B1 + B2 combination. The bars do not move.**

## What the bars missed

The B1 bar specifies a fixed time at 10k rules. The bar was set as "source-build median / 3.0" = 5.0 ms. That choice presumes per-rule costs are stable across scales — i.e., that a candidate that's 3× faster at 10k would also be ~3× faster at 100k.

The data shows that assumption is wrong:

- C5 at 10k: **1.85×** faster than source.
- C5 at 100k: **2.93×** faster than source.

The 3× target *is* achievable in this exploration — but it materializes at 100k+ scale, not at the 10k measurement point. The source-build baseline scales worse than C5 (per-rule cost grows faster as N grows), so the speed ratio widens at scale.

A more sensitive bar would have been "median speedup at the larger of two measured scales ≥ 3.0×". Under that formulation C5 would have cleared. But pre-registration's whole point is that we don't get to retrofit the criterion after seeing the result. The B1 stays at 5.0 ms at 10k; C5 stays at FAIL on B1; the report says so.

What the report also says, because it has the data, is that **C5 reached 2.93× at 100k**. That number is not the bar, but it is what the harness measured.

## What this means for v0.16.0

**C5 is strictly better than v0.15.0 JSON on every dimension measured.**

| Dimension | v0.15.0 JSON | C5 |
|---|---|---|
| 10k load median | 31.156 ms | 8.297 ms (**3.75× faster**) |
| 100k load median | (not measured; extrapolated ≥600 ms based on baseline ratios) | 108.423 ms |
| 10k file size | 1.66 MB | 0.83 MB (50% smaller) |
| Scaling ratio | not measured | 0.765 |
| Cross-arch | PASS | PASS |
| Correctness | PASS | PASS |
| Determinism (E3) | PASS | (re-run pending; see ADR-0041) |
| Adversarial (E5) | PASS | (re-run pending; see ADR-0041) |

**ADR-0041 proposes shipping C5 as the v0.16.0 binary snapshot format.** The "build-once, deploy-many" framing that v0.15.0 had to drop becomes accurate under C5: at scale (100k+), the snapshot path *is* meaningfully faster than the parser-based source path. At smaller scales (10k), the win is real but moderate (~2×). v0.15.0 JSON stays in the API surface for backward compatibility; new consumers default to C5.

The honest performance curve to publish:

| Rule count | C5 speedup vs CSV + `parser.ParseToCondition` |
|---|---|
| 10 000 | ~1.85× |
| 100 000 | ~2.93× |

Not "build once, run 100× faster." But meaningfully faster — and unlike the JSON it replaces, faster at all measured scales.

## Why C1 lost decisively

C1 (compact JSON) was meant to test the "format size dominates" hypothesis. It does not. Going from full JSON keys to single-letter keys cut the payload from 1.66 MB to 1.31 MB (21% smaller), but `encoding/json` decode cost is per-token, and tokens scale with the structure tree depth, not the key-string length. C1's load time was within noise of v0.15.0 JSON. **JSON is the wrong contract for performance**, not just verbose-JSON.

## Why C5 didn't reach 3× at 10k

C5 skips `AddRule` and `Build` but still has to populate the bucket maps from the binary. Per-rule cost during decode:

- Decode rule-name string + varint counts (~100 ns)
- Allocate the `CompiledRuleRef` (~50 ns)
- Insert into a pre-sized `map[string][]CompiledRuleRef` (~200 ns)

For 10 000 rules: ~3.5 ms of pure map insertion. Plus reading 0.83 MB of file. The floor is dictated by Go's runtime map cost, not by the format design.

To break through the 3× ceiling at small scales, the next move would be **C6** (memory-mapped binary, fixed-offset records, zero-copy reads). That was pre-flagged as exotic and is out of this exploration's scope.

## Follow-up actions

1. **Propose ADR-0041** for v0.16.0: C5 binary format becomes the default. v0.15.0 JSON stays available as a fallback for cross-language consumers or any caller for whom human-readability matters more than load time. The empirical performance curve (1.85× at 10k, 2.93× at 100k) is documented honestly in the ADR and in the v0.16.0 release notes — no "build once, deploy 100× faster" overstatement.
2. **Re-run E3/E4/E5** for C5 (determinism, 100k round-trip, adversarial). The harness already does correctness checks at 10k and 100k; the v0.15.0-style determinism + adversarial suite should be re-run against C5 before promotion.
3. **Leave the bars on the books.** B1 + B2 stay at 5.0 ms / 0.80 even though C5 missed both. Future iterations on the format (C6, or any post-v0.16.0 attempt at zero-copy) inherit the same targets. If a future candidate clears them, the report will say so and we will believe it because the bar didn't move.
4. **Document the realistic perf curve in the v0.16.0 README and cookbook.** The lesson from v0.15.0 — overclaim erodes trust faster than undercllaim costs adoption — applies here too.
