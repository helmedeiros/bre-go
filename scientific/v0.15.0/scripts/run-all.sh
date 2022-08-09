#!/usr/bin/env bash
# Orchestrator for the v0.15.0 snapshot scientific harness.
#
# Pre-registered bars (must be set before running, do not move after):
#   E2: median speedup (source-build / snapshot-load) >= 3.00
#   E1: zero byte-level differences across 10k inputs in both
#       arch directions
#   E3: snapshot byte-identical across two independent builds
#   E4: 100k-rule round-trip with zero result differences
#   E5: all adversarial cases pass
#   E6 + E7: all refusal paths return their declared sentinels
#
# Outputs:
#   results/                 raw timings + result files + JSON reports
#   results/SUMMARY.txt      human-readable verdict per experiment
#
# Exit code:
#   0  every experiment passed its pre-registered bar
#   1  at least one experiment failed (do NOT change the bar; revise
#      the claim instead)

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
SCI_DIR="$ROOT/scientific/v0.15.0"
RESULTS="$SCI_DIR/results"

ARM_IMAGE="bre-go-sci-v0.15.0:arm64"
AMD_IMAGE="bre-go-sci-v0.15.0:amd64"

TRIALS_E2="${TRIALS_E2:-50}"
RULES_E2="${RULES_E2:-10000}"
INPUTS_E2="${INPUTS_E2:-10000}"
RULES_E4="${RULES_E4:-100000}"
INPUTS_E4="${INPUTS_E4:-1000}"
SPEEDUP_BAR="${SPEEDUP_BAR:-3.0}"

mkdir -p "$RESULTS"
SUMMARY="$RESULTS/SUMMARY.txt"
: > "$SUMMARY"

log()  { printf '\n=== %s ===\n' "$*" | tee -a "$SUMMARY" >&2; }
note() { printf '%s\n' "$*"          | tee -a "$SUMMARY" >&2; }

run_native() { docker run --rm --cpus 1 --memory 1g -v "$RESULTS:/data" "$ARM_IMAGE" -c "$1"; }
run_amd64()  { docker run --rm --platform linux/amd64 --cpus 1 --memory 1g -v "$RESULTS:/data" "$AMD_IMAGE" -c "$1"; }

failed=0

# ---------------------------------------------------------------- build
log "BUILD: harness images"
docker build -f "$SCI_DIR/Dockerfile" -t "$ARM_IMAGE" "$ROOT" >>"$RESULTS/docker-build.arm64.log" 2>&1 \
  || { note "arm64 build failed (see results/docker-build.arm64.log)"; exit 1; }
docker buildx build --platform linux/amd64 -f "$SCI_DIR/Dockerfile" -t "$AMD_IMAGE" --load "$ROOT" >>"$RESULTS/docker-build.amd64.log" 2>&1 \
  || { note "amd64 build failed (see results/docker-build.amd64.log)"; exit 1; }
note "images ok"

# ---------------------------------------------------------------- E1
log "E1: cross-arch behavioral equivalence (10k rules x 10k inputs)"

run_native "scigen -seed 42 -rules $RULES_E2 -inputs $INPUTS_E2 -out /data/e1-arm -trials 1 || true; scigen -seed 42 -rules $RULES_E2 -inputs $INPUTS_E2 -out /data/e1-arm" >/dev/null 2>&1
run_native "sci-build -dir /data/e1-arm -trials 1 -snapshot /data/e1-arm/snapshot.json -exec" >>"$RESULTS/e1-arm.log" 2>&1
run_amd64  "sci-load  -dir /data/e1-arm -snapshot snapshot.json -trials 1 -exec" >>"$RESULTS/e1-arm.log" 2>&1
mv "$RESULTS/e1-arm/results-snapshot.jsonl" "$RESULTS/e1-arm/results-snapshot-amd64.jsonl"
run_native "sci-compare -a /data/e1-arm/results-source.jsonl -b /data/e1-arm/results-snapshot-amd64.jsonl" >>"$RESULTS/e1-arm.log" 2>&1
arm_to_amd_rc=$?

run_amd64  "scigen -seed 42 -rules $RULES_E2 -inputs $INPUTS_E2 -out /data/e1-amd" >/dev/null 2>&1
run_amd64  "sci-build -dir /data/e1-amd -trials 1 -snapshot /data/e1-amd/snapshot.json -exec" >>"$RESULTS/e1-amd.log" 2>&1
run_native "sci-load  -dir /data/e1-amd -snapshot snapshot.json -trials 1 -exec" >>"$RESULTS/e1-amd.log" 2>&1
mv "$RESULTS/e1-amd/results-snapshot.jsonl" "$RESULTS/e1-amd/results-snapshot-arm64.jsonl"
run_native "sci-compare -a /data/e1-amd/results-source.jsonl -b /data/e1-amd/results-snapshot-arm64.jsonl" >>"$RESULTS/e1-amd.log" 2>&1
amd_to_arm_rc=$?

