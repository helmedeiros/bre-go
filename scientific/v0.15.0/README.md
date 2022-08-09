# Scientific Harness for v0.15.0 (Snapshot Export / Import)

This directory contains the pre-registered scientific validation suite for the `engine/indexed.Engine.ExportSnapshot()` / `LoadSnapshot()` feature shipped in v0.15.0. The headline finding lives in [REPORT.md](REPORT.md); this README is the operator's manual.

## What gets measured

Seven experiments, each with a pre-registered success criterion that cannot be moved after the fact:

| ID | Experiment | Bar |
|----|------------|-----|
| E1 | Cross-architecture behavioral equivalence (10k rules × 10k inputs, arm64 ↔ amd64) | zero byte-level differences in either direction |
| E2 | Load-time speedup, n=50 trials, 10k rules, native architecture | median speedup ≥ 3× (source-build ÷ snapshot-load) |
| E3 | Determinism: same seed → same snapshot in two independent processes | byte-identical snapshot files |
| E4 | Scale: 100k rules round-trip Export → Load → Execute | every match result identical across 1000 inputs |
| E5 | Adversarial round-trip: 11 corner-case rule shapes (deep AND nesting, unicode names, all-Inf bounds, pointer variants, special-char values, etc.) | all 28 input checks identical |
| E6 | Format-version refusal: mutated `formatVersion`, malformed `Min`/`Max`, unknown condition types | all refusal sentinels match (`ErrSnapshotFormatVersionMismatch`, `ErrSnapshotMalformed`) |
| E7 | Hook-incompatibility refusal: hook-bearing engine pre- and post-Build | `ErrSnapshotIncompatibleHook` on both |

## Repository layout

```
scientific/v0.15.0/
├── cmd/
│   ├── scigen/         deterministic seed-driven rule + input generator
│   ├── build/          source-load baseline (CSV + parser.ParseToCondition + AddRule + Build)
│   ├── load/           snapshot-load consumer (JSON decode + LoadSnapshot)
│   ├── compare/        byte-level results.jsonl diff
│   ├── stats/          mean/median/p99/stddev + speedup ratio + bar check
│   ├── adversarial/    11 corner-case round-trip checks (E5)
│   └── refusals/       7 declared-sentinel checks (E6 + E7)
├── schema/             shared on-disk formats (CSV source, JSONL inputs, JSONL results)
├── scripts/
│   └── run-all.sh      orchestrator: builds images, runs E1–E7, writes SUMMARY.txt
├── Dockerfile          pinned golang:1.22-alpine, builds all eight binaries
├── docker-compose.yml  per-service CPU/memory caps for timing stability
├── results/            raw timings + reports + per-experiment logs (committed)
├── REPORT.md           the report with the actual numbers and conclusions
└── README.md           this file
```

## Reproducing

Prerequisites:
- Docker 20.10+ with buildx (cross-arch via QEMU; Docker Desktop on macOS has this preconfigured)
- ~2 GB free disk for build cache + result artifacts
- ~10 minutes wall time on native arm64

From the repo root:

```sh
bash scientific/v0.15.0/scripts/run-all.sh
```

The orchestrator:
1. Builds `bre-go-sci-v0.15.0:arm64` and `:amd64` images (latter under QEMU on Apple Silicon).
2. Runs every experiment in order, capturing raw output under `results/`.
3. Writes `results/SUMMARY.txt` with one line per experiment plus a final verdict.
4. Exits 0 iff every experiment hit its pre-registered bar; otherwise 1.

Environment knobs (defaults shown):
- `TRIALS_E2=50` — number of timing trials for E2 (set higher for tighter intervals)
- `RULES_E2=10000` — rule count for E1, E2, E3
- `INPUTS_E2=10000` — input count for E1, E2
- `RULES_E4=100000` — rule count for E4
- `INPUTS_E4=1000` — input count for E4
- `SPEEDUP_BAR=3.0` — pre-registered minimum median speedup for E2

Bars are pre-registered. The orchestrator reports failures honestly. **Do not edit the bar after a failing run** — the point of pre-registration is that moving the bar invalidates the experiment.

## What "fake" would look like — and how this isn't

A non-scientific "validation" would:
- Run timings once and quote the number.
- Use a baseline so cheap or so expensive that the result is foregone.
- Compare snapshot-load to itself (in-process round-trip) and call it portability.
- Hide the raw numbers.
- Move the bar after seeing the result.

This harness:
- Runs n=50 trials per timing measurement, reports the full distribution (mean / median / p99 / stddev / min / max), and commits the raw nanosecond-per-line files at `results/e2/source-timings.txt` and `results/e2/snapshot-timings.txt`. Anyone can reanalyze them.
- The baseline is `parser.ParseToCondition` + `AddRule` + `Build` — the same code paths a real consumer writes today using bre-go's parser. Not steel-manned, not strawmanned.
- Cross-arch equivalence is a Docker-mediated transfer: snapshot built in container A on one architecture, loaded in container B on another, compared via byte-level diff.
- The pre-registered bar fired at 3.0×. The observed speedup is 0.49× (snapshot is *slower*). The result is reported as a failure, and the v0.15.0 documentation is revised to drop the load-time claim. The feature still ships — its value lives elsewhere — but the false claim does not.

The output files under `results/` (timing distributions, per-experiment logs, build-time output) are the evidence anyone can audit.

## CI integration

`.github/workflows/scientific.yml` runs this suite on every tag push and on manual `workflow_dispatch`. Results upload as a build artifact. If any pre-registered bar misses, the job fails.

Local runs are faster and don't depend on hosted-runner availability; CI is the regression net.
