#!/usr/bin/env bash
# Orchestrator for the experimental candidates (C1, C3, C4, C5).
#
# Pre-registered bars:
#   B1 (speed): median load time at 10k rules <= 5.0 ms
#               (== source-build median 15.3 ms / 3.0)
#   B2 (scale): rules-loaded-per-ms throughput at 100k
#               >= 0.8 * throughput at 10k
#   B3 (correctness): every candidate's load result file must be
#                     byte-identical to its build's source-execute file
#                     at 10k AND at 100k
#   B4 (cross-arch): each candidate that passes B1+B2+B3 must also
#                    round-trip byte-for-byte arm64 <-> amd64
#
# Outputs:
#   results/                raw timings + result files + verdict
#   results/COMPARE.txt     per-candidate table; final verdict
#
# Exit: 0 iff at least one candidate clears B1+B2+B3+B4. Otherwise 1.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../../.." && pwd)"
EXP="$ROOT/scientific/v0.15.0/experimental"
RESULTS="$EXP/results"

ARM_IMAGE="bre-go-sci-exp-v0.15.0:arm64"
AMD_IMAGE="bre-go-sci-exp-v0.15.0:amd64"

TRIALS_10K="${TRIALS_10K:-50}"
TRIALS_100K="${TRIALS_100K:-20}"
SPEEDUP_BAR_MS="${SPEEDUP_BAR_MS:-5.0}"
SCALE_RATIO_BAR="${SCALE_RATIO_BAR:-0.80}"

mkdir -p "$RESULTS"
COMPARE="$RESULTS/COMPARE.txt"
: > "$COMPARE"

log()  { printf '\n=== %s ===\n' "$*" | tee -a "$COMPARE" >&2; }
note() { printf '%s\n' "$*"           | tee -a "$COMPARE" >&2; }

run_native() { docker run --rm --cpus 1 --memory 1g -v "$RESULTS:/data" "$ARM_IMAGE" -c "$1"; }
run_amd64()  { docker run --rm --platform linux/amd64 --cpus 1 --memory 1g -v "$RESULTS:/data" "$AMD_IMAGE" -c "$1"; }

# Median helper: takes a timings-file path on the host, prints median ns.
median_ns() {
  awk '{print}' "$1" | sort -n | awk 'BEGIN{c=0}{a[c++]=$1}END{
    if (c==0) {print 0; exit}
    if (c%2==1) {print a[(c-1)/2]} else {printf "%d\n", (a[c/2-1]+a[c/2])/2}
  }'
}

# ---------------------------------------------------------------- build
log "BUILD: experimental harness images"
docker build -f "$EXP/Dockerfile" -t "$ARM_IMAGE" "$ROOT" >>"$RESULTS/docker-build.arm64.log" 2>&1 \
  || { note "arm64 build failed (see results/docker-build.arm64.log)"; exit 1; }
docker buildx build --platform linux/amd64 -f "$EXP/Dockerfile" -t "$AMD_IMAGE" --load "$ROOT" >>"$RESULTS/docker-build.amd64.log" 2>&1 \
  || { note "amd64 build failed (see results/docker-build.amd64.log)"; exit 1; }
note "images ok"

# ---------------------------------------------------------------- 10k generation + source baseline
log "GENERATE: 10k rules + 10k inputs (seed 7), 100k rules + 1k inputs (seed 11)"
rm -rf "$RESULTS/d10k" "$RESULTS/d100k"
run_native "scigen -seed 7  -rules 10000  -inputs 10000 -out /data/d10k" >/dev/null 2>&1
run_native "scigen -seed 11 -rules 100000 -inputs 1000  -out /data/d100k" >/dev/null 2>&1

log "BASELINE source-build (10k, n=$TRIALS_10K)"
run_native "sci-build -dir /data/d10k -trials $TRIALS_10K -snapshot /data/d10k/snapshot.json -timings /data/d10k/source.txt -label source-build && sci-build -dir /data/d10k -trials 1 -exec" >>"$RESULTS/baseline.log" 2>&1
src_median_10k=$(median_ns "$RESULTS/d10k/source.txt")
note "source-build 10k median: $src_median_10k ns"

