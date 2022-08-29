#!/usr/bin/env bash
# Tag-triggered harness for the v0.17.0 OTel adapter scientific
# review. Runs cmd/otel-review (which captures span trees + attribute
# payloads for 11 production-mirroring scenarios) and writes them to
# results/scenarios.json. No Docker / multi-arch -- this is a pure-Go
# review of attribute semantics, not a performance harness.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

cd "$ROOT"
mkdir -p results
go run ./cmd/otel-review > results/scenarios.json
echo "wrote $(wc -l < results/scenarios.json) lines to results/scenarios.json"
