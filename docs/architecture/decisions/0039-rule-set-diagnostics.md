# 39. Rule-Set Diagnostics on `engine/indexed`

## Status

Proposed — target v0.14.0. Opportunistic improvement during the
Phase-5 calendar gap (GoRules library doesn't ship until mid-2023).
Adds an `Engine.Diagnose()` method that reports rules which cannot
fire because an earlier rule shadows them. Not a parity gap; the
parity target doesn't ship this either. It's a defensive feature
for any team operating a non-trivial rule set, surfaced by
implementation experience with v0.8.0–v0.13.0.

## Context

`engine/indexed` uses first-match semantics with insertion-order
tie-break across key-sets. That makes the *order* of rule
registration a load-bearing decision. The current API gives no
help to operators who want to verify:

- "Is rule X actually reachable, or does some earlier rule always
  fire first?"
- "If I add a new rule, does it shadow any existing rule (silent
  behavior change)?"
- "Is the rule set internally consistent, or are there latent dead
  rules accumulated from incremental edits?"

Today the only way to answer these is to write a test that exhaustively
generates inputs and observes which rules fire. That's brittle and
expensive.

The structured rule shape (`parser.Condition` AST) gives us
everything we need to answer these statically. v0.14.0 ships the
analyzer.

Three design questions.

### 1. What does the analyzer report?

Three candidate signals:

- **Dead rules.** A rule R is dead iff every input that satisfies
  R's match also satisfies some earlier rule's match (and that
  earlier rule has no post-filter to reject the input). The earlier
  rule fires first; R never executes. Computable from the typed
  Match.
- **Overlap.** Two rules overlap iff some input satisfies both
  matches. For first-match adapters this is informational, not
  necessarily a bug — overlap is how layered rule sets work
  (specific rules registered before general fallbacks). Surfacing
  every overlap would be noisy.
- **Coverage.** Which inputs are guaranteed to match at least one
  rule. Useful when callers expect a catch-all default; less useful
  when they expect some inputs to legitimately not match.

Pick **dead rules only** for v0.14.0. Dead rules are unambiguously
a bug — the operator wrote a rule that never fires. Overlap and
coverage are more domain-specific judgment calls; defer until a
real consumer asks. The framework added in v0.14.0 supports adding
more signals as fields on `DiagnoseReport` without breaking
callers.

### 2. How conservative is the dead-rule check?

