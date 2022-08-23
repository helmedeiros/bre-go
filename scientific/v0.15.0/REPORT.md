# v0.15.0 Snapshot Scientific Validation Report

> **TL;DR.** Six of seven pre-registered experiments pass. E2 (load-time speedup) fails at 0.49× median — the JSON snapshot path is roughly twice as *slow* as the CSV + `parser.ParseToCondition` baseline at 10k rules. The snapshot feature still ships in v0.15.0; its value is determinism, cross-architecture behavioral equivalence, scale-to-100k correctness, refusal-path integrity, and adversarial robustness, all of which were proven. The "build-once, deploy-many cuts load time" framing in ADR-0040, CHANGELOG, README, and cookbook was wrong on the speed dimension and gets revised in the same release window as this report.
>
> **Follow-up (v0.16.0):** the load-time gap surfaced here drove the experimental exploration at [`scientific/v0.15.0/experimental/`](experimental/REPORT.md). Four candidate formats were measured under the same pre-registered conditions; C5 (pre-bucketed binary) reached **1.85× faster than source-build at 10k rules and 2.93× at 100k**. It shipped as the v0.16.0 binary snapshot path ([ADR-0041](../../docs/architecture/decisions/0041-binary-snapshot-format.md)). The v0.15.0 JSON format stays available for cross-language and human-readable use cases; the v0.16.0 binary format is the right choice when load time matters.

## Pre-registration

Bars locked in before the harness was wired up; reproduced here verbatim so a future reader can verify nothing moved:

| ID | Hypothesis | Bar |
|----|------------|-----|
| E1 | Loaded engines produce identical match results across architectures. | zero byte-level differences across 10k inputs in either direction |
| E2 | Snapshot load is meaningfully faster than CSV + `parser.ParseToCondition` rebuild. | median speedup (source ÷ snapshot) ≥ 3.0× at 10k rules |
| E3 | Building the same rule set twice in independent processes produces byte-identical snapshots. | `diff -q` equal |
| E4 | Round-trip works at 100k rules. | every match result identical across 1000 sampled inputs |
| E5 | Adversarial round-trip (deep AND, unicode, all-Inf bounds, pointer variants, special-char values, 500-element OpIn) holds. | all checks identical |
| E6 | Tampered `formatVersion` and malformed conditions refuse to load. | declared error sentinels match (`Is(err, sentinel)`) |
| E7 | Hook-bearing engines refuse to export. | `ErrSnapshotIncompatibleHook` returned both pre- and post-Build |

## Environment

- **Host:** Apple Silicon (arm64), Darwin 25.4. Single-core measurements via `--cpus 1 --memory 1g` Docker constraints.
- **Container base:** `golang:1.22-alpine`. Same image for arm64 and amd64; amd64 runs under QEMU emulation via Docker Desktop's binfmt_misc.
- **bre-go module:** `replace`d to the workspace checkout at the commit being validated.
- **Cross-arch policy:** E1 timings are *not* compared (QEMU adds non-negligible overhead). E2 timings run on native arm64 only.
- **Reproducibility:** seeds fixed per experiment (E1=42, E2=7, E3=99, E4=11). Same seed → byte-identical CSV + JSONL.

## E1 — Cross-architecture behavioral equivalence

**Setup.** Generate 10k rules + 10k inputs deterministically in container A. Source-build the engine and export `snapshot.json`. Source-build the same engine and execute inputs to produce `results-source.jsonl` (the reference). In container B (different architecture), `LoadSnapshot` the JSON and execute the same inputs to produce `results-snapshot-<arch>.jsonl`. `sci-compare` byte-diffs the two results files.

Run both directions: arm64 → amd64 and amd64 → arm64.

**Result.** PASS.

```
compare: 10000 rows match byte-for-byte    [arm64 builder -> amd64 loader]
compare: 10000 rows match byte-for-byte    [amd64 builder -> arm64 loader]
```

**Interpretation.** The snapshot JSON is portable across architectures. No byte order, no native float-formatting leak, no architecture-specific map-iteration nondeterminism affecting the output. This is the operational contract the feature *promises*, and it holds at 10k rules.

## E2 — Load-time speedup (failed pre-registered bar)

**Setup.** Generate 10k rules with seed 7. Run two timing experiments in the same container, n=50 trials each, native arm64, single CPU pinned, memory capped:

- **Baseline (source-build):** read `rules.csv`, `parser.ParseToCondition(expr)` per row, `AddRule` per row, `Build`.
- **Candidate (snapshot-load):** read `snapshot.json`, `json.Decode`, `LoadSnapshot`.