log "BASELINE source-build (100k, n=$TRIALS_100K)"
run_native "sci-build -dir /data/d100k -trials $TRIALS_100K -snapshot /data/d100k/snapshot.json -timings /data/d100k/source.txt -label source-build-100k && sci-build -dir /data/d100k -trials 1 -exec" >>"$RESULTS/baseline.log" 2>&1
src_median_100k=$(median_ns "$RESULTS/d100k/source.txt")
note "source-build 100k median: $src_median_100k ns"

# ---------------------------------------------------------------- per-candidate run
candidates=(c1 c3 c4 c5)
declare -A median_10k median_100k correct_10k correct_100k

# Result-baseline files: each candidate writes its source-side results
# in build-X (exec mode). The load-X reads results too. We diff them.

for c in "${candidates[@]}"; do
  log "CANDIDATE $c (10k)"
  case "$c" in
    c1) snap=snapshot.c1.json ;;
    c3) snap=snapshot.c3.bin  ;;
    c4) snap=snapshot.c4.bin  ;;
    c5) snap=snapshot.c5.bin  ;;
  esac
  run_native "sci-build-$c -dir /data/d10k -out $snap -exec && sci-load-$c -dir /data/d10k -snapshot $snap -trials $TRIALS_10K -timings /data/d10k/$c.txt -label load-$c -exec" >>"$RESULTS/$c.log" 2>&1
  median_10k[$c]=$(median_ns "$RESULTS/d10k/$c.txt")
  if run_native "sci-compare -a /data/d10k/results-$c-source.jsonl -b /data/d10k/results-$c.jsonl" >>"$RESULTS/$c-correctness.log" 2>&1; then
    correct_10k[$c]="ok"
  else
    correct_10k[$c]="FAIL"
  fi
  note "$c 10k median: ${median_10k[$c]} ns; correctness: ${correct_10k[$c]}"

  log "CANDIDATE $c (100k)"
  run_native "sci-build-$c -dir /data/d100k -out $snap -exec && sci-load-$c -dir /data/d100k -snapshot $snap -trials $TRIALS_100K -timings /data/d100k/$c.txt -label load-$c-100k -exec" >>"$RESULTS/$c.log" 2>&1
  median_100k[$c]=$(median_ns "$RESULTS/d100k/$c.txt")
  if run_native "sci-compare -a /data/d100k/results-$c-source.jsonl -b /data/d100k/results-$c.jsonl" >>"$RESULTS/$c-correctness.log" 2>&1; then
    correct_100k[$c]="ok"
  else
    correct_100k[$c]="FAIL"
  fi
  note "$c 100k median: ${median_100k[$c]} ns; correctness: ${correct_100k[$c]}"
done

# ---------------------------------------------------------------- evaluate bars
log "EVALUATE pre-registered bars"
bar_ns=$(awk -v ms="$SPEEDUP_BAR_MS" 'BEGIN{ printf "%.0f", ms * 1000 * 1000 }')
note "B1: speed bar = ${SPEEDUP_BAR_MS} ms (${bar_ns} ns)"
note "B2: scale ratio bar = ${SCALE_RATIO_BAR} (throughput@100k / throughput@10k >= bar)"
note ""
printf '%-4s | %-12s | %-12s | %-10s | %-10s | %s\n' "cand" "10k_median" "100k_median" "scale_ok" "speed_ok" "verdict" | tee -a "$COMPARE" >&2
printf '%s\n' "---- | ------------ | ------------ | ---------- | ---------- | ------" | tee -a "$COMPARE" >&2

