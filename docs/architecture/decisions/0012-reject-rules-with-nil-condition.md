# 12. Reject Rules With A Nil Condition

## Status

Accepted

## Context

`inmemory.Engine.AddRule` validates the rule name (non-empty, unique) but does nothing about the `Condition` field. A rule registered with `nil` `Condition` is accepted and silently never matches:

```go
e.AddRule(inmemory.Rule{Name: "alpha", Action: actionFn}) // accepted
// during Execute: Condition == nil branch is skipped
```

The caller's intent is almost certainly "this rule should fire whenever something else triggers it" or, more likely, a typo where they forgot to set `Condition`. Either way the engine is hiding the bug.

This is exactly the same shape as the duplicate-name case ADR-0009 closed: a registration-time gap that converts a programming error into a silent behavioral one. Same fix shape: reject at `AddRule`, return a new sentinel.

The alternative -- treat `nil` `Condition` as "always fires" -- is *also* a real BRE pattern (a "fact" or "default action" that runs unconditionally). We do not take that path today because:

1. It overloads the same field with two meanings (`nil` and "true-returning func"), which is the kind of cute that bites readers later.
2. If we ever want unconditional rules, an explicit boolean flag (`Always bool` or similar) or a separate `AddAlwaysRule` method is unambiguous and a small additive API change.

So the conservative move is: reject `nil` `Condition` at registration. If the unconditional-rule use case appears in real callers, a follow-up ADR adds it with an explicit shape.

## Decision

Add `ErrNilCondition` next to `ErrEmptyRuleName` and `ErrDuplicateRuleName` in `engine/inmemory/errors.go`. `AddRule` runs the checks in **shape-first, state-second** order:

1. `Name == ""` → `ErrEmptyRuleName`
2. `Condition == nil` → `ErrNilCondition`
3. duplicate name in existing rules → `ErrDuplicateRuleName`

Shape-first means: invariants the *rule alone* must satisfy (a name, a condition) are checked before invariants that depend on the *engine state* (uniqueness). The visible benefit is that error returns are deterministic regardless of registration order. Calling `AddRule` twice with the same malformed rule returns the same error both times, instead of the first call complaining about shape and the second about a duplicate of an in-fact-never-stored rule.

The `engine.Engine` port is unchanged -- this is purely an `inmemory` adapter invariant.

The contract test suite stays silent on this. Other adapters (gorules, file-loaded) might represent "always fires" natively and not need a `Condition` callback at all, so the suite cannot assume a `nil` callback is rejection-worthy.

The pre-existing branch `if r.Condition == nil || !r.Condition(req.Input) { continue }` in `Execute` becomes dead but is left in place: defense in depth costs one branch per rule per call, and a future code path (rule loading from a file format) could in principle reintroduce nil-Condition rules.

## Consequences

`inmemory.Engine` now rejects three classes of malformed rule at registration time: empty name, duplicate name, nil condition. All three return distinct sentinels callers can branch on with `errors.Is`.

The `nil`-Condition test cases in `inmemory_test.go` (specifically `TestExecuteSkipsRulesWithNilCondition`) need to be reworked: that path is now impossible to reach through the public API. We replace the test with one that asserts `AddRule` rejects the rule, preserving the *intent* (nil conditions must not silently fire) without weakening the suite.
