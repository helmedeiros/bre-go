# 37. Build-then-Execute Lifecycle and Concurrent-Safe Execute

## Status

Accepted — landed in v0.12.0. `engine/indexed.Engine.Build()` +
`Built()` shipped; lockless concurrent `Execute` against an
atomic-snapshot via `sync/atomic.Value`. `engine/internal/adapter.Notifier`
moved to copy-on-write listener semantics across all four adapters.
All v0.8.0–v0.11.0 success-bar cells continue to pass; new
concurrent + lifecycle tests gate `ci-local` under `-race`. Hot
reload documented as a caller-side `atomic.Value` pattern (no new
library type).

## Context

After v0.11.0, the indexed adapter has a usable shape contract but
two production-grade gaps:

1. **`Execute` is not safe for concurrent calls.** The current
   implementation mutates the listener slice via `AddListener`
   without synchronization; reading from the bucket structures
   while a separate goroutine mutates them via `AddRule` would
   trigger the race detector. Today's tests run single-threaded;
   the `//go:build stress` test verifies that one goroutine
   calling Execute 100 000 times doesn't leak goroutines, but does
   not exercise concurrent callers.
2. **No hot-reload story.** Production rule sets change while the
   service runs. Today's only path is "stop the service, restart
   with new rules" — not what consumers ship.

The canonical solution in this space is an immutable snapshot of
the indexed state held in an atomic reference. Reads are lockless
because the snapshot doesn't mutate; reloads swap the atomic
reference; concurrent calls finish against whichever snapshot they
Loaded. It's the standard pattern across rule-engine, routing-
table, and feature-flag libraries.

v0.12.0 adopts this pattern. The lifecycle change is small but
load-bearing:

