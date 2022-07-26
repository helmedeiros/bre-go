# 35. Negation in `engine/indexed` and Value-Expression Syntax in `engine/parser`

## Status

Accepted — landed in v0.10.0. `engine/indexed` admits `OpNeq` and
`OpNotIn` via a per-rule post-filter applied after bucket hits;
pure-negation rules return the new `ErrNoIndexableTerms`.
`engine/parser` ships `ParseValueExpression(field, value)` for
CSV-shaped callers. Both v0.10.0 success-bar cells clear at >110×
(1k/Last) and >2 900× (10k/NoHit); existing v0.8.0 and v0.9.0
bars hold.

## Context

After v0.9.0, the indexed adapter covers:

- `StringCondition{Op: OpEq}` — bucketed.
- `SetCondition{Op: OpIn}` — bucketed via Cartesian-product fan-out.
- Field omission — wildcard, walked via the key-set mechanism.

Negation remains rejected. That's the next obvious widening:
real-world rule sets have "everything except X" rules
(`!flight`, `!premium`) that don't fit any of the existing
shapes.

Negation cannot be cleanly fanned out the way set membership can. A
rule `country != BR` semantically constrains `country` to one of
{every country except BR} — an unbounded set the engine has no way to
enumerate at `AddRule`. Three viable strategies:

1. **Fan-out against a caller-supplied `knownValues` set.** Caller
   tells the engine "the universe of countries is {BR, AR, CL,
   US, ...}"; the negation expands to the complement. Requires a
   new piece of API surface (knownValues per dimension) and
   silently breaks when a new value arrives.
2. **Post-filter pass.** Keep the negation as a runtime predicate
   on the rule's bucket entry. The indexed lookup narrows the
   candidate set via the indexable terms; the post-filter applies
   any negations to surviving candidates.
3. **Scan-list fallback.** A separate non-bucketed list, walked
   linearly on every Execute. Destroys the sub-linear claim;
   ruled out immediately.

This ADR picks (2). It preserves the v0.8.0/v0.9.0 single-hash-probe
path for everything indexable, and pays a small post-filter cost only
for rules that actually carry non-indexable terms. The cost is
bounded by the number of bucket hits, which is typically 1–2 per
Execute.

Strategy (1) stays available for v0.11.0 if a real consumer needs
the perf characteristics. The IndexDimension framework will handle
"knownValues per dimension" cleanly as one of its built-in patterns.

A second open question for v0.10.0: **CSV-shaped rule sources can't
express operators positionally.** A CSV row has one cell per
dimension; the cell contains the rule's value constraint for that
dimension. Decision-table CSV conventions encode operators inside
the cell value: `!flight` (negation), `flight|train` (alternatives),
`*` (wildcard). Today our CSV loader has no way to translate these
into typed Conditions; callers have to write per-cell parsing
themselves. v0.10.0 adds a `ParseValueExpression(field, value)`
helper in `engine/parser` so this translation is centralized,
tested, and consistent across consumers.

Four design questions.

### 1. Where does the post-filter live structurally?

Two options:

**(a) On the `indexedRule` struct itself.**

```go
type indexedRule struct {
    name       string
    action     ...
    ctxAct     ...
    postFilter []parser.Condition // NEW: non-indexable terms
}
```

Each bucket entry carries the post-filter list. At Execute, after
a bucket hit, the engine evaluates every post-filter against the
fact; if any returns false, skip the rule.

**(b) On the `keysetBucket` as a separate index.**

Rules with post-filters live in a parallel structure that maps the
indexable value-key to a list of `(rule, post-filter)` pairs.

Pick **(a)**. The post-filter is per-rule; storing it per-rule is
the natural fit. Option (b) would require duplicating the bucket
walk for the post-filter path and managing two parallel structures
in lock-step. Adding a field to `indexedRule` is the minimum
viable change.

### 2. Do we admit rules with *only* non-indexable terms?

A rule whose `Match` is `country != BR` (and nothing else) has no
indexable term. It would fire for any input where `country != BR` —
which is most inputs in a large value space. To handle it in the
indexed adapter, we'd need either:

- **A scan-list walked on every Execute.** Adds linear cost to
  every request regardless of whether such rules exist. Rejected.
- **A "negation bucket" per field.** A separate structure that
  indexes by *negated value*; Execute probes both the equality
  bucket and the negation bucket. Complex, doubles the per-key-set
  cost.

