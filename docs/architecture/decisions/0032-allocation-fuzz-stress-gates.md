# 32. Allocation, Fuzz, and Stress Gates

## Status

Proposed — target v0.7.2. A second patch release on top of v0.7.0,
landing the Go-native regression gates that complement (and do not
duplicate) v0.7.1's advisory benchmark matrix.

## Context

ADR-0031 explicitly decided that bench-matrix numbers are **advisory,
not gated by CI**. Benchmark timing has too high a per-commit noise
floor to gate without producing false positives, and `make
bench-matrix` is too slow (~48s) to run on every push. That decision
stands.

But it leaves a gap: between "no performance signal in CI" (today)
and "the bench matrix as a gate" (rejected for good reasons) there is
a middle path that uses Go's testing toolchain to gate **specific
performance and robustness invariants** without paying the
noise/cost tax. Three Go-native tools cover three distinct
regression classes:

1. **Allocation regressions.** Someone introduces a `fmt.Sprintf`
   inside a rule-evaluation loop; `Execute` goes from 15 allocs/op to
   25. The bench matrix would surface this only at release prep, and
   even then only as a delta you have to read. `testing.AllocsPerRun`
   asserts an exact count in a regular `*_test.go`; the diff is the
   failure message.
2. **Edge-case panics on adversarial input.** The expression parser
   accepts string input from the caller. Handwritten unit tests
   cover the grammar; they do not exercise the long tail of
   syntactically-weird inputs (zero-width whitespace, deeply nested
   parens, malformed UTF-8). Go 1.18's `go test -fuzz=` generates
   that long tail systematically.
3. **Concurrency races and goroutine leaks under sustained load.**
   `-race` (already in `make test`) catches a race only if the racing
   code path actually runs. A 5-rule unit test does not exercise the
   listener-fan-out path at 100k req/s. A stress loop forces the path
   to fire enough times that `-race` and goroutine counters do their
   job.

Each tool handles regressions the others would miss. None duplicates
the bench matrix; together they make the bench matrix what it should
be — a release-prep "did the new feature pay its perf rent?"
question — by closing the small-leak gaps that would otherwise rely
on the bench matrix being run more often than is honest.

Three design questions:

**1. What allocation counts do we freeze, and where?**

`testing.AllocsPerRun` returns the observed allocation count for a
function run N times (averaged). It is deterministic when the
function under test does not depend on RNG, map iteration order, or
goroutine scheduling.

Pin allocation counts in per-adapter `allocs_test.go` files at a
small, fixed workload (10 rules, 1 dimension). Reasons for a small
fixed workload:

- The number is small enough that a regression of even 5 allocs is
  noisy and obvious (versus the matrix's 1000-rule cell where +5
  hides in the per-rule cost).
- Small workloads are stable across Go versions and hardware in a
  way that timing is not.
- The asserted constant lives in the test next to the rule construction,
  so a deliberate change to the adapter's hot path naturally re-asserts
  the new number in the same commit.

If a future change earns a higher alloc count (e.g., adding a new
required field), the test updates in the same commit as the
implementation change. The pin is **a tripwire, not a budget** — it
fires on any change, intended or not. That is the value.

**2. What gets a fuzz target?**

Only the **parsing surfaces** — the packages that ingest untrusted
input. Two qualify today:

- `engine/parser.Parse` — string in, AST out. Adversarial inputs
  could plausibly trigger panics in the lexer, recursive descent, or
  unbalanced-paren handling.
- `engine/json.Loader[RC].RuleConfigs` — JSON bytes in, slice out. The
  outer `json.Decoder` is well-fuzzed by the Go team; what we own is
  the per-item dispatch into the caller's `ItemParser`. A fuzz target
  exercises edge cases like deeply-nested arrays, mixed
  whitespace, and trailing data.

`engine/csv` is not fuzzed in this cut — `encoding/csv` is already
fuzzed by the Go team, and our adapter is thin. Future ADR if a real
bug surfaces.

The fuzz targets ship as `Fuzz*` functions in `*_test.go`; they
**compile** under `make test` (catching API drift) but only **run**
under `make fuzz-quick`, which does a fixed 30-second pass per
target. A nightly long-running fuzz is out of scope today (no nightly
CI yet).

**3. How do stress tests integrate without bloating `make test`?**

A stress test that loops `Execute` 100k+ times costs hundreds of
milliseconds even on fast hardware. Multiply by N adapters and the
test suite gets noticeably slower. Two approaches:

- **(a) Always-on, tightly bounded.** A 10k-iteration stress test
  runs in ~50ms. Cheap enough to keep in `make test`.
- **(b) Build tag.** `//go:build stress` excludes the file from
  default builds; `make stress` runs `go test -tags=stress`.