- **Builder phase** (today's default): `New() → AddRule() ×
  N → WithPostFilterHook()`. The engine is mutable.
- **Built phase** (new): `Build()` finalizes a snapshot. After
  Build, `Execute` is concurrent-safe and lockless; `AddRule`
  returns the new `ErrEngineBuilt` sentinel.

Existing callers (v0.8.0–v0.11.0) never called Build. To preserve
their code without changes, **Execute implicitly calls Build on
its first invocation**. They get the new concurrency guarantee
automatically; they keep their existing code.

Four design questions.

### 1. Explicit `Build()` vs. implicit-only

Two shapes:

**(a) Explicit `Build()` method.** Callers that care about
seal-the-engine semantics call it before the first Execute.
Callers that don't care let the first Execute trigger it
implicitly.

**(b) Implicit-only.** No `Build()` API. The first Execute
seals the engine; subsequent `AddRule` returns
`ErrEngineBuilt`.

Pick **(a)**. Explicit Build:

- Gives callers a deterministic seal point for hot-reload logic
  ("build, validate, swap").
- Lets callers detect rule-set validation errors before any
  request is in flight.
- Makes the lifecycle a documented contract instead of a side
  effect of "the first request happened."

Implicit-only would force callers to issue a dummy Execute to
finalize the engine. Awkward.

### 2. Snapshot held in `sync/atomic.Value` vs `atomic.Pointer[T]`

The standard primitive for "atomic pointer to a struct" in Go is
`atomic.Pointer[T]` (Go 1.19+). The project targets Go 1.18
(declared in `go.mod`). `atomic.Pointer` is unavailable.

`atomic.Value` (Go 1.0+) holds an `interface{}` with consistent
type. Stores and loads are atomic, lockless on the read side.
Type-asserts on every Load — small cost (single TLB lookup).

Pick **`atomic.Value`**. Standard library, ships in Go 1.18. The
type assertion overhead is single-digit nanoseconds — well under
the Execute hot path's per-call cost.

A future minor release can switch to `atomic.Pointer[*snapshot]`
when we bump the minimum Go version. The Snapshot type itself
stays.

### 3. Listener synchronization

`adapter.Notifier` (embedded in every adapter, ADR-0029) carries
`listeners []observability.ExecutionListener`. Today's
`AddListener` appends without synchronization; today's
`NotifyMatched` iterates the slice. Concurrent calls would race.

Three options:

- **(i) Lock the listener slice with `sync.RWMutex`.** Reads
  acquire RLock during the Notify* iteration; AddListener acquires
  Lock. Read contention exists.
- **(ii) `atomic.Value` holding the listener slice;
  copy-on-write on AddListener.** Lockless reads. AddListener
  pays O(N) to copy the slice and store the new pointer.
- **(iii) Listeners are part of the immutable snapshot; cannot
  add listeners after Build.** Most symmetric with the
  rule-snapshot model; most restrictive on callers.

Pick **(ii)**. Reads are the hot path; AddListener is one-shot at
startup or rarely during operation. Copy-on-write is the right
tradeoff. Option (iii) is too strict — observability tooling
sometimes attaches listeners dynamically (per-request correlation,
per-tenant filtering); we shouldn't lock that out.

This change lives in `engine/internal/adapter.Notifier` and
benefits all four adapters at once. Other adapters
(`inmemory`, `firstmatch`, `priority`) continue to mutate their
own rule state per AddRule — they don't get the v0.12.0
build-then-execute lifecycle in this ADR. The listener
concurrency fix is shared; the rule-set immutability is
indexed-only.

### 4. How does hot reload work?

Three options:

- **(a) An in-place `Reload(newRules)` method.** The engine
  rebuilds its snapshot in place; in-flight Executes complete
  against whichever snapshot they Loaded. Mutates the engine,
  conflicts with the build-then-execute model.
- **(b) A wrapper type `Reloadable`** (or similar) that holds an
  `atomic.Value` of `*Engine` and exposes the same `Execute`
  surface. Caller builds new engines and swaps via the wrapper.
- **(c) No new library type — document the pattern.** Caller
  uses `atomic.Value` directly to hold the current `*Engine`,
  swaps when reloading. The library doesn't add a new type.

Pick **(c)**. Once Execute is concurrent-safe, "hot reload" is
just "store a different `*Engine` in your atomic reference."
Adding a Reloadable wrapper would be one more type in the public
surface for behavior any Go programmer expresses in five lines.

The library's deliverable for hot reload is **documentation +
example**, not a new type. ADR-0037 ships the cookbook entry.

If a real consumer asks for a higher-level wrapper (with
generations, drain timeouts, or build-validation hooks), that's
a future ADR with a real use case driving the surface.

## Decision

### Lifecycle on `engine/indexed.Engine`

```go
// New API surface (additions to existing Engine):
func (e *Engine) Build() error
func (e *Engine) Built() bool

// Error sentinel:
var ErrEngineBuilt = errors.New("indexed: engine is already built (call AddRule / WithPostFilterHook before Build)")
var ErrAlreadyBuilt = errors.New("indexed: Build called on an already-built engine")
```

State transitions:

| State | AddRule | WithPostFilterHook | Build | Execute |
|---|---|---|---|---|
| New | ok | ok (method-chain) | seals → Built | implicit Build → executes |
| Built | `ErrEngineBuilt` | `ErrEngineBuilt` (via panic — see below) | `ErrAlreadyBuilt` | ok (concurrent-safe) |

Two patterns for the hook:

- `WithPostFilterHook(h)` called pre-Build: stores the hook in
  the builder state. The hook is sealed into the snapshot at
  Build time.
- `WithPostFilterHook(h)` called post-Build: **panics**, because
  the alternative (silent no-op) hides a programming error. The
  caller has confused the lifecycle; loud failure is the
  appropriate response.

Existing callers (v0.8.0–v0.11.0) don't call Build explicitly.
The first Execute on their engine performs an implicit Build
under a mutex; subsequent Executes are lockless. Behaviorally
identical to today; concurrency-safe as a bonus.

### Engine structure

```go
type Engine struct {
    adapter.Notifier // listener wiring with copy-on-write semantics

    mu       sync.Mutex     // protects builder during AddRule / Build
    builder  *builderState  // nil after Build
    snapshot atomic.Value   // stores *snapshot once Build runs
}

type builderState struct {
    buckets        map[string]*keysetBucket
    keysetOrder    []string
    rulesInOrder   []Rule
    ruleNames      map[string]struct{}
    postFilterHook PostFilterHook
}

type snapshot struct {
    buckets        map[string]*keysetBucket
    keysetOrder    []string
    rulesInOrder   []Rule
    postFilterHook PostFilterHook
}
```

`snapshot` is the immutable read-side representation. After
Build, every Execute Loads the snapshot via
`atomic.Value.Load().(*snapshot)`. The mutex is touched only
during AddRule / Build (the build phase), never during Execute
(the read phase).

### Algorithms

**AddRule** (essentially unchanged, but locked):
```
1. lock mu
2. if snapshot is non-nil: return ErrEngineBuilt
3. (existing classification + bucket insertion against e.builder)
4. unlock mu
```

**Build**:
```
1. lock mu
2. if snapshot is non-nil: unlock; return ErrAlreadyBuilt
3. snap = &snapshot{copies of builder fields}
4. atomic.Value.Store(snap)
5. builder = nil
6. unlock mu
```

**Execute**:
```
1. snap = e.snapshot.Load().(*snapshot)
2. if snap == nil:
     // First Execute -- implicit Build.
     lock mu
     reload snap from atomic.Value (in case another goroutine raced us)
     if still nil: snap = build snapshot from e.builder; store; builder = nil
     unlock mu
3. (existing key-set walk, bucket probe, post-filter, action — against snap)
```

The race-recheck in step 2 is the double-checked locking idiom;
Go's memory model + atomic.Value guarantees correctness when the
inner check Loads via the same atomic primitive.

### `engine/internal/adapter.Notifier` — copy-on-write listeners

Replace `listeners []observability.ExecutionListener` with
`listeners atomic.Value` holding `[]observability.ExecutionListener`.

`AddListener`:
```go
func (n *Notifier) AddListener(l observability.ExecutionListener) {
    n.mu.Lock()  // serializes concurrent AddListeners
    cur := n.loadListeners()
    next := make([]observability.ExecutionListener, len(cur)+1)
    copy(next, cur)
    next[len(cur)] = l
    n.listeners.Store(next)
    n.mu.Unlock()
}
```

`NotifyMatched` / `NotifyStarted` / `NotifyFinished` / `NotifyErrored`:
```go
listeners := n.loadListeners() // single atomic Load
for _, l := range listeners {
    // existing dispatch logic
}
```

Lockless read path. AddListener pays O(N) to copy on every call —
fine for the AddListener cost profile (rare, startup-only typically).

This change affects all four adapters' notifier behavior. The
v0.8.0+ tests are single-threaded so they're unaffected; the new
v0.12.0 stress tests exercise the concurrent path with `-race`.

### Stress test additions

`engine/indexed/concurrent_test.go` (in addition to the
existing `//go:build stress` file):

```go
func TestConcurrentExecuteSafe(t *testing.T) {
    e := buildPopulatedEngine() // ~1000 rules
    _ = e.Build()
    var wg sync.WaitGroup
    for g := 0; g < 16; g++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := 0; i < 10_000; i++ {
                _, _ = e.Execute(ctx, req)
            }
        }()
    }
    wg.Wait()
    // -race must not fire.
}
```

This is part of the regular test suite (NOT under `//go:build
stress`) because:
- Fast (~50 ms on Apple silicon).
- The race-detector check is the actual assertion; without it
  the test is meaningless.
- `make test` already runs with `-race`, so the test fits the
  default pipeline.

A second concurrent-listener test exercises the
copy-on-write Notifier.

### Cookbook: Hot-reload pattern

```go
// Caller code -- the library doesn't ship a Reloadable type.
type RuleService struct {
    current atomic.Value // holds *indexed.Engine
}

func (s *RuleService) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
    return s.current.Load().(*indexed.Engine).Execute(ctx, req)
}

func (s *RuleService) Reload(newRules []indexed.Rule) error {
    next := indexed.New()
    for _, r := range newRules {
        if err := next.AddRule(r); err != nil {
            return err
        }
    }
    if err := next.Build(); err != nil {
        return err
    }
    s.current.Store(next) // atomic swap; in-flight requests finish against the previous engine
    return nil
}
```

The cookbook section documents:
- Listeners attached to the old engine don't carry over.
  Re-attach on the new engine before the swap if you need them.
- The old engine remains referenced by in-flight goroutines
  until those return. Memory frees normally after that; no
  explicit close needed.
- The atomic.Value pattern is "single writer, many readers" —
  serialize Reload calls from the caller's side if reloads can
  overlap.

### BENCHMARKS.md additions

Two new measurements:

1. **Concurrent-Execute throughput.** N goroutines each running
   Execute against the same engine; report ns/op per goroutine.
   Expectation: linear scaling up to GOMAXPROCS minus a small
   constant (no lock contention).
2. **Cost of implicit-vs-explicit Build.** Compare the latency
   of the first Execute on an engine where Build was called
   ahead-of-time vs an engine that triggers implicit Build on
   the first request. The implicit case pays the build cost
   on the request; the explicit case has already paid it.

Both are reference numbers, not bars; concurrency isn't a single
success-bar shape because the metric is "scales with N goroutines"
not "X× faster than baseline."

The four v0.11.0 success-bar cells continue to gate per
single-threaded Execute (no change).

## Consequences

### Closed by v0.12.0

- `Execute` is safe for concurrent calls from any number of
  goroutines, with lockless read paths after Build (or after the
  first Execute, whichever comes first).
- The build-then-execute lifecycle is explicit: callers know when
  the engine is finalized and can validate before serving traffic.
- Hot reload becomes a documented one-liner using `atomic.Value`
  in caller code — no new library type required, no breaking API
  change.
- `engine/internal/adapter.Notifier` is concurrent-safe across
  all four adapters (the change benefits inmemory / firstmatch /
  priority too, but only indexed gets the build-then-execute
  lifecycle in v0.12.0).

### Still open after v0.12.0

- **Concurrent Execute on the linear adapters** (inmemory /
  firstmatch / priority). They share the Notifier fix but not
  the build lifecycle; their rule slices are still mutated by
  AddRule without sync. A future ADR ports the lifecycle if
  callers ask.
- **Higher-level hot-reload wrapper** (drain timeouts,
  generations, build-validation hooks). Out of scope; cookbook
  shows the pattern.
- **Structured execution telemetry** — v0.13.0 (ADR-0038).

### Performance impact

**Lockless read path** for Execute after Build:
- One `atomic.Value.Load` (single TLB lookup + type assertion).
  ~3-5 ns overhead vs today's direct struct field access.
- No mutex contention from concurrent Executes.
- Per-Execute alloc count unchanged (still 2/op per the alloc
  tripwire, since the snapshot is heap-allocated once at Build,
  not per call).

**AddRule cost**:
- Per call: mu.Lock + mu.Unlock + existing logic + mu.Unlock.
  Adds two atomic CAS ops (~10 ns) per AddRule.
- Total load-time cost grows ~1% on a 10k-rule load. Within
  noise.

**First Execute** (when Build wasn't called explicitly): pays
the build cost on the first request. For a 10k-rule engine this
is ~17 ms (from `make bench-load`). Callers that care about
P99 first-request latency should call Build explicitly during
startup.

The v0.11.0 alloc tripwire continues to pass at 2 allocs/op
(snapshot Load doesn't allocate; the Eval path is unchanged).

### Validation strategy

1. The new `TestConcurrentExecuteSafe` runs under `-race` in
   `make test`; gates `ci-local`.
2. A `TestConcurrentListenersSafe` exercises the Notifier
   copy-on-write under `-race`.
3. Lifecycle tests verify state transitions:
   - AddRule after Build returns ErrEngineBuilt.
   - Build twice returns ErrAlreadyBuilt.
   - WithPostFilterHook after Build panics (recovered + asserted).
   - Implicit Build on first Execute works without explicit call.
4. Pre-tag external scientific test demonstrates hot reload:
   build engine A, route traffic, hot-swap to engine B,
   verify subsequent requests hit engine B's rules.
5. Existing alloc tripwire (2/op) continues to pass.
6. v0.8.0 / v0.9.0 / v0.10.0 / v0.11.0 success-bar cells unchanged.

### Backward compatibility

- All v0.8.0–v0.11.0 callers continue to work without code
  changes. Implicit Build covers them.
- A caller who never calls Build but adds rules concurrently
  from multiple goroutines was already in undefined-behavior
  territory pre-v0.12.0; that case now returns
  `ErrEngineBuilt` after the first implicit Build instead of
  silently corrupting state. Net win.
- The new sentinels (`ErrEngineBuilt`, `ErrAlreadyBuilt`) are
  documented and discoverable. No existing callers reference
  them (they didn't exist).