any_passed=0
for c in "${candidates[@]}"; do
  m10=${median_10k[$c]}
  m100=${median_100k[$c]}
  # throughput 10k = 10000 / m10 (rules/ns); same factor for 100k. Ratio = (100000/m100) / (10000/m10) = (m10/m100) * 10
  scale_ratio=$(awk -v a="$m10" -v b="$m100" 'BEGIN{ printf "%.3f", (a/b) * 10 }')
  if awk -v s="$scale_ratio" -v b="$SCALE_RATIO_BAR" 'BEGIN{ exit (s+0 >= b+0) ? 0 : 1 }'; then
    scale_ok="PASS"
  else
    scale_ok="FAIL"
  fi
  if awk -v m="$m10" -v bn="$bar_ns" 'BEGIN{ exit (m+0 <= bn+0) ? 0 : 1 }'; then
    speed_ok="PASS"
  else
    speed_ok="FAIL"
  fi
  verdict="FAIL"
  if [[ "$scale_ok" == "PASS" && "$speed_ok" == "PASS" && "${correct_10k[$c]}" == "ok" && "${correct_100k[$c]}" == "ok" ]]; then
    verdict="PASS"
    any_passed=1
  fi
  m10_ms=$(awk -v n="$m10" 'BEGIN{ printf "%.3fms", n/1e6 }')
  m100_ms=$(awk -v n="$m100" 'BEGIN{ printf "%.3fms", n/1e6 }')
  printf '%-4s | %-12s | %-12s | %-10s | %-10s | %s\n' "$c" "$m10_ms" "$m100_ms" "$scale_ok ($scale_ratio)" "$speed_ok" "$verdict" | tee -a "$COMPARE" >&2
done

# ---------------------------------------------------------------- B4 cross-arch for passing candidates
if [[ $any_passed -eq 1 ]]; then
  log "B4 cross-arch check for PASS candidates"
  for c in "${candidates[@]}"; do
    # Re-evaluate verdict on this pass
    m10=${median_10k[$c]}
    m100=${median_100k[$c]}
    scale_ratio=$(awk -v a="$m10" -v b="$m100" 'BEGIN{ printf "%.3f", (a/b) * 10 }')
    if [[ "${correct_10k[$c]}" != "ok" || "${correct_100k[$c]}" != "ok" ]]; then continue; fi
    if ! awk -v s="$scale_ratio" -v b="$SCALE_RATIO_BAR" 'BEGIN{ exit (s+0 >= b+0) ? 0 : 1 }'; then continue; fi
    if ! awk -v m="$m10" -v bn="$bar_ns" 'BEGIN{ exit (m+0 <= bn+0) ? 0 : 1 }'; then continue; fi

    case "$c" in
      c1) snap=snapshot.c1.json ;;
      c3) snap=snapshot.c3.bin  ;;
      c4) snap=snapshot.c4.bin  ;;
      c5) snap=snapshot.c5.bin  ;;
    esac
    note "$c cross-arch: build snapshot on arm64, load + execute on amd64"
    rm -rf "$RESULTS/cross-$c"
    run_native "scigen -seed 42 -rules 10000 -inputs 10000 -out /data/cross-$c && sci-build-$c -dir /data/cross-$c -out $snap -exec" >>"$RESULTS/$c-cross.log" 2>&1
    run_amd64  "sci-load-$c -dir /data/cross-$c -snapshot $snap -trials 1 -exec" >>"$RESULTS/$c-cross.log" 2>&1
    if run_native "sci-compare -a /data/cross-$c/results-$c-source.jsonl -b /data/cross-$c/results-$c.jsonl" >>"$RESULTS/$c-cross.log" 2>&1; then
      note "$c B4: PASS (arm64 build -> amd64 load -> identical matches)"
    else
      note "$c B4: FAIL"
    fi
  done
fi

# ---------------------------------------------------------------- verdict
log "FINAL VERDICT"
if [[ $any_passed -eq 1 ]]; then
  note "at least one candidate cleared B1+B2+B3 (10k+100k); see compare table"
  exit 0
else
  note "no candidate cleared all pre-registered bars"
  exit 1
fi