Both paths end at `engine.Built() == true`. Timing brackets the entire path.

**Raw timings.** Committed at `results/e2/source-timings.txt` and `results/e2/snapshot-timings.txt`, one nanosecond per line.

**Result.** **FAIL.**

```
source-build       n=50 mean=25.752ms median=15.320ms p99=61.440ms stddev=18.231ms min=12.858ms max=62.784ms
snapshot-load      n=50 mean=38.910ms median=31.156ms p99=62.060ms stddev=12.861ms min=24.535ms max=63.005ms
speedup (source-build / snapshot-load): median=0.49x mean=0.66x
verdict: FAIL (median 0.49x < 3.00x)
```

The snapshot path is **2.0× slower** at the median than the source-build path.

### Why

The hypothesis assumed snapshot loading "skips the parse cost." It does — but the parse cost it skips is bre-go's parser, which is a hand-rolled recursive-descent over `==`, `!=`, `IN`, `NOT IN`, `AND`, `OR`, `NOT`. For the expression shapes the harness generates (single equality, two-term AND, three-term AND with one negation, IN with 2-5 values), `parser.ParseToCondition` runs in microseconds per rule. The CSV source for 10k rules is 627 KB.

Meanwhile, the snapshot JSON for the same rule set is **2.6× larger** (1.6 MB) because each tagged-union condition carries `"type":"and"`, `"type":"string"`, `"field":"…"`, `"op":"…"`, `"value":"…"` — verbose by JSON's nature. Decoding 1.6 MB of nested JSON into Go structs, then walking the tagged union to reconstruct typed `parser.Condition`s, then funneling each through `AddRule` (which canonicalizes values, fans out OpIn, and inserts into buckets) costs more than the source path costs to do less. The dominant work — `AddRule` itself — is the *same* in both paths. The difference is what comes before: cheap CSV+parse vs. verbose JSON-decode-and-walk.

The snapshot path saves the parse cost only when source-side parsing is the dominant work. For bre-go's parser at this rule complexity, it isn't.

### What this means for v0.15.0

The "build-once, deploy-many cuts startup time" framing in ADR-0040 §1.1, the v0.15.0 CHANGELOG, the v0.15.0 README banner, and the cookbook's "Build once, deploy many with snapshots" entry is wrong for the parser bre-go ships. Those passages are revised in the same release window as this report to:

- Drop the "skip every parse / validate / canonicalize cost" claim.
- Lead with the dimensions that *did* pass: determinism, portability, refusal-path integrity, adversarial round-trip.
- Note explicitly that a future ADR for a compact binary format is the right hammer if a real consumer comes back with a load-time requirement that this report's measurements miss.

### What this means for the bar

**The bar does not move.** Pre-registration's only purpose is to prevent exactly this — picking a threshold to match the result. The result is what it is. The methodology is what it is. The feature ships with its real value, not its imagined value.

### Limits of this measurement

- Single host. Different storage / different filesystem caching / different Go runtimes might shift the absolute numbers, but the relative ratio is dominated by JSON-decode cost vs. CSV+parse cost — which is a property of the formats, not the host.
- Single parser. A consumer using a more expensive parser (a full DSL with a heavy grammar) would see the relative ratio shift in snapshot's favor. The harness does not measure that case because it would mean shipping a synthetic parser that isn't part of bre-go.
- Single rule complexity profile. The generator emits realistic but moderate-complexity rules (2-4 conditions per rule, OpIn cardinality 2-5). Synthetic worst cases (10-deep AND nesting, 1000-element OpIn) might push parser cost higher, but those don't reflect typical production rule sets either.

The honest conclusion: **for bre-go's parser at typical scales (10k–100k rules with typical expression complexity), the v0.15.0 JSON snapshot does not provide a load-time speedup.** It does provide everything else the experiments tested for.

## E3 — Determinism

**Setup.** Two independent processes, same seed (99), each runs `scigen` followed by `sci-build` with `-snapshot`. `diff -q` the two snapshot files.

**Result.** PASS — `snapshot.json` files are byte-identical.

**Interpretation.** Snapshots are content-addressable: you can checksum, deduplicate, and cache them safely. A build artifact for v0.15.0 is identifiable by a hash of its bytes.

## E4 — Scale to 100k rules

**Setup.** Generate 100k rules + 1000 inputs (seed 11). Source-build the engine, export `snapshot.json`. Load the snapshot in a separate invocation. Execute the 1000 inputs against both engines. Compare results.

**Result.** PASS.

```
compare: 1000 rows match byte-for-byte
E4 artifacts: snapshot.json size 16625922 bytes; rules.csv size 6416996 bytes
```