Pick **(b)**. The stress loops are not just about *whether* the code
runs — they are about *how many times* the code runs before
`runtime.NumGoroutine` drifts or `-race` catches a data race. The
useful run is 100k+ iterations, not 10k. Tying them to a build tag
keeps `make test` fast while giving release prep (and CI's future
nightly job) a meaningful stress signal.

Each adapter gets a `stress_test.go` with `//go:build stress` and at
minimum:

- `TestExecuteSurvivesHighVolume` — sustained Execute calls, no
  panic, no error.
- `TestNoGoroutineLeakUnderListenerFanout` — runs Execute with
  several listeners attached; asserts goroutine count before/after
  has not drifted.

## Decision

Three additions, one new Makefile target each:

### (a) Allocation tripwires

Per-adapter `allocs_test.go` in `engine/inmemory`, `engine/firstmatch`,
`engine/priority`, and `engine/parser`. Each pins the allocation count
of the adapter's hot path at a fixed small workload:

```go
func TestExecuteAllocCount(t *testing.T) {
    e := buildTenRuleEngine()
    req := engine.Request{Input: 5}
    n := testing.AllocsPerRun(100, func() {
        _, _ = e.Execute(context.Background(), req)
    })
    const want = 6 // frozen 2022-06-13; bump in same commit as any deliberate change
    if int(n) != want {
        t.Fatalf("Execute allocs: want %d, got %d", want, int(n))
    }
}
```

These run in `make test`; failures gate `ci-local`.

### (b) Fuzz targets

Two `Fuzz*` functions, one per parsing surface:

- `engine/parser/fuzz_test.go` → `FuzzParse(f *testing.F)` seeded
  with the grammar's known-good corpus, asserts `Parse` returns
  cleanly (parse-success or `*ParseError`) and never panics.
- `engine/json/fuzz_test.go` → `FuzzRuleConfigsArrayShape` seeded
  with valid + slightly-broken arrays, asserts `RuleConfigs` returns
  cleanly (success or `*LoadError`) and never panics.

`make fuzz-quick` runs each target for 30 seconds:

```
go test -fuzz=FuzzParse -fuzztime=30s ./engine/parser/...
go test -fuzz=FuzzRuleConfigsArrayShape -fuzztime=30s ./engine/json/...
```

The `Fuzz*` functions themselves compile under `make test` (so they
cannot rot silently); fuzzing runs only under `make fuzz-quick`. A
nightly fuzz job lands in a future CI ADR.

### (c) Stress tests behind a build tag

Per-adapter `stress_test.go` files with `//go:build stress`. Run via
`make stress`:

```
go test -tags=stress -count=1 -race ./...
```

Each file ships at least:

- `TestExecuteSurvivesHighVolume` — 100k iterations of a populated
  Execute, no panic, no error.
- `TestNoGoroutineLeakUnderListenerFanout` — adds several listeners,
  runs 10k iterations, asserts `runtime.NumGoroutine` stable.

Stress tests are **not** in `ci-local`. They are in `make stress`,
which release prep runs alongside `make bench-matrix`.

### Makefile additions

```
make test         # unchanged -- runs allocs assertions automatically
make fuzz-quick   # new: 30s fuzz pass per target
make stress       # new: build-tagged stress tests with -race
make ci-local     # unchanged
```

The release-prep ritual gains two manual gates: `make bench-matrix`
(already), `make fuzz-quick`, and `make stress`. CI gates one extra
invariant per commit (the allocation tripwires).

## Consequences

The library gains three regression gates that fire on three real
classes of failure, none of which the bench matrix would catch
cheaply enough to gate per-commit.

Cost is bounded: allocation assertions are pure-Go and run in milliseconds;
fuzz targets compile but don't run unless invoked; stress tests are
opt-in. `make test` runtime increases by ~50ms total (the allocation
assertions). `ci-local` stays under its current wall-clock budget.

The allocation tripwires are deliberately strict. A change that
legitimately changes the alloc count must update the constant in the
same commit. That is the point: "I didn't mean to change perf" is the
expensive bug; "I did mean to change perf, update the assertion" is
trivial code review.

Fuzz targets do not produce frozen baselines — they produce a corpus
of inputs that previously broke things, kept in
`testdata/fuzz/Fuzz*/` and committed alongside the test. New
discoveries enlarge the corpus. The corpus is the regression record.

Stress tests behind a build tag means they do not gate per commit.
The tradeoff for this is honest: per-commit stress would mean every
contributor's local `go test` becomes a 30+ second run. The release
gate (`make stress` before tagging) is the right granularity for a
library that does not yet promise concurrency safety on `Execute`
(deferred to v0.12.0 per the roadmap).

If a real regression in any of the three classes ever slips through
to a release, the fix is to add a test case to the relevant suite —
not to escalate that class's gate to a stricter mode. The shape
established here scales: more allocation tripwires, more fuzz seeds,
more stress loops. The infrastructure exists; the surface grows by
addition.
