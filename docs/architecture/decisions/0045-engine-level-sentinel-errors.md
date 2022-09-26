# 45. Engine-level Sentinel Errors

## Status

Accepted — landed in v0.19.0. Closes one of the three "soft spots" ADR-0044 named (sentinel-error duplication across adapters). Adds `engine.ErrEmptyRuleName` and `engine.ErrDuplicateRuleName` at the port level. Each adapter's per-adapter sentinel keeps its name and its message but is rewritten to wrap the engine-level one. Backward-compatible: existing `errors.Is(err, inmemory.ErrEmptyRuleName)` checks keep working unchanged; new consumers can use the umbrella `errors.Is(err, engine.ErrEmptyRuleName)` to handle any adapter with a single import.

## Context

ADR-0044's stability audit named three soft spots before any contract freeze. One of them was the four-way duplication of the same sentinel across adapters:

```go
// engine/inmemory/errors.go
var ErrEmptyRuleName = errors.New("inmemory: rule name must not be empty")

// engine/firstmatch/errors.go
var ErrEmptyRuleName = errors.New("firstmatch: rule name must not be empty")

// engine/priority/errors.go
var ErrEmptyRuleName = errors.New("priority: rule name must not be empty")

// engine/indexed/errors.go
var ErrEmptyRuleName = errors.New("indexed: rule name must not be empty")
```

Same name, same semantic, four distinct `error` values. A consumer wanting `errors.Is(err, ErrEmptyRuleName)` to work across adapters has to:

1. Import all four adapter packages just for the sentinel.
2. Or chain four `errors.Is` calls.
3. Or fall back to string-matching the error message (fragile).

Same problem for `ErrDuplicateRuleName`.

The fix is the wrapping pattern: define the canonical sentinel once at the port level, have each adapter's existing variable wrap it. `errors.Is` walks the wrap chain, so both the umbrella and the per-adapter check succeed against the same returned error.

Two design questions.

### 1. Which sentinels unify?

The four adapters share three error names: `ErrEmptyRuleName`, `ErrDuplicateRuleName`, `ErrNilCondition` (linear adapters) / `ErrNilMatch` (indexed). The semantic of each:

- **`ErrEmptyRuleName`**: identical across all four adapters — "Rule.Name is empty." Unify.
- **`ErrDuplicateRuleName`**: identical across all four adapters — "a rule with this name is already registered." Unify.
- **`ErrNilCondition`** (linear) / **`ErrNilMatch`** (indexed): semantically "the rule's predicate piece is nil" but the field names differ — linear adapters have `Rule.Condition func(...)`; indexed has `Rule.Match parser.Condition`. A unified `engine.ErrNilCondition` umbrella would hide the field-name distinction. The cross-adapter ergonomics win is smaller when the underlying types and names actually differ; keep them adapter-specific.

v0.19.0 ships the two clean-unification sentinels. The Nil pair stays adapter-specific.

### 2. Wrapping or aliasing?

Three options for how the per-adapter variable relates to the engine-level one:

- **(a) Aliasing.** `var inmemory.ErrEmptyRuleName = engine.ErrEmptyRuleName`. They'd be the literally same `error` value across all four adapters. `errors.Is` is trivial.
- **(b) Wrapping.** `var inmemory.ErrEmptyRuleName = fmt.Errorf("inmemory: %w", engine.ErrEmptyRuleName)`. Distinct values, but `errors.Is` walks `%w` chains.
- **(c) Typed error.** A typed `*MissingNameError` with an `Is(target) bool` method. Most flexible, most code.

Pick **(b) wrapping**. Reasons:

- Preserves the adapter-prefixed error message (`"inmemory: rule name must not be empty"`). Existing consumers grepping logs for `"inmemory:"` keep working.
- Each adapter's `ErrEmptyRuleName` stays its own distinct value, so consumers using the per-adapter check (`errors.Is(err, inmemory.ErrEmptyRuleName)`) keep working without modification.
- Both checks succeed on the same returned error — the wrap chain makes this automatic.

Aliasing (a) would force adopting a single message for all four adapters, losing the adapter prefix. Typed error (c) is overkill for a sentinel that carries no payload beyond identity.

## Decision

Add to `engine/`:

```go
package engine

import "errors"

var (
    ErrEmptyRuleName     = errors.New("rule name must not be empty")
    ErrDuplicateRuleName = errors.New("rule name already registered")
)
```

Modify each of the four adapters' existing sentinels:

