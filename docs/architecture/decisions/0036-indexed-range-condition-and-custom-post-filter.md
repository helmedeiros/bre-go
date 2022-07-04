# 36. Numeric Range and Caller-Defined Custom Conditions in `engine/indexed`

## Status

Proposed — target v0.11.0. Third of five Phase-4 parity-closure
releases. Adds numeric-range matching (a long-requested
parity-target shape) and the small extension hook that lets callers
plug in their own typed `Condition` shapes without forking the
package.

## Context

After v0.10.0 the indexed adapter natively recognizes:

- `StringCondition{Op: OpEq}` — bucket-key contributor
- `SetCondition{Op: OpIn}` — bucket-key contributor (fan-out)
- `StringCondition{Op: OpNeq}` — post-filter
- `SetCondition{Op: OpNotIn}` — post-filter
- Field omission — wildcard via key-set walking

What's still missing relative to the parity target:

- **Numeric range** (`amount >= 100 AND amount <= 500`). Real
  production rule sets carry these for tariff bounds, eligibility
  windows, etc.
- **Caller-defined custom shapes**. The parity target has an
  abstract `IndexDimension` mechanism that lets the embedding
  application teach the engine about new dimension types
  (hierarchical lookups, geo proximity, prefix matches). Today our
  adapter has a closed switch — anything not in the four
  recognized shapes hits `ErrNonIndexableCondition`.

v0.11.0 addresses both. The framework piece is deliberately small:
callers register a `PostFilterHook` that classifies additional
condition types, the adapter calls into the hook when its built-in
classifier doesn't recognize a shape. This is a thin contract —
not the full per-dimension index-strategy framework the parity
target has — because v0.11.0's success-bar gating forces us to
ship something that demonstrably works rather than something
ambitious that needs another release to validate.

Three design questions.

### 1. What shape does numeric range take?

A new typed Condition in `engine/parser`:

```go
type RangeCondition struct {
    Field string
    Min   float64
    Max   float64
}
```

Inclusive bounds: `Min <= value <= Max`. Open-ended ranges use
`math.Inf(-1)` / `math.Inf(+1)`. Float64 covers both integer and
fractional values; callers comparing integer values pay the float
conversion cost (negligible).

`Eval(fact map[string]interface{})` parses the input field's
value as float64 and returns:

- `true` if Min ≤ value ≤ Max.
- `false` if the field is missing or the value isn't parseable
  as a number.

Reasons to reject other shapes:

- **Half-open intervals**: `[Min, Max)` is the convention in many
  range libraries but reads ambiguously when written as
  `RangeCondition{Min: 100, Max: 200}`. Inclusive is what humans
  type when they say "100 to 200." Future ADR adds
  `RangeConditionExclusive` if a real consumer asks.
- **String comparison** (lexicographic ranges): different
  semantic, different field type. A `StringRangeCondition` is a
  separate ADR.
- **Time ranges**: encode as float64 (Unix timestamp) or as
  ISO-8601 strings + a custom dimension. Out of v0.11.0 scope.

### 2. Bucket-key contributor or post-filter?

Range cannot become a bucket-key contributor via the existing
fan-out path: an unbounded interval `[100, +Inf)` would expand to
infinity bucket entries. Two alternatives:

**(a) Per-field interval index.** Maintain a sorted structure
(interval tree, sorted-bounds list) per range-constrained field.
At Execute, query the structure with the input value, get matching
rule IDs, intersect with the equality-bucket result.

**(b) Post-filter.** Range conditions become entries on the
rule's post-filter list. At Execute, after the equality-bucket
hit, evaluate range conditions against the input value.

Pick **(b)** for v0.11.0. Post-filter reuses the existing
mechanism from ADR-0035; cost is O(C) per bucket hit where C is
the number of range conditions on the rule (typically 1-2). The
per-field interval index would be O(log N) instead of O(C), but
the constant factors are much larger (interval-tree traversal,
list intersection) and the win only matters when bucket hits are
large.

