# 33. The Indexed Adapter

## Status

Proposed — target v0.8.0. Fourth concrete `engine.Engine` adapter,
the one ADR-0001 always pointed at. Must clear the v0.8.0 success
bar in [`BENCHMARKS.md`](../../../BENCHMARKS.md) (frozen by
ADR-0031) to ship as `v0.8.0`. If the bar is missed, the work stays
on a branch until it is met or this ADR is renegotiated.

## Context

Three earlier ADRs set this up:

- **ADR-0001** (the original "what's this project for") named indexed
  matching as the eventual production path: rule sets in the
  thousands need sub-linear lookup, and the existing linear adapters
  (`inmemory`, `firstmatch`, `priority`) all walk every rule on every
  `Execute`.
- **ADR-0028** (the typed Condition tree, v0.6.0) explicitly framed
  itself as "groundwork for indexed-matcher analysis." Rules built
  through `parser.ParseToCondition` produce inspectable ASTs whose
  shape an adapter can introspect at `AddRule` time — exactly what an
  index builder needs.
- **ADR-0031** (the bench harness, v0.7.1) pre-committed the
  performance bar: ≥10× firstmatch at 1k rules / 5 dims / NoHit,
  ≥50× at 10k / 5 dims / NoHit, within 2× at 10 rules
  (anti-regression). The bar exists precisely so this ADR's success
  is decidable without subjective judgment.

The indexed adapter's value proposition: for the **subset of rules
expressible as conjunctions of equality predicates**, sub-linear
lookup via hashed buckets. For everything else, fall back or
reject — depending on how this ADR draws the line.

Five design questions:

**1. What rule shape does the adapter accept?**

Options:

- **(a)** Reuse the existing opaque `func(input interface{}) bool`
  shape and a separate metadata struct (`Constraints
  map[string]string`) the caller sets in parallel. The adapter
  trusts the metadata; the closure is the actual evaluator.
- **(b)** Require a `parser.Condition` (the typed tree from
  ADR-0028). The adapter walks the tree to extract structure and
  evaluates it at match time as a defensive check.

Pick **(b)**. The whole reason ADR-0028 exists is so we do not have
two sources of truth (the closure says one thing, the metadata says
another, they drift). One source of truth — the typed `Condition` —
fed to one analyzer at `AddRule` time. Rules constructed with
`parser.ParseToCondition` are first-class inputs; rules constructed
programmatically use the same `StringCondition`/`AndCondition`
builders.

**2. What is "indexable" and how do we draw the line?**

Indexable shapes are the ones whose semantics reduce to "this fact
key equals this value." Concretely, a rule is indexable iff its
`Match` is either:

- A single `StringCondition{Op: OpEq, Field, Value}` — one-dim rule.
- An `AndCondition` whose flattened children are *all* `OpEq`
  `StringCondition`s — multi-dim rule.

Three options for non-indexable rules:

- (i) **Reject at AddRule.** Stricter; the adapter's contract is
  "indexable rules only." Errors surface immediately rather than at
  Execute time.
- (ii) **Accept and fall back to scan.** Mixed mode: indexable rules
  go in the index, non-indexable join a "scan list" evaluated on
  every Execute. Looser contract; consistent behavior with
  surprising perf.
- (iii) **Accept and ignore non-indexable terms.** Indexable terms
  steer the lookup; non-indexable terms run as post-filters. Most
  flexible, most surprising.

Pick **(i)** for v0.8.0. The performance bar is the headline; mixed
mode would let an incidental non-indexable rule turn a 10k-rule
Execute back into a linear scan. A new sentinel —
`ErrNonIndexableCondition` — names the failure. Callers who need
mixed mode get it via a future adapter (`engine/hybrid`) or via
`ChainProviders` composing different adapter pools.

**3. How are indexable rules grouped?**

A rule's *key set* is the sorted tuple of fact-key names it
constrains. Examples:

- `country == "BR" AND tier == "premium"` → key set `(country, tier)`.
- `product == "flight"` → key set `(product,)`.
- `country == "BR" AND product == "flight" AND tier == "premium"` →
  key set `(country, product, tier)`.