if [[ $arm_to_amd_rc -eq 0 && $amd_to_arm_rc -eq 0 ]]; then
  note "E1: PASS (arm64->amd64 and amd64->arm64 both byte-identical)"
else
  note "E1: FAIL (arm->amd rc=$arm_to_amd_rc, amd->arm rc=$amd_to_arm_rc)"
  failed=$((failed+1))
fi

# ---------------------------------------------------------------- E2
log "E2: load-time speedup (n=$TRIALS_E2 trials, native arm64, $RULES_E2 rules)"
rm -rf "$RESULTS/e2"
run_native "
  scigen -seed 7 -rules $RULES_E2 -inputs $INPUTS_E2 -out /data/e2 &&
  sci-build -dir /data/e2 -trials $TRIALS_E2 -snapshot /data/e2/snapshot.json -timings /data/e2/source-timings.txt -label source-build &&
  sci-load  -dir /data/e2 -snapshot snapshot.json -trials $TRIALS_E2 -timings /data/e2/snapshot-timings.txt -label snapshot-load
" >>"$RESULTS/e2.log" 2>&1

run_native "
  sci-stats -baseline /data/e2/source-timings.txt -candidate /data/e2/snapshot-timings.txt -bar $SPEEDUP_BAR -labelA source-build -labelB snapshot-load -report /data/e2/report.txt
" >>"$RESULTS/e2-stats.log" 2>&1
cat "$RESULTS/e2/report.txt" >>"$SUMMARY"
if grep -q '^verdict: PASS' "$RESULTS/e2/report.txt"; then
  note "E2: PASS"
else
  note "E2: FAIL (pre-registered $SPEEDUP_BAR x bar not met)"
  failed=$((failed+1))
fi

# ---------------------------------------------------------------- E3
log "E3: snapshot determinism (two independent builds, same seed)"
rm -rf "$RESULTS/e3a" "$RESULTS/e3b"
run_native "scigen -seed 99 -rules $RULES_E2 -inputs 100 -out /data/e3a && sci-build -dir /data/e3a -trials 1 -snapshot /data/e3a/snapshot.json" >>"$RESULTS/e3.log" 2>&1
run_native "scigen -seed 99 -rules $RULES_E2 -inputs 100 -out /data/e3b && sci-build -dir /data/e3b -trials 1 -snapshot /data/e3b/snapshot.json" >>"$RESULTS/e3.log" 2>&1
if diff -q "$RESULTS/e3a/snapshot.json" "$RESULTS/e3b/snapshot.json" >>"$RESULTS/e3.log" 2>&1; then
  note "E3: PASS (snapshots byte-identical across independent builds)"
else
  note "E3: FAIL (snapshots differ -- see results/e3.log)"
  failed=$((failed+1))
fi

# ---------------------------------------------------------------- E4
log "E4: scale to $RULES_E4 rules"
rm -rf "$RESULTS/e4"
run_native "
  scigen -seed 11 -rules $RULES_E4 -inputs $INPUTS_E4 -out /data/e4 &&
  sci-build -dir /data/e4 -trials 1 -snapshot /data/e4/snapshot.json -exec &&
  sci-load  -dir /data/e4 -snapshot snapshot.json -trials 1 -exec
" >>"$RESULTS/e4.log" 2>&1
if run_native "sci-compare -a /data/e4/results-source.jsonl -b /data/e4/results-snapshot.jsonl" >>"$RESULTS/e4.log" 2>&1; then
  note "E4: PASS ($RULES_E4 rules, $INPUTS_E4 inputs round-trip identical)"
else
  note "E4: FAIL"
  failed=$((failed+1))
fi
note "E4 artifacts: snapshot.json size $(wc -c < "$RESULTS/e4/snapshot.json") bytes; rules.csv size $(wc -c < "$RESULTS/e4/rules.csv") bytes"

# ---------------------------------------------------------------- E5
log "E5: adversarial round-trip"
if run_native "sci-adv" >>"$RESULTS/e5.log" 2>&1; then
  note "E5: PASS (every adversarial case round-trips identically)"
else
  note "E5: FAIL"
  failed=$((failed+1))
fi
tail -1 "$RESULTS/e5.log" >>"$SUMMARY" 2>/dev/null || true

# ---------------------------------------------------------------- E6/E7
log "E6 + E7: refusal-path verification"
if run_native "sci-refusals" >>"$RESULTS/e67.log" 2>&1; then
  note "E6+E7: PASS"
else
  note "E6+E7: FAIL"
  failed=$((failed+1))
fi
tail -1 "$RESULTS/e67.log" >>"$SUMMARY" 2>/dev/null || true

# ---------------------------------------------------------------- verdict
log "FINAL VERDICT"
if [[ $failed -eq 0 ]]; then
  note "all experiments passed"
  exit 0
else
  note "$failed experiment(s) failed pre-registered bars -- do not move bars; revise claims"
  exit 1
fi