The general subsumption problem ("does rule A's match imply rule
B's match?") is decidable for our shape language but gets messy
once post-filters enter the picture. Two-tier approach:

**Tier 1 (decidable, ship this):** Earlier rule has NO post-filter.
Then the earlier rule's match shape (indexable terms only) fully
determines whether it fires. If every field the earlier rule
constrains is also constrained by the later rule, with the later's
admitted values ⊆ the earlier's admitted values, the later is
dead.

**Tier 2 (semi-decidable, defer):** Earlier rule HAS post-filter.
The post-filter could reject inputs the bucket admitted, so the
earlier rule doesn't necessarily fire. Subsumption reasoning over
post-filters is shape-specific (OpNeq subset, RangeCondition
interval comparison, custom-hook conditions are opaque). A future
ADR widens the analyzer if real consumers ask.

For v0.14.0 we skip the earlier-rule-has-post-filter case entirely:
no shadowing report. That's the conservative call. Reports are
true positives only; we never falsely accuse a rule of being dead.
The cost is false negatives (we miss some dead rules), which is
the right side of the tradeoff for a diagnostic tool — false
positives erode operator trust faster than missed warnings.

### 3. Method signature

Three shapes:

- **(a) `Diagnose() DiagnoseReport`** — value return, no error path.
- **(b) `Diagnose() (DiagnoseReport, error)`** — error path for
  analyzer-internal failures.
- **(c) `Diagnose(opt ...DiagnoseOption) DiagnoseReport`** — option
  pattern for future "include overlap," "include coverage" toggles.

Pick **(a)**. The analyzer is pure: walks the engine's existing
state, allocates a slice, returns. No I/O, no panic paths, no
caller-tunable behavior in v0.14.0. Future signals get added as
fields on `DiagnoseReport`; future tuning gets the option pattern
*if* real consumers ask. Premature option-pattern construction is
the standard YAGNI trap.

Diagnose works in both engine phases (pre-Build via builder state,
post-Build via snapshot), same as `RuleNames` / `RuleInfos`. Does
not trigger implicit Build — Diagnose is a meta-query, not a
runtime invocation.

## Decision

Add to `engine/indexed`:

```go
// DiagnoseReport is the result of Engine.Diagnose. v0.14.0 ships
// the DeadRules field only; future ADRs may add overlap or
// coverage fields.
type DiagnoseReport struct {
    DeadRules []DeadRule
}

// DeadRule names a rule that can never fire because an earlier
// rule in walking order shadows it. Reported by Engine.Diagnose
// when the shadowing relation is decidable (the earlier rule has
// no post-filter terms).
type DeadRule struct {
    Name       string // the dead rule's name
    ShadowedBy string // the earlier rule's name
    Reason     string // human-readable explanation
}

// Diagnose analyzes the engine's rule set and returns a report
// of any dead rules. A rule is dead iff every input that satisfies
// its match also satisfies some earlier rule's match -- and that
// earlier rule has no post-filter terms that could reject the
// input. The check is conservative: we report only true dead
// rules. Earlier rules with post-filter terms are skipped (we
// can't statically determine they would fire).
//
// Safe to call in either phase (pre-Build via builder state,
// post-Build via snapshot). Does not trigger implicit Build.
//
// Complexity: O(N^2 * F) where N is the rule count and F is the
// number of constrained fields per rule. Linear over rule pairs
// times field comparisons; a 1000-rule engine with 5 fields is
// ~5M comparisons, single-millisecond on modern hardware. Not
// suitable for per-request invocation; call from startup
// validation or admin endpoints.
func (e *Engine) Diagnose() DiagnoseReport
```

### Algorithm

1. Snapshot the current rule list (works in either phase via the
   existing `rulesView` helper).
2. For each rule, extract its match shape: the set of constrained
   fields with their admitted values, and whether it carries any
   non-indexable (post-filter) terms.
3. For each pair `(earlier, later)` where `earlier` has a lower
   insertion index:
   - Skip if `earlier.hasPostFilter` — can't decide shadowing.
   - Skip if `earlier.fields` is not a subset of `later.fields` —
     `earlier` constrains a field `later` doesn't, so `later`
     admits values for that field that `earlier` rejects; `later`
     is not dead.
   - For each field `f` in `earlier.fields`:
     - `later.values[f]` must be a subset of `earlier.values[f]`.
   - If all checks pass, `later` is dead. Record (later.Name,
     earlier.Name). Continue to the next `later` (we report only
     the first shadower per dead rule; multiple shadowers per rule
     would be noise).

### Reason string

Each `DeadRule.Reason` follows a fixed template the test suite can
assert against without brittleness:

```
"every input matching this rule also matches an earlier, less-constrained rule"
```

Future signals (overlap, coverage) add their own reason templates.

### Tests

Standard battery in `engine/indexed/diagnose_test.go`:

- Empty engine → empty report.
- Single rule → empty report.
- Two identical rules → second is dead, shadowed by first.
- Narrower rule registered after broader → dead.
- Broader rule after narrower → NOT dead (broader admits inputs
  narrower rejects).
- Disjoint rules (different fields or different values) → NOT dead.
- OpIn shadows OpEq (`country IN [BR, AR]` then `country = BR`) → BR
  rule dead.
- OpEq registered before OpIn → BR rule fires first → IN-rule
  not dead unless its values are exactly {BR}.
- Earlier rule has post-filter → no shadow report (conservative
  tier-1 limit).
- Later rule has post-filter, earlier doesn't → still dead (the
  earlier rule fires before the later's post-filter runs).
- Multi-field rules with partial overlap → exhaustive truth-table
  coverage.

### Cookbook section

A new "Validate your rule set with Diagnose" entry:

- Show the canonical call (`report := e.Diagnose(); if len(report.DeadRules) > 0 { ... }`).
- Recommend wiring it into startup validation: "fail-fast on dead
  rules before serving traffic."
- Note the conservative tier-1 limit: a clean Diagnose doesn't
  *prove* no dead rules exist; it proves no dead rules exist where
  the shadower has no post-filter. Operationally, this catches the
  90% case (most shadowing is between equality-only rules).

## Consequences

### Closed by v0.14.0

- Operators get a way to detect dead rules statically before they
  ship a new rule set. Especially valuable when rules are loaded
  from external config and the engine team isn't reviewing every
  change.
- The `engine/indexed.DiagnoseReport` shape sets a precedent for
  future static analyses; new fields land additively.

### Still open after v0.14.0

- **Overlap and coverage analysis.** Same framework; new
  `DiagnoseReport` fields when a consumer asks.
- **Diagnose with post-filter reasoning.** Tier-2 algorithm above.
  Adds OpNeq subset analysis, RangeCondition interval comparison.
  Out of v0.14.0 scope; future ADR with worked examples from real
  consumers driving the spec.
- **Diagnose on linear adapters.** The opaque closure shape on
  `inmemory` / `firstmatch` / `priority` can't be introspected
  statically. A future ADR could add an optional "shape-aware"
  rule constructor that preserves the typed match for analysis.
  Not a v0.14.0 concern.

### Performance impact

- Diagnose is O(N² × F). For a 1k-rule engine with 5 fields:
  ~5 million comparisons, single-millisecond on modern hardware.
- No allocations beyond the report itself + the per-rule shape
  cache (one `ruleShape` per rule, freed after Diagnose returns).
- Not safe for per-request invocation; document as
  "startup-validation or admin-endpoint usage."
- Per-Execute hot path is unchanged — Diagnose doesn't touch the
  snapshot's bucket structures, just reads `rulesInOrder`.

### Validation strategy

- Unit tests covering the dead-rule decision table (above).
- A pre-tag external scientific test verifies Diagnose against a
  10-rule fixture with known dead and live rules; the report's
  shape must match expectations exactly.
- Existing success-bar tests unchanged — Diagnose doesn't touch
  Execute.

### What this validates for v0.15.0+

The `DiagnoseReport` shape is the first "static analysis" surface
in bre-go. v0.15.0+ ADRs that add overlap, coverage, or post-filter
reasoning extend the same report. The struct field ordering is the
forward-compat contract: new fields land at the end; readers using
struct literals with field names continue to compile.

If a future ADR adds analysis that's expensive enough to want
caller control (e.g., overlap analysis at O(N²) on a 10k-rule
engine), that's the moment to introduce the option pattern. v0.14.0
ships without it because the dead-rule analyzer is cheap enough to
always run.
