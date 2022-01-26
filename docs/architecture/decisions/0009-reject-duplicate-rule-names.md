# 9. Reject Duplicate Rule Names On AddRule

## Status

Accepted

## Context

`inmemory.Engine.AddRule` validates that `Rule.Name` is non-empty (returns `ErrEmptyRuleName`) but does not check for duplicates. Two rules with the same name are accepted, both run on a match, and both push the same name onto `Result.Matched`. The downstream effect is that:

- `CountingListener.Count("alpha")` returns 2 for a single semantic match, which over-counts.
- `Result.Matched` contains the same name twice, which any caller reading "did rule X fire?" has to deduplicate themselves.
- Replacing a rule by re-adding it under the same name silently *adds* a second rule instead of swapping the first.

Rule names are the identifier callers reason about, so the engine must guarantee they are unique. Rejecting duplicates at registration time -- rather than detecting them later or silently accepting them -- surfaces a typo or accidental re-registration immediately, where the caller can fix it.

There are two reasonable shapes for the rejection:

1. **Hard reject**: `AddRule` returns a new sentinel `ErrDuplicateRuleName`. Callers handle the error or panic.
2. **Soft replace**: re-adding a rule with an existing name overwrites the first.

We pick (1). Silent replacement is the kind of "convenient" behavior that hides bugs (a typo in a rule name elsewhere can make a rule disappear without warning). The cost of the explicit error is one extra branch at the caller, which is exactly the right cost.

## Decision

Add `ErrDuplicateRuleName` next to `ErrEmptyRuleName` in `engine/inmemory/errors.go`. In `AddRule`, after the empty-name check, walk the existing `rules` slice and return `ErrDuplicateRuleName` if any existing rule shares the name. The walk is O(N); for a rule set big enough to make that slow, an index map is a follow-up optimization, not a correctness concern today.

The error wraps the offending name through `%w`-style formatting only if we add an `errors.Is`-friendly wrapper later. For now, a bare sentinel keeps parity with `ErrEmptyRuleName`.

## Consequences

`Result.Matched` is now guaranteed to have unique entries per execution (modulo a rule firing once -- it cannot share a name with another fired rule, because both could not have been registered). `CountingListener` counts semantic matches, not duplicate registrations.

The contract test suite gains a case asserting that an adapter rejects a duplicate name. Any future adapter (gorules, file-loaded, etc.) is held to the same promise.

Callers who *want* "replace this rule" need to do it explicitly. We do not ship a `ReplaceRule` method today -- the use case has not appeared. If it does, the API addition is small.
