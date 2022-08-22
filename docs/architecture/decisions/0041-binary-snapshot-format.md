# 41. Binary Snapshot Format (Pre-Bucketed) for `engine/indexed`

## Status

Accepted — landed in v0.16.0. Adds `Engine.ExportCompiledSnapshot()` + `LoadCompiledSnapshot()` (Go-level types) and `MarshalCompiledSnapshot()` + `UnmarshalCompiledSnapshot()` (binary wire format) as the recommended snapshot path; v0.15.0's JSON format stays available for cross-language and human-readable use cases. The new format encodes the engine's already-bucketed compiled state, so the loader skips `AddRule` and `Build` entirely. Empirically validated in [`scientific/v0.15.0/experimental/REPORT.md`](../../../scientific/v0.15.0/experimental/REPORT.md): **1.85× faster than `parser.ParseToCondition`-based source-build at 10 000 rules, 2.93× faster at 100 000 rules**, scaling 0.765 (per-rule cost grows ~30% from 10k to 100k vs source-build's ~210%). The pre-registered 3.0× bar at 10k was *not* cleared; the documentation calibrates the claim to the measured numbers.

## Context

v0.15.0 shipped a JSON-based snapshot format that, as the scientific harness documented, runs at 0.49× the speed of a CSV + `parser.ParseToCondition` source-build at 10 000 rules — the snapshot path was twice as slow as the path it replaced. Behavioral parity and determinism are properties the JSON format provides cleanly. Speed is not.

The post-v0.15.0 exploration tried four format alternatives (`scientific/v0.15.0/experimental/`):

- **C1** compact JSON (single-letter keys). Tested whether smaller payload alone would clear the gap. It does not — `encoding/json`'s per-token cost dominates over key-string size. Result: 0.50× at 10k (no improvement over v0.15.0).
- **C3** length-prefixed big-endian binary. Same shape as v0.15.0's `Snapshot`, but bytes instead of JSON. Result: 1.11× at 10k. Binary encoding is cheap; the per-rule `AddRule` cost still dominates.
- **C4** pre-classified rules + `AddPreClassifiedRule`. Skips `extractIndexablePairs` + `canonicalizeValues` in the load path. Result: 1.45× at 10k. Saves some per-rule work but the bucket insertion / fan-out still runs.
- **C5** pre-bucketed snapshot + `LoadCompiledSnapshot`. Skips `AddRule` and `Build` entirely; the loader populates the engine's compiled state directly. Result: 1.85× at 10k, **2.93× at 100k**.

The harness's correctness, determinism, and cross-arch checks all pass for C5 (10 000 inputs identical across arm64 ↔ amd64; 100 000-rule round-trip identical).

Four design questions.

### 1. What goes on the wire?

C5's wire format carries:

- `KeysetOrder []string` — the sorted-fields → keyset-ID values, in first-seen order.
- `Buckets map[string]CompiledBucket` — per keyset, the `(fields, byValueKey)` shape.
- `ByValueKey map[string][]CompiledRuleRef` — per value-key tuple, the rules that fire there.
- `CompiledRuleRef{Name, PostFilter}` — the rule name + its surviving post-filter `parser.Condition`s.
- `RulesInOrder []SnapshotRule` — same shape as v0.15.0, preserves `Name + Description + Tags + Match` so `RuleNames` / `RuleInfos` / `Diagnose` continue to work.

Encoding: 4-byte magic (`"BRG5"`), uint16 version, varint counts, length-prefixed strings, single-byte condition-type and op tags. Big-endian throughout (portability). Same condition-type tags as C3 (`string=1`, `set=2`, `range=3`, `and=4`).

### 2. How does this interact with v0.15.0?

Two choices:

- **(a) Replace.** v0.15.0's JSON format is deprecated and removed at v0.16.0.
- **(b) Keep both.** v0.15.0 stays available; C5 becomes the default for new consumers.

Pick **(b)**. JSON has independent value: cross-language consumers (a Java service that reads the snapshot to validate it before deploying), human-readability for debugging, and stability for any caller who depends on the v0.15.0 contract. The cost of keeping both is small (the C5 format coexists in `engine/indexed/snapshotv2.go`). v0.16.0 release notes recommend C5 for performance-sensitive callers; v0.15.0 stays in the cookbook as an option.

### 3. What's the v0.16.0 public API?

```go
// In engine/indexed, already landed in v0.15.0's snapshotv2.go (experimental):

func (e *Engine) ExportCompiledSnapshot() (*CompiledSnapshot, error)
func LoadCompiledSnapshot(cs *CompiledSnapshot, rebuild map[string]RuleCallbacks) (*Engine, error)

type CompiledSnapshot struct {
    KeysetOrder  []string
    Buckets      map[string]CompiledBucket
    RulesInOrder []SnapshotRule
}

type CompiledBucket struct {
    Fields     []string
    ByValueKey map[string][]CompiledRuleRef
}

type CompiledRuleRef struct {
    Name       string
    PostFilter []parser.Condition
}
```

For v0.16.0 promotion:

- Mark the API stable (drop the "EXPERIMENTAL" warning from the doc comment).
- Add wire-format helpers `MarshalCompiledSnapshot` / `UnmarshalCompiledSnapshot` (currently the wire format lives in the experimental harness's `format/` package; promoting it to `engine/indexed/snapshotv2.go` is part of v0.16.0).
- Define `CompiledSnapshotFormatVersion = 1` and the same strict-version-gate policy as v0.15.0.
- Document the format header (`"BRG5"` magic + uint16 version) so independent readers in other languages can implement decoders.

### 4. What's the upgrade path for v0.15.0 consumers?

Same engine, two formats. A v0.15.0 consumer using `ExportSnapshot` + `LoadSnapshot` keeps working unchanged after upgrading the bre-go dep to v0.16.0. To opt into the faster format, callers switch to `ExportCompiledSnapshot` + `LoadCompiledSnapshot`. No migration of stored snapshots is required — consumers regenerate from source at deploy time.

`v0.15.0` snapshots are not auto-convertible to v0.16.0 binary snapshots (the source rules would need to be re-loaded). This is fine for the "build-once, deploy-many" use case where the build job re-runs on every rule-source change anyway.

## Decision

Promote `engine/indexed.ExportCompiledSnapshot` / `LoadCompiledSnapshot` from experimental to stable in v0.16.0. Add the wire-format helpers (`MarshalCompiledSnapshot` / `UnmarshalCompiledSnapshot`) that the experimental harness currently provides in its own `format/` package. Document the empirical performance curve honestly:

- 10 000 rules: ~1.85× faster than CSV + `parser.ParseToCondition`.
- 100 000 rules: ~2.93× faster.
- Per-rule scaling: 0.765 — 30% slower-than-linear at 100k vs the source path's 210% slower-than-linear.

v0.15.0 JSON snapshot stays available and supported. v0.16.0 release notes name it as the legacy format and steer new consumers to the binary format.

## Consequences

### Closed by v0.16.0

- The "build-once, deploy-many" pitch is empirically true at the 100k scale (2.93×) and meaningfully true at 10k (1.85×). The v0.15.0 retraction in `README.md` and `CHANGELOG.md` gets a follow-up note pointing to v0.16.0 for performance-sensitive callers.
- Better scaling than every alternative measured. C5's per-rule cost grows only ~30% from 10k to 100k; source-build grows ~210%. At 1M+ rules the gap continues to widen.
- The cross-arch contract from v0.15.0 holds: big-endian wire format + portable encoding for floats (decimal strings) + same JSON-style Inf handling. Verified arm64 ↔ amd64.

### NOT closed by v0.16.0

- **3.0× speedup at 10 000 rules.** The pre-registered B1 bar measured 5.0 ms; C5 measured 8.297 ms. The bar stays where it is. The next move would be a zero-copy / memory-mapped binary (C6 in the exploration), which is exotic enough to want a real consumer before it ships. v0.16.0 does not address this.
- **Cross-language consumers.** The binary format has a documented wire shape, but reference decoders for languages other than Go are not part of v0.16.0. Adopters needing cross-language read access stay on JSON.
- **Snapshot diffability.** The v0.15.0 JSON snapshot is `git diff`-able. C5 binary is not. Consumers who relied on diffability for change auditing should stay on JSON or fork-decode the binary into a textual report.

### Performance impact (measured)

- ExportCompiledSnapshot at 10k: not separately timed; embedded in the build-c5 binary's end-to-end (~15 ms for source-parse + classify + bucket + encode). Run at build time, not request time.
- LoadCompiledSnapshot at 10k: 8.297 ms median (n=50).
- LoadCompiledSnapshot at 100k: 108.423 ms median (n=20).
- File size at 10k: 0.83 MB (50% smaller than v0.15.0's 1.66 MB JSON).
- Per-Execute hot path is unchanged. The compiled snapshot deserializes into the exact same internal `*snapshot` shape that v0.15.0's `LoadSnapshot` produces.

### Validation strategy

- The experimental harness is the test bed. Promotion to v0.16.0 includes:
  - Lifting `MarshalCompiledSnapshot` / `UnmarshalCompiledSnapshot` from experimental into `engine/indexed`.
  - Adding the wire format to `scientific/v0.16.0/` with the same E1–E7 battery (cross-arch, determinism, 100k round-trip, adversarial, refusal paths, plus the same E2 timing measurement with pre-registered bars carried forward).
  - 100% per-package coverage maintained for `engine/indexed`.

### What this validates for v0.17.0+

Zero-copy / memory-mapped formats remain the only credible path to break 3× at 10k. A v0.17.0 ADR could explore mmap if a real consumer's load-time SLA forces it. Until then, C5 is the published best.

The "fixed-time-at-10k" framing of B1 is the wrong way to measure snapshot performance for the next iteration; the right framing is "speedup at the operator's actual scale." A future ADR proposing C6 should pre-register against a *curve* (10k, 100k, 1M) rather than a point.