A real rule set has a small number of distinct key-set shapes (a
dozen, typically) and many rules per shape. The adapter bucketizes
rules by **(key-set, value-tuple)**:

```
engine.buckets : map[keysetID]*keysetBucket
keysetBucket.byValueKey : map[valueTupleString][]Rule
```

On `Execute`, the adapter walks every known `keysetID`, projects the
input fact map onto that key set, looks up the value-tuple bucket,
and returns on first match. Cost is O(K) hash lookups where K is the
number of distinct key sets in the engine — typically a small
constant, independent of N (the total rule count).

The order in which key-sets are walked is **insertion order of
first-seen rule per key-set**. Deterministic, debuggable, and
matches the conceptual "specificity prefers first-declared shape"
intuition of insertion-order tie-break (ADR-0019).

**4. What input shape does Execute accept?**

Indexed lookup requires a fact map. The adapter accepts:

- `map[string]string` (the canonical "routing fact" shape used by the
  bench harness).
- `map[string]interface{}` (broader; values must be comparable
  strings via fmt or assertion).

Anything else returns `ErrIncompatibleInput` — a new sentinel — at
the top of `Execute`. The linear adapters happily accept any
`interface{}` because their conditions are opaque closures; the
indexed adapter cannot.

**5. What about semantics: first-match or all-match?**

The performance bar in `BENCHMARKS.md` compares against `firstmatch`.
For semantic continuity, the indexed adapter is **first-match**: it
walks key-sets in insertion order, walks rules within a bucket in
insertion order, and returns on the first matching rule. An
all-match variant would be a separate adapter (`engine/indexedall`)
if a real caller asks.

When multiple rules within the *same* bucket all match (i.e.,
identical key-set + value-tuple), insertion order picks the winner.
This matches the existing tie-break rule from ADR-0019.

## Decision

Add `engine/indexed` exporting:

```go
package indexed

// Rule is a typed-Condition rule for the indexed adapter.
// Match must be a pure conjunction of OpEq StringConditions;
// AddRule returns ErrNonIndexableCondition for anything else.
type Rule struct {
    Name        string
    Description string
    Tags        []string
    Match       parser.Condition
    Action      func(input interface{}) interface{}
    ActionContext func(ctx context.Context, input interface{}) interface{}
}

// Engine is a first-match adapter with O(K) Execute (K = number of
// distinct key sets among registered rules).
type Engine struct { /* unexported */ }

func New() *Engine
func (e *Engine) AddRule(r Rule) error
func (e *Engine) Execute(ctx context.Context, req engine.Request) (engine.Result, error)

// Sentinels.
var ErrEmptyRuleName        = errors.New("indexed: rule name is empty")
var ErrNilMatch             = errors.New("indexed: rule.Match is nil")
var ErrDuplicateRuleName    = errors.New("indexed: rule name already registered")
var ErrNonIndexableCondition = errors.New("indexed: Match is not a pure conjunction of equality conditions")
var ErrIncompatibleInput    = errors.New("indexed: Execute input must be map[string]string or map[string]interface{}")
```

The `Engine` struct embeds `engine/internal/adapter.Notifier` (per
ADR-0029) so `AddListener` + the four lifecycle notifications come
for free.

### AddRule algorithm

1. Validate `Name != ""`, `Match != nil`, name uniqueness — same
   shape-first / state-second order the other adapters use.
2. Walk `Match` to extract `(field, value)` pairs:
   - `*parser.StringCondition{Op: OpEq}` → one pair.
   - `*parser.AndCondition` → recursively walk children; require all
     to be `OpEq` `StringCondition`s (after recursion); collect the
     pairs.
   - Anything else → return `ErrNonIndexableCondition`.
3. Sort `(field, value)` pairs by field name to canonicalize.
4. Compute `keysetID` (sorted field names joined by `\x1f`) and
   `valueKey` (sorted `field=value` pairs joined by `\x1f`). The
   `\x1f` (unit separator) byte is a safe joiner that cannot appear
   in normal facts.