Pick **reject** for v0.10.0. A rule must have ≥ 1 indexable
(OpEq / OpIn) term. Returns a new sentinel
`ErrNoIndexableTerms` from `AddRule`, error message points at
v0.11.0's IndexDimension framework as the intended home for
pure-negation shapes.

For callers who have pure-negation rules today: use one of the
linear adapters, or split the rule by adding a known-bounded
indexable term (e.g., `country != BR AND product IN (flight,
train)` works in v0.10.0).

### 3. What's the value-expression grammar?

The grammar recognizes a small set of patterns inside a single
string, mirroring conventions common in decision-table CSV
formats:

```
value-expr := plain-value
            | wildcard
            | negation
            | alternatives

plain-value  := <any non-special string>
wildcard     := "*"
negation     := "!" plain-value
alternatives := plain-value ("|" plain-value)+
```

Combinations like `!a|b` are **not** in this grammar — parse
error. Each cell is either negation OR alternatives, not both.
Real-world rule sources rarely mix the two; the restriction is
conservative but matches actual usage.

Special-character escaping (a literal `|` or `!` inside a value)
is **out of scope** for v0.10.0. Real-world rule values for
business-key fields (country, product, tier, carrier) don't
contain these characters; adding escape syntax now is premature.
If a real consumer hits the issue, a future ADR adds
backslash-escape or quoted-literal support.

### 4. Where does `ParseValueExpression` live?

Two options:

- (a) `engine/parser.ParseValueExpression(field, value string)`.
- (b) A new sub-package `engine/parser/cellexpr` or similar.

Pick **(a)**. The existing `parser` package owns the typed
`Condition` AST; the new function is a thin alternative entry
point that returns the same Condition types. A sub-package would
fragment the API for no benefit.

Naming nit: `ParseValueExpression` is descriptive but long.
Considered alternatives: `ParseCell`, `ParseValueAtom`,
`ParseColumnValue`. Settled on `ParseValueExpression` because the
existing `Parse` and `ParseToCondition` set a precedent and the
asymmetry matches the call site (loaders, not direct callers).

## Decision

Two parallel changes.

### Change 1 — `engine/indexed` admits non-indexable terms as post-filter

The walk over `Match` splits each child into two buckets:

- **Indexable** (OpEq StringCondition, OpIn SetCondition) →
  contributes a `fieldValueSet` to the bucket key.
