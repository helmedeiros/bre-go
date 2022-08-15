# Snapshot Format Exploration (post-v0.15.0)

Companion harness to `scientific/v0.15.0/`. After v0.15.0's E2 failed (snapshot load 0.49× vs source-build), this directory explores four alternative formats to see which, if any, can deliver the 3× speedup the original ADR-0040 framing implied. Headline result: see [REPORT.md](REPORT.md). C5 (pre-bucketed binary) leads at 1.85× at 10k and 2.93× at 100k — best of the four, technically misses the pre-registered 5.0 ms bar but is strictly better than v0.15.0 JSON on every measured dimension.

## Candidates

| | Format | Engine API change | What gets skipped at load |
|---|---|---|---|
| C1 | compact JSON (single-letter keys) | none | none — format size only |
| C3 | length-prefixed big-endian binary | none | none — format encoding only |
| C4 | C3 + pre-classified rule encoding | `indexed.AddPreClassifiedRule` | `parser.Condition` walk + value canonicalize |
| C5 | C3 + pre-bucketed snapshot | `indexed.ExportCompiledSnapshot` + `indexed.LoadCompiledSnapshot` | `AddRule` (fan-out, bucket insert) + `Build` |

## Reproducing

```sh
bash scientific/v0.15.0/experimental/scripts/run-experimental.sh
```

Knobs (with defaults):

- `TRIALS_10K=50` — n at 10 000 rules
- `TRIALS_100K=20` — n at 100 000 rules
- `SPEEDUP_BAR_MS=5.0` — pre-registered B1 threshold (median load time at 10k)
- `SCALE_RATIO_BAR=0.80` — pre-registered B2 throughput ratio at 100k vs 10k

The orchestrator builds two Docker images (arm64 native, amd64 emulated), runs each candidate at both scales with their pre-registered trial counts, runs cross-arch correctness, computes per-candidate verdict, writes `results/COMPARE.txt`, and exits 0 iff any candidate clears B1+B2+B3.

Expect ~5 minutes wall time on native arm64; ~15 minutes including QEMU's amd64 emulation overhead.

## Pre-registered bars

These were set before any candidate was implemented:

- **B1** (speed): median load time at 10k rules ≤ 5.0 ms.
- **B2** (scale): throughput ratio 100k / 10k ≥ 0.80.
- **B3** (correctness): byte-identical load results vs source-execute at both scales.
- **B4** (cross-arch): byte-identical load results when built on arm64 and loaded on amd64.

The bars do not move. Mid-run thresholds set to match a particular candidate's measured number would invalidate the experiment.

## Output

- `results/COMPARE.txt` — the table from a run. Per-candidate verdict + raw medians.
- `results/d10k/*.txt` — per-candidate raw nanosecond timings (n=50). Same for `d100k/*.txt` (n=20).
- `results/REPORT.md` — written by hand based on the latest run's data (this file is `REPORT.md` in the parent).

## CI

`.github/workflows/scientific.yml` is the same workflow that runs the v0.15.0 baseline suite. The experimental orchestrator could be wired in as a second job by setting `release: v0.15.0/experimental` in the `workflow_dispatch` input — left as a follow-up for whoever lands ADR-0041.

## What we didn't try

- **C2 (gob)** — pre-flagged as non-portable (Go-specific). Likely would have measured between C1 and C3 on speed.
- **C6 (mmap / zero-copy)** — exotic. Likely could break the 3×@10k ceiling. Out of scope until a real consumer needs to.
- **Compressed JSON.** Well-known to be CPU-bound; unlikely to help on this workload.