5. Append the rule to `e.buckets[keysetID].byValueKey[valueKey]`,
   creating either as needed. Also record the insertion order of the
   keysetID itself so Execute can walk in stable order.

### Execute algorithm

1. Coerce `req.Input` to `map[string]string` (the `map[string]interface{}`
   case stringifies values via `fmt.Sprintf("%v", ...)`; nil and any
   other shape return `ErrIncompatibleInput`).
2. Notify started.
3. For each `keysetID` in insertion order:
   - Project the input onto the key set: build `valueKey` from the
     input's values at those fields. If the input is missing a field
     the key set requires, skip this key set.
   - Look up `e.buckets[keysetID].byValueKey[valueKey]`.
   - For each candidate rule, defensively call `rule.Match.Eval(fact)`
     — should always return true by construction; if it ever returns
     false in production, that is a bug worth knowing about, not
     hiding.
   - On first true: run the action (with panic recovery, matching
     other adapters), notify matched + finished, return.
4. Notify finished with empty result, return.

### Tests + benchmarks

- `engine/indexed/indexed_test.go` — the usual unit + behavior
  battery, runs the `enginetest.RunContractTests` shared suite.
- `engine/indexed/allocs_test.go` — alloc tripwire per ADR-0032
  shape.
- The bench matrix in `engine/enginetest/bench` extends to include
  an `indexed` row in every cell. A new `bench.RuleSpec` and a
  `StructuredSeedFunc` carry the rule's structural form (the
  bench-generated rules are already pure conjunctions of equality,
  so this is just exposing what is already there).
- A new dedicated benchmark file (`engine/indexed/bench_test.go`)
  runs the four success-bar cells specifically and asserts the
  multipliers in a normal `*testing.T` via captured benchmark
  results. **A failed multiplier is a test failure**, not just a
  number to read. The bar enforces itself.

### Bench-harness extension

`engine/enginetest/bench` gains:

```go
type RuleSpec struct {
    Name      string
    KeyValues map[string]string // conjunction of equality, by field name
}

type StructuredSeedFunc func(spec RuleSpec) error
type StructuredFactory func() (engine.Engine, StructuredSeedFunc)

func PopulateStructured(seed StructuredSeedFunc, w Workload) (Input, error)
func RunStructured(b *testing.B, w Workload, factory StructuredFactory)
```

The existing `SeedFunc` / `Factory` / `Run` shapes stay unchanged —
linear adapters keep using them. Indexed (and any future adapter
that introspects rule shape) uses the structured variant. Both
produce the same `Input` and the same logical workload for the same
`Workload` value — the **only** difference is whether the rule is
handed over as an opaque closure or a structural spec.

## Consequences

The library gains its first sub-linear adapter, completing what
ADR-0001 always promised the engine port was for. For indexable rule
shapes the cost of `Execute` becomes O(K) where K is the count of
distinct key sets — bounded by the rule designer's choices, not by
the rule count.

The contract is tighter than the linear adapters. Indexed rules must
be expressible as conjunctions of equality predicates and inputs
must be fact-maps. Callers whose rule shapes do not fit keep using
the linear adapters; the four adapters are now genuinely different
tools for different shapes, not three near-duplicates.

The bench harness gains a parallel structured rule API. This is
additive (existing benchmarks unchanged) and tracks ADR-0031's
forward-compat promise.

The performance bar in `BENCHMARKS.md` is the release gate.
v0.8.0's tag commit walks the matrix and the dedicated
multiplier-asserting tests pass; if any cell fails, the tag does
not get cut.

Future ADRs extend the indexed adapter's reach:

- **0034** — wider indexable shapes: `OpIn` (set membership) as a
  multi-key bucket index, `OpNeq` via post-filter, range predicates
  via interval-tree-style auxiliary structures.
- **0035** — the `IndexDimension` framework: callers declare custom
  index dimensions (e.g., hierarchical lookup, geo proximity) that
  the adapter can leverage.

Both stay out of scope for v0.8.0. Land the simplest sub-linear
adapter that clears the bar, document the bar was cleared, ship it.