- **Non-indexable** (OpNeq StringCondition, OpNotIn SetCondition,
  any other typed Condition we don't recognize as indexable) →
  stays as a runtime predicate.

```go
type indexedRule struct {
    name       string
    action     func(input interface{}) interface{}
    ctxAct     func(ctx context.Context, input interface{}) interface{}
    postFilter []parser.Condition // empty for rules with no negation
}
```

`AddRule` returns `ErrNoIndexableTerms` if the rule has zero
indexable terms after the walk. The existing
`ErrNonIndexableCondition` is unchanged; it now means "shape we
do not understand," distinct from "shape understood, but pure
negation."

At Execute, after the bucket hit but before notifying matched:

```go
for _, cand := range candidates {
    if !passesPostFilter(cand.postFilter, fact) {
        continue
    }
    // ... existing matched / action / notify path
}
```

`passesPostFilter` evaluates every condition in the rule's
`postFilter` slice against the fact (coerced to
`map[string]interface{}` for `parser.Condition.Eval`). Returns
true iff all pass; false on first failure.

The bucket invariant (rules-in-this-bucket-match-the-value-key)
holds for the indexable part. The post-filter then handles the
non-indexable part. Together: each candidate matches iff bucket
membership AND post-filter both succeed.

### Change 2 — `engine/parser` exports `ParseValueExpression`

```go
// ParseValueExpression turns a single value-cell string into a
// typed Condition over field. Recognized shapes:
//
//   "BR"      -> StringCondition{Field: field, Op: OpEq,  Value: "BR"}
//   "!BR"     -> StringCondition{Field: field, Op: OpNeq, Value: "BR"}
//   "BR|AR"   -> SetCondition{Field: field, Op: OpIn, Values: ["BR","AR"]}
//   "!BR|AR"  -> *ValueExpressionError (mixed negation+alternatives)
//   "*"       -> nil, nil  (wildcard / no constraint)
//   ""        -> nil, nil  (empty / no constraint)
//
// Whitespace around values is trimmed. Empty alternatives
// ("BR||AR") return *ValueExpressionError.
func ParseValueExpression(field, value string) (Condition, error)

type ValueExpressionError struct {
    Field string
    Value string
    Cause string
}

func (e *ValueExpressionError) Error() string
```

Returns `(nil, nil)` for wildcards / empty cells so callers can
unconditionally append the result to an `AndCondition`'s children
and drop nils. This avoids special-casing wildcard at the
caller.

The function lives next to `Parse` and `ParseToCondition`.
Standalone — does not call into the operator-level parser. The
two surfaces stay decoupled; the existing operator-level DSL is
unchanged.

### BENCHMARKS.md additions

Two new bar cells for v0.10.0:

| Cell | Required vs firstmatch |
|---|---:|
| 1k rules, 5 dims, 1 of 5 fields uses `OpNeq` post-filter, `Last` | ≥ 5× faster |
| 10k rules, 5 dims, 1 of 5 fields `OpNeq`, `NoHit` | ≥ 30× faster |

The 30× target (vs 50× in v0.8.0) acknowledges that post-filter
adds a small per-candidate cost. Even at the relaxed bar, indexed
remains overwhelmingly faster than the linear adapters for these
workloads.

Existing v0.8.0 + v0.9.0 bars must continue to pass; v0.10.0 must
not regress equality / OpIn paths.

### Bench-harness extension

`Workload` gains `OpNeqDims int` — number of dims that become
negation predicates instead of equality / OpIn. The structured
seed populates the appropriate `parser.StringCondition{Op: OpNeq}`
for those dims; the closure seed implements equivalent
set-difference logic so the firstmatch baseline measures the same
predicate.

`RuleSpec` does not gain a new field; the negation is expressed
via an additional value-pair entry in a new optional map (TBD --
the implementation will pick the cleanest shape).

## Consequences

### Closed by v0.10.0

- `StringCondition{Op: OpNeq}` is now an indexable rule when
  combined with at least one OpEq / OpIn term.
- `SetCondition{Op: OpNotIn}` admitted on the same terms.
- CSV-shaped value cells with `!`, `|`, `*` syntax can be
  translated to typed Conditions via a single library call
  (`ParseValueExpression`). Callers stop writing per-cell parsers.

### Still open after v0.10.0

- Pure-negation rules (no indexable term). Rejected today; the
  IndexDimension framework in v0.11.0 is the intended home.
- Mixed negation + alternatives at value level (`!a|b`). Out of
  scope.
- Escape syntax for literal `!`, `|`, `*` inside values. Out of
  scope.
- Numeric range (`>=`, `<=`). v0.11.0 via IndexDimension.

### Performance impact

The post-filter adds one `Condition.Eval` call per candidate
*per non-indexable term* in the rule. For workloads where rules
have at most one or two negation terms, this is single-digit
nanoseconds per bucket hit. The success bar absorbs it without
issue; the new 30× threshold for the 10k-cell is a deliberate
margin in case the post-filter shows up in run-to-run noise.

Rules without any negation terms (the v0.8.0 / v0.9.0 case)
have a `nil` `postFilter`. The Execute hot path checks
`len(cand.postFilter) == 0` and skips the Eval entirely. **No
perf cost for callers who never touch OpNeq.**

### Memory impact

A rule with one OpNeq term holds one extra `parser.Condition`
interface (16 bytes on 64-bit) in its bucket entry. Negligible.
Rules without negation hold a nil slice; no per-rule cost.

### Validation strategy

The success-bar tests for v0.10.0 follow the same pattern as
v0.9.0: run `firstmatch` and `indexed` live on OpNeq-bearing
workloads in the same test process, assert the multiplier in a
regular `*testing.T`. Two new `TestSuccessBar_OpNeq_*` tests
gate `ci-local`.

Pre-tag scientific test: 6+ scenarios in an external module
verifying single-OpNeq, mixed OpEq+OpNeq, pure-OpNeq rejection,
value-expression round-trip, wildcard cell handling, and the
empty-alternatives error path.

### What this validates for v0.11.0

The post-filter pattern from v0.10.0 generalizes naturally to
custom predicates the IndexDimension framework will introduce:
range checks (`amount >= 100`), prefix matches (`country
LIKE "B%"`), and hierarchical lookups (`region IN
descendants-of("Americas")`) all sit on the same hook. v0.11.0
formalizes the predicate-interface part; v0.10.0 makes the
mechanism real with the simplest non-trivial example.