If a real consumer ships ≫1k rules where every rule has a range
constraint and bucket hits are large, the per-field interval
index becomes worthwhile and ships in a follow-up ADR. v0.11.0
keeps the contract simple.

### 3. How does caller extension work?

Three options:

**(a) Caller defines a custom `parser.Condition` type, the
adapter has no idea what it is and rejects it.** Status quo. No
extension.

**(b) Caller registers a custom typed handler with the adapter.**
A typed registry maps `reflect.Type` → handler. AddRule looks up
the handler when it hits an unknown Condition. Type-reflective,
allocates at AddRule time.

**(c) Caller registers a *classification hook* — a single
function that the adapter calls for every Condition the built-in
classifier doesn't recognize.** Functional, no reflection, no
registry.

Pick **(c)**. The hook signature:

```go
type PostFilterHook func(c parser.Condition) (handled bool)

func (e *Engine) WithPostFilterHook(h PostFilterHook) *Engine
```

The hook returns `true` iff it recognizes `c` as a valid
post-filter shape. When the built-in classifier hits an unknown
Condition AND the hook returns `false`, the adapter falls back
to `ErrNonIndexableCondition`. When the hook returns `true`, the
adapter treats `c` as a post-filter (it appears in
`indexedRule.postFilter` and is `Eval`'d at Execute time).

This is the minimum surface that lets callers extend the adapter
without modifying the package. The hook does not give callers
control over bucket-key contribution — that's reserved for a
future ADR that introduces a wider framework if real consumers
need it. Today's hook is "classify as post-filter or reject" —
which is the right surface for shapes like prefix-match, regex,
hierarchical lookup, and caller-specific custom predicates.

Callers who want a *bucketed* custom dimension (e.g., geo S2
cells) need to wait for a follow-up release with a richer
framework. v0.11.0 documents this gap and points at the future
ADR.

## Decision

Two surfaces.

### Change 1 — `parser.RangeCondition`

New typed Condition in `engine/parser`:

```go
type RangeCondition struct {
    Field string
    Min   float64
    Max   float64
}

func (c RangeCondition) Eval(fact map[string]interface{}) bool
```

`Eval` parses the field's value via `strconv.ParseFloat` on the
string form (matching how `StringCondition.Eval` and
`SetCondition.Eval` work). Returns false if the field is missing,
non-string, or non-numeric. Inclusive bounds:
`Min <= parsed <= Max`.

`math.Inf(-1)` and `math.Inf(+1)` are valid bound values for
open-ended intervals. `Min > Max` is a degenerate rule that never
matches — accepted at construction, not at Eval (allows callers
to build pathological ranges if they really want to).

Pointer form (`*RangeCondition`) accepted by the adapter, mirroring
the StringCondition / SetCondition pattern.

### Change 2 — `engine/indexed` recognizes `RangeCondition` as post-filter

`classifyStringCondition` / `classifySetCondition` are joined by
`classifyRangeCondition`:

```go
func classifyRangeCondition(v parser.RangeCondition, sets *[]fieldValueSet, post *[]parser.Condition) error {
    *post = append(*post, v)
    return nil
}
```

`collectSets` adds two new cases (value and pointer) routing to
the new classifier. Pure-range rules (no indexable term) return
`ErrNoIndexableTerms` — same path as pure-negation rules from
v0.10.0.

### Change 3 — `Engine.WithPostFilterHook`

```go
// PostFilterHook is a caller-supplied classifier for typed
// Conditions the indexed adapter does not natively recognize.
// When the hook returns true, the adapter treats the Condition
// as a post-filter (appended to indexedRule.postFilter and
// Eval'd against the input at Execute time).
type PostFilterHook func(c parser.Condition) (handled bool)

// WithPostFilterHook installs h. Subsequent AddRule calls will
// consult the hook before returning ErrNonIndexableCondition.
// Returns the engine to allow method chaining.
//
// Multiple calls overwrite the previous hook. Only one hook is
// active at a time.
func (e *Engine) WithPostFilterHook(h PostFilterHook) *Engine
```

The hook is consulted only for conditions the built-in classifier
doesn't recognize. Built-in shapes (OpEq, OpIn, OpNeq, OpNotIn,
RangeCondition) always go through the native path — the hook
cannot override their behavior.