**Interpretation.** The 100k-rule snapshot is 15.9 MB and round-trips with zero result drift. Export, JSON marshaling, JSON unmarshaling, and `LoadSnapshot` all succeed at this scale. The file is well within e-mail-attachment territory, comfortably within S3-object territory.

## E5 — Adversarial round-trip

**Setup.** `cmd/adversarial` constructs 11 hand-built rule shapes designed to stress the encoder and decoder:

1. AndCondition nested 10 levels deep.
2. Rule name with Brazilian flag emoji, snowman, and accented Latin characters.
3. Field and value with Latin extended characters (`país`, `Brésił`).
4. Range condition with finite degenerate bounds (`Min == Max == 0`).
5. Range condition with `-Inf` lower bound, finite upper.
6. Range condition with finite lower, `+Inf` upper.
7. Range condition with both bounds at infinity.
8. AndCondition mixing pointer variants of String/Set/Range conditions.
9. SetCondition with 500-element OpIn (large fan-out path).
10. String value containing escaped quotes, backslashes, and literal `\n` `\t` sequences.
11. Rule with a 1 KB Description and an 8-element Tags slice, verifying preservation.

Each case round-trips Export → `json.Marshal` → `json.Unmarshal` → `LoadSnapshot`, then executes a fixed input table against both the original and loaded engines.

**Result.** PASS.

```
adversarial: cases=11 checks=28 failures=0
```

**Interpretation.** Every corner case the harness constructs round-trips identically. The infinity-bound encoding via `strconv.FormatFloat(±Inf)` → `"+Inf"`/`"-Inf"` and reverse via `strconv.ParseFloat` is the load-bearing piece for ranges; it works in both directions at every corner.

## E6 + E7 — Refusal-path integrity

**Setup.** `cmd/refusals` exercises seven declared refusal paths:

| Path | Expected sentinel |
|------|-------------------|
| `formatVersion: 99999` in memory | `ErrSnapshotFormatVersionMismatch` |
| Range Min `"not-a-float"` | `ErrSnapshotMalformed` |
| Unknown `Type: "no-such-type"` | `ErrSnapshotMalformed` |
| `formatVersion: 42` on disk after round-trip through JSON | `ErrSnapshotFormatVersionMismatch` |
| Hook-bearing engine, pre-Build, calling `ExportSnapshot` | `ErrSnapshotIncompatibleHook` |
| Hook-bearing engine, post-Build, calling `ExportSnapshot` | `ErrSnapshotIncompatibleHook` |
| Empty engine, calling `ExportSnapshot` | `ErrSnapshotEmpty` |

Each check uses `errors.Is(got, want)`.

**Result.** PASS — all 7 checks return their declared sentinels.

**Interpretation.** The refusal contracts in ADR-0040 (no migration shims, no hook-aware encoding, no silent acceptance of empty engines or unknown types) are real, not aspirational. A hand-edited or malformed snapshot fails loudly and predictably.

## Verdict summary

| ID | Result | Notes |
|----|--------|-------|
| E1 | PASS | both directions byte-identical |
| **E2** | **FAIL** | **0.49× median (target 3.00×) — bar does not move; documentation gets revised** |
| E3 | PASS | byte-identical across independent builds |
| E4 | PASS | 100k rules round-trip cleanly |
| E5 | PASS | 11 cases × 28 checks identical |
| E6 + E7 | PASS | 7/7 refusal paths fire correctly |

## Follow-up actions

1. **Revise the v0.15.0 documentation** to drop load-time speedup language and lead with the dimensions that the harness validated. Same release window as this report. **Done.**
2. **Open a follow-up ADR** for a compact binary snapshot format if a real consumer comes back with a load-time requirement this measurement does not meet. **Done — became [ADR-0041](../../docs/architecture/decisions/0041-binary-snapshot-format.md), v0.16.0.** The exploration at [`scientific/v0.15.0/experimental/`](experimental/REPORT.md) measured four candidate formats; the pre-bucketed binary (C5) reached 1.85× at 10k and 2.93× at 100k, and was promoted to the public `engine/indexed.MarshalCompiledSnapshot` / `UnmarshalCompiledSnapshot` API.
3. **Treat the bar as standing.** Future work that touches either snapshot format re-runs this harness AND `scientific/v0.15.0/experimental/scripts/run-experimental.sh`. E2 stays on the books at the 3.0×-at-10k bar; v0.16.0 closed the 100k gap (2.93×) but missed the 10k bar (1.85×). The remaining headroom would need a zero-copy / memory-mapped format (out of scope until a consumer asks).
