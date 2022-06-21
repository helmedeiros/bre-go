# 34. Set-Membership and Wildcard Semantics in `engine/indexed`

## Status

Accepted — landed in v0.9.0. `OpIn` set-membership admitted via
bucket fan-out at `AddRule`; new `FanoutTooLargeError` for
overflow. Wildcard semantics documented and locked in by tests;
no production-code change needed. Both v0.9.0 success-bar cells
in `BENCHMARKS.md` clear at >200× firstmatch (1k OpIn/Last) and
>2 600× (10k OpIn/NoHit). Existing v0.8.0 bar still holds.

## Context

ADR-0033 shipped the indexed adapter with a deliberately tight
shape contract: `Match` must be a pure conjunction of
`StringCondition{Op: OpEq}`. `SetCondition` is rejected at
`AddRule` with `ErrNonIndexableCondition`, as is anything with
negation, disjunction, or range predicates.

That was the right call at the time — the v0.8.0 success bar gates
the headline perf claim and we wanted no contract surface beyond
what the bucket-walk algorithm cleanly handles. v0.9.0 starts the
incremental widening.

The first two shapes to add:

1. **`SetCondition{Op: OpIn}`** — "field is one of these N values."
   Real-world rule sets are full of this shape: "carrier in
   {alpha, beta, gamma}", "tier in {premium, vip}",
   "country in {BR, AR, CL}". Today such rules can only ride on the
   linear adapters.
2. **Wildcard semantics** — a rule that does not constrain a
   particular field. Currently a rule's `Match` only references
   the fields it cares about, and the indexed adapter handles this
   correctly via key-set walking. We document this and lock it in
   with a test + bar cell so the property cannot drift.

`SetCondition{Op: OpNotIn}` and `StringCondition{Op: OpNeq}` stay
rejected in v0.9.0 — those are negation shapes and need a
post-filter pass that v0.10.0 introduces.

Three design questions.

### 1. How does `OpIn` map to bucket structure?

Two viable approaches:

**(a) Multiple bucket entries (fan-out at `AddRule`).** A rule
`country IN ("BR", "AR")` gets inserted into both the
`country=BR` and the `country=AR` bucket. The walker probes by
input value; whichever bucket matches the input, finds the rule.
No new code in the hot path; only `AddRule` changes.

**(b) A second-level set-membership check at `Execute`.** Keep the
rule in a single bucket keyed by a special "any of these" marker;
on probe, check the candidate's value set against the input.

Pick **(a)**. Fan-out preserves the single-hash-probe Execute path
that ADR-0033's success bar protects. Approach (b) would
re-introduce a per-candidate predicate check in the hot loop —
exactly what ADR-0033 removed by amendment.

The cost moves to `AddRule` (Cartesian product across all `OpIn`
fields) and to memory (one rule consumes `Π len(values_i)` bucket
entries). For typical rule sets — 1 to 3 `OpIn` fields with 2 to
10 values each — this is dozens of entries per rule, not
thousands. Pathological shapes (5 OpIn fields × 100 values each =
10⁸ entries) get rejected at `AddRule`. We pick a numeric
fan-out cap (1024 expansions per rule) to keep the contract
explicit.

### 2. What about the `OpIn` empty-set edge?

`SetCondition{Op: OpIn, Values: []}` is "field is in nothing" — a
predicate that is always false. Two responses:

- **Accept and produce zero bucket entries.** The rule is loaded
  but never fires. Clean, but hides what is almost always a bug
  in the caller's rule construction.
- **Reject at `AddRule` with `ErrNonIndexableCondition`.** Forces
  the caller to either drop the rule or fix the value list.

Pick **reject**. Consistent with the v0.8.0 stance on
`duplicate-field-in-conjunction`: shapes that are well-defined
semantically but pointless as indexed rules get rejected so the
caller hears about them at load time, not at "why isn't my rule
firing" time.