The hook is engine-scoped: different engines can have different
hooks. Hooks installed *after* `AddRule` calls only affect
subsequent calls — already-registered rules retain whatever
classification they had at AddRule time.

### BENCHMARKS.md additions

Two new bar cells:

| Cell | Required vs firstmatch |
|---|---:|
| 1k rules, 5 dims, 1 of 5 is `RangeCondition`, `Last` | ≥ 5× faster |
| 10k rules, 5 dims, 1 `RangeCondition`, `NoHit` | ≥ 30× faster |

The 30× bar matches v0.10.0's relaxed OpNeq threshold; range Eval
has similar per-candidate cost (parse float + compare).

### Bench-harness extension

`Workload` gains `RangeDims int`. The structured generator emits
`RangeCondition`s for the tail dims (after OpNeq dims if any).
The closure generator implements equivalent numeric-range checks.

## Consequences

### Closed by v0.11.0

- Numeric range matching is now an indexable shape (as a post-
  filter). Rules with `amount >= 100 AND amount <= 500` types
  ride the sub-linear matcher.
- Callers can extend the adapter to recognize their own typed
  Condition shapes as post-filters. The hook mechanism is small
  but real — it's the first time the adapter is open for
  extension.

### Still open after v0.11.0

- **Caller-defined *bucketed* dimensions** (geo S2 cells,
  hierarchical lookups that contribute to the bucket key). v0.11.0
  only opens post-filter extension. A wider framework lands in a
  later release.
- **Per-field interval index** for range workloads where bucket
  hits are large. Today's post-filter is fine for typical workloads
  (1-2 range terms per rule, ≤ a few hits per bucket); a future
  ADR adds the interval index if measurement shows it's needed.
- **String range** (lexicographic comparison): separate
  StringRangeCondition shape, separate ADR.
- **Concurrency / hot reload** — v0.12.0.
- **Structured telemetry** — v0.13.0.

### Performance impact

`RangeCondition.Eval` parses the field value via
`strconv.ParseFloat` (~30 ns on modern hardware), then compares
two floats (~1 ns). Per matched candidate, ~31 ns total. Similar
order of magnitude to OpNeq's ~190 ns per candidate (which
includes a map lookup + string compare).

The success-bar cells are calibrated against firstmatch's
linear scan, where each rule's range check also pays
`strconv.ParseFloat`. Both sides pay the parse cost; the ratio
is dominated by the linear-vs-sub-linear difference.

For rules without RangeCondition (the v0.8.0 / v0.9.0 / v0.10.0
case): zero impact. Same nil-postFilter fast path.

### Memory impact

A rule with one RangeCondition holds one extra Condition value
(24 bytes for the struct, plus the field string). Negligible.

### Validation strategy

Same pattern as v0.9.0 and v0.10.0:

1. Unit tests covering RangeCondition.Eval for all the edge
   cases (missing field, non-numeric, inclusive bounds, infinity).
2. Indexed-adapter integration tests for the new condition
   admitted as post-filter + pure-range rejection.
3. Two new `TestSuccessBar_*` tests gating ci-local.
4. A pre-tag external module with at least one scenario per
   change: range happy path, infinite bounds, custom hook
   classifying a caller-defined shape.

### What this validates for v0.12.0+

The PostFilterHook pattern sets the precedent for caller-side
extension. v0.12.0's concurrency work (build-then-execute,
snapshot swap) needs to define how custom hooks interact with
those lifecycles — the hook itself stays constant per engine but
the engine's rule set rebuilds, so the question is whether
already-registered rules retain their hook-classified post-filters
after a snapshot swap. v0.11.0 documents the per-rule
classification-at-AddRule-time contract, which v0.12.0 inherits.

If a real consumer needs *bucketed* custom dimensions before
that release, the right hammer is a separate `engine/custom`
adapter that gives them full control — composing via
`ChainProviders` and adapter-per-shape lets them stay productive
while the framework matures.