```go
// engine/inmemory/errors.go
var ErrEmptyRuleName     = fmt.Errorf("inmemory: %w", engine.ErrEmptyRuleName)
var ErrDuplicateRuleName = fmt.Errorf("inmemory: %w", engine.ErrDuplicateRuleName)

// engine/firstmatch/errors.go
var ErrEmptyRuleName     = fmt.Errorf("firstmatch: %w", engine.ErrEmptyRuleName)
var ErrDuplicateRuleName = fmt.Errorf("firstmatch: %w", engine.ErrDuplicateRuleName)

// engine/priority/errors.go
var ErrEmptyRuleName     = fmt.Errorf("priority: %w", engine.ErrEmptyRuleName)
var ErrDuplicateRuleName = fmt.Errorf("priority: %w", engine.ErrDuplicateRuleName)

// engine/indexed/errors.go
var ErrEmptyRuleName     = fmt.Errorf("indexed: %w", engine.ErrEmptyRuleName)
var ErrDuplicateRuleName = fmt.Errorf("indexed: %w", engine.ErrDuplicateRuleName)
```

Each adapter's `AddRule` keeps returning its own per-adapter sentinel as before. No behavior change inside the adapters.

### Consumer surface

```go
// New consumers can use the umbrella:
if errors.Is(err, engine.ErrEmptyRuleName) {
    // handle "rule name was empty" across any of the four adapters
}

// Pre-v0.19 consumers using the per-adapter sentinel keep working:
if errors.Is(err, inmemory.ErrEmptyRuleName) {
    // unchanged
}

// Error message strings are unchanged:
err.Error() // "inmemory: rule name must not be empty"
```

### Out of scope

- `ErrNilCondition` / `ErrNilMatch`: the field names differ between the linear adapters and the indexed adapter. Unifying would erase a real semantic distinction.
- `engine/indexed`-specific sentinels (`ErrEngineBuilt`, `ErrAlreadyBuilt`, `ErrIncompatibleInput`, `ErrNoIndexableTerms`, `ErrNonIndexableCondition`, `ErrSnapshot*`, `ErrCompiledSnapshot*`): these have no analogs in other adapters. Nothing to unify with.
- `*ActionPanicError` / `*FanoutTooLargeError`: these are typed errors with payload (the panicking rule's name), not sentinels. A separate consolidation question.

## Consequences

### Closed by this ADR

- A single `errors.Is(err, engine.ErrEmptyRuleName)` check works across all four adapters. Importing `engine` is enough; consumers no longer need to import every adapter package just to handle the error.
- Same for `ErrDuplicateRuleName`.
- One of the three soft spots ADR-0044 named is resolved. ADR-0044's classification table is updated in this commit to reflect that `engine.ErrEmptyRuleName` + `engine.ErrDuplicateRuleName` are now Tier 2 Stable port-level sentinels.

### NOT closed by this ADR

- `ErrNilCondition` / `ErrNilMatch` unification — the field names differ between adapters; a unified umbrella would hide that.
- Generic cross-adapter validation-error surface — out of scope. Each adapter still has its own additional sentinels for adapter-specific failures.

### Backward compat guarantee

- The per-adapter `ErrEmptyRuleName` and `ErrDuplicateRuleName` keep their names, their types (`error`), and their string messages. `errors.Is(err, inmemory.ErrEmptyRuleName)` returns the same answer as before.
- The error message strings are unchanged. Consumers doing log-grep on `"inmemory: rule name must not be empty"` keep working.
- No public method signature changes.

### Performance impact

`fmt.Errorf("%s: %w", ...)` creates one extra `*fmt.wrapError` per sentinel at package init. The runtime cost is one extra pointer dereference per `errors.Is` walk (negligible — sentinel checks are not on a hot path).

### Validation strategy

`engine/cross_adapter_errors_test.go` ships with v0.19.0. Three test groups:

- **`TestErrEmptyRuleNameUmbrella`**: trigger the empty-name path on each of the four adapters; assert `errors.Is(err, engine.ErrEmptyRuleName)` returns true on every result.
- **`TestErrDuplicateRuleNameUmbrella`**: same for the duplicate path.
- **`TestPerAdapterSentinelsStillWork`**: trigger each path; assert `errors.Is(err, <adapter>.ErrXxx)` still returns true on every result. Proves backward compat.
- **`TestPerAdapterMessagesUnchanged`**: assert each adapter's sentinel `.Error()` string matches what it returned before v0.19.0. Proves log-grep compat.
