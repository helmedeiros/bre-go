#!/usr/bin/env bash
# Tag-triggered harness for the v0.18.0 metrics-port scientific
# review. Runs cmd/sink-demo (which exercises three independent
# ExecutionMetricSink implementations through stacked metrics.Wrap
# calls) and writes the captured output to results/sink-demo.txt.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

cd "$ROOT"
mkdir -p results
go run ./cmd/sink-demo > results/sink-demo.txt
echo "wrote $(wc -l < results/sink-demo.txt) lines to results/sink-demo.txt"