A single-value `OpIn` (`Values: ["BR"]`) is semantically identical
to `OpEq`. Accept it; the fan-out produces one bucket entry,
indistinguishable from the equivalent `StringCondition` shape.

### 3. How does wildcard work?

The parity target's CSV format has every column on every row.
A `*` in a column means "this rule does not constrain this
dimension." The loader has historically had to fan out the
wildcard rule into every concrete value the dimension can take
(Cartesian product against `knownValues`).

In `engine/indexed` we get this **for free** via the existing
key-set walking. A rule that does not mention field `X` in its
`Match` lives in a key-set that does not include `X`. At `Execute`
time, the walker visits **every registered key-set** in insertion
order; the rule's key-set gets probed independently from
key-sets that do include `X`. The rule fires whenever its own
field constraints match, regardless of the input's `X` value.

So the implementation work for "wildcard semantics" is **zero
new code**. What we add in v0.9.0:

- **A test** that registers two rules — one constraining
  `country` only, one constraining `country + tier` — and proves
  both fire correctly against inputs containing both fields.
- **A success-bar cell** that mixes wildcard and non-wildcard
  rules at 1k-rule scale, proving the multi-key-set walk does
  not regress the sub-linear claim.

Documenting this here so the property is a contract, not an
accident.

Two consequences worth calling out:

- **Loaders that originate from CSV / decision-table sources do
  the wildcard translation themselves.** The loader sees `*` in a
  column, omits that field from the resulting rule's `Match`. The
  indexed adapter never needs to know "wildcard" was the
  semantic.
- **Insertion order across key-sets still breaks ties.** A
  wildcard-on-tier rule (key-set `{country}`) registered before a
  specific-on-tier rule (key-set `{country, tier}`) wins when
  both match the same input. Same as ADR-0019, applied across
  key-sets.

## Decision

Two changes to `engine/indexed`:

### Change 1 — `SetCondition{Op: OpIn}` is admitted

`extractEqualityPairs` becomes `extractIndexablePairs` and accepts
`SetCondition{Op: OpIn, Values: [...]}` as a member of an
`AndCondition`. Each `SetCondition` contributes one entry to a
slice of `fieldValueSet`:

```go
type fieldValueSet struct {
    field  string
    values []string // canonicalized -- sorted, deduplicated
}
```

The expansion to bucket entries happens in `AddRule`:

```go
1. Walk Match, collect []fieldValueSet (one per constrained field).
2. Sort field list, canonicalize each value set (sort + dedup).
3. Validate:
   - Empty value set -> ErrNonIndexableCondition.
   - Cartesian-product cardinality > maxFanout (1024) -> ErrFanoutTooLarge.
   - Duplicate field across the conjunction -> ErrNonIndexableCondition (existing rule).
4. Compute keysetID = strings.Join(sorted-fields, US).
5. Find-or-create bucket.
6. Cartesian-product over value sets, build one valueKey per
   combination, append indexedRule to each bucket.byValueKey[key].
7. Record rule in rulesInOrder; mark name used; conditionally
   extend keysetOrder.
```

A new sentinel `ErrFanoutTooLarge` carries the rule name and the
computed cardinality so the error message is actionable.

### Change 2 — Wildcard semantics is documented and tested

No code change. New tests:

- `TestWildcardRuleFiresOnAnyValueOfOmittedField` — single rule
  constraining only `country`; multiple inputs differing only on
  `tier` all match.
- `TestKeysetWalkOrderRespectsRegistrationAcrossWidths` — rule A
  constrains 1 field, rule B constrains 2 fields including A's
  field; A registered first wins the tie when both could match.
- `TestSuccessBar_OpIn_*` — new success-bar tests asserting
  indexed-vs-firstmatch multipliers for OpIn-bearing workloads
  (see BENCHMARKS.md additions below).

### BENCHMARKS.md additions

Two new bar cells in `BENCHMARKS.md`:

| Cell | Required vs firstmatch |
|---|---:|
| 1k rules, 5 dims, **2 of 5 fields use `OpIn` with 3 values each**, `Last` | ≥ 5× faster |
| 1k rules, **mixed key-sets** (50% rules constrain 3 dims, 50% constrain 5 dims), `NoHit` | ≥ 10× faster |

The existing four cells from v0.8.0 must continue to pass —
v0.9.0 must not regress the equality-only headline. The two new
cells extend the gate to cover the v0.9.0 shapes.

### Bench-harness extension

`engine/enginetest/bench` gets a new field on `RuleSpec`:

```go
type RuleSpec struct {
    Name      string
    KeyValues map[string]string         // equality, unchanged
    InValues  map[string][]string       // NEW: OpIn shape per field
}
```

The structured workload generator (`PopulateStructured`) can now
produce mixed equality + OpIn rule shapes per workload. Existing
benchmarks continue to use the equality-only shape; new cells
specify `InValues` to exercise OpIn.

`RunStructured` is unchanged. Closure-based `Run` is unchanged.

## Consequences

### Closed by v0.9.0

- `SetCondition{Op: OpIn}` is now an indexable shape. Rule sets
  with set-membership predicates ride the sub-linear matcher
  rather than falling back to a linear adapter.
- Wildcard semantics (fields omitted from `Match`) is now
  documented and gated by a test, not an undocumented emergent
  behavior.
- The bench harness can generate mixed-shape workloads, opening
  the door to v0.10.0's negation work without re-tooling.

### Still open after v0.9.0

- `SetCondition{Op: OpNotIn}` — negation. Needs a post-filter
  pass. v0.10.0 (ADR-0035).
- `StringCondition{Op: OpNeq}` — single-value negation. Same
  shape as `OpNotIn` of one value. v0.10.0.
- Value-expression syntax (`!a`, `a|b`, `a&b`) in the parser DSL
  so string-source rules can express these shapes. v0.10.0.
- Numeric range (`>=`, `<=`). v0.11.0 (ADR-0036, via the
  IndexDimension framework).
- Concurrency safety, hot reload, telemetry — v0.12.0+.

### Memory and AddRule cost

For a typical rule that uses 1 `OpIn` field with 10 values: 10
bucket entries instead of 1. AddRule cost grows linearly with the
fan-out. For 1 000 rules at typical fan-out (×10 average), the
bucket count grows to ~10 000 entries total. Hash-map memory
overhead is dominated by the per-entry overhead Go's `map`
maintains internally; on Apple silicon with Go 1.18 this is
~50 bytes per entry. ~500 KB total bucket memory — fine.

The 1024-cap on per-rule fan-out is empirical, not load-bearing.
It catches the worst pathology (5 OpIn fields × 5 values each =
3125 expansions) and provides an actionable error. Future ADR
revises the cap if a real workload runs into it.

### What v0.9.0 does not change

- The success-bar tests from v0.8.0 still gate `ci-local`. New
  bar cells extend; they do not replace.
- The hot-path `Execute` algorithm is unchanged. Same single
  hash probe per key-set, same constant-time per candidate.
- The Rule struct shape is unchanged. The only API surface
  growth is the set of admissible `Match` shapes.
- Listener lifecycle is unchanged.

The contract is genuinely additive. A v0.8.0 caller's rule set
loads and runs identically on v0.9.0 with no code changes.

### What this validates for v0.10.0

If `OpIn` fan-out at `AddRule` works at the bar, the same
"expand at load time, single hash probe at Execute time" pattern
extends naturally to:

- `OpNotIn` — fan out to "every value except the listed ones,"
  bounded by `knownValues` per dimension. Requires either a
  caller-supplied knownValues set or a post-filter fallback.
- `OpNeq` — special case of `OpNotIn` with one value.

The decision between fan-out (preserves hot path, expensive at
load) and post-filter (cheaper load, adds hot-path cost) is
v0.10.0's question. v0.9.0's empirical fan-out cost informs
that choice with real numbers, not speculation.
