# 20. RuleInfo Introspection Beyond Names

## Status

Accepted

## Context

`engine.RuleLister.RuleNames() []string` (ADR-0016) gives callers a list of registered rule names. That covered the headline introspection need -- "what's in this engine?" -- but it stops there. Real callers want more:

- **Debug endpoints** exposing rule sets via `/rules` need a human-readable description per rule. Today every adapter forces the caller to maintain a parallel `map[string]string` of descriptions keyed by rule name, which drifts the moment a rule is added without a description.
- **Rule auditing and tagging** want to group rules by responsibility ("approval-flow", "fraud-checks", "tenant-overrides") without parsing names with conventional prefixes. Tags are first-class metadata that survives name changes.
- **Documentation generators** building rule catalogs need both. No caller has a way to ask the engine for both today.

ADR-0016 explicitly forecast this gap:

> A future ADR can add a richer introspection shape (e.g., `RuleMetadata{Name, Description, Tags}`) without breaking this one -- adapters would satisfy both interfaces in parallel.

This is that future ADR.

Three design questions:

**1. Where does the metadata live?** Two options:

- (a) On each adapter's `Rule` struct (`inmemory.Rule.Description`, `firstmatch.Rule.Description`, `priority.Rule.Description`). Adds two fields per adapter; the rule-defining call site declares the metadata next to the condition and action.
- (b) On a separate registration call (`engine.AddDescription(ruleName, description)`). Decouples metadata from rules but introduces a sync-or-drift problem (a rule can be registered without its description, or with a description after the fact).

Pick (a). The metadata belongs *with* the rule it describes. Callers who don't set the fields get empty defaults; nothing in the engine forces metadata where it's not wanted.

**2. What's the introspection interface shape?**

```go
type RuleInfo struct {
    Name        string
    Description string
    Tags        []string
}

type RuleInfoLister interface {
    RuleInfos() []RuleInfo
}
```

`RuleInfo` is a port-level value type in the `engine` package, paralleling `Request` and `Result`. Tags is `[]string`, not `map[string]string` or a typed enum -- tag values are caller-defined strings, and they need to round-trip through JSON cleanly for debug endpoints.

**3. Does it supersede `RuleLister`?** No. `RuleLister.RuleNames()` stays as the cheap-and-cheerful "just the names" path; `RuleInfoLister.RuleInfos()` is the richer one. Adapters satisfy both (today's three do); callers pick the one matching their need. The two interfaces are parallel optional capabilities, not a v1/v2 of the same one.

## Decision

Add a port-level value type and an optional capability interface to the `engine` package:

```go
// engine/rule_info.go
type RuleInfo struct {
    Name        string
    Description string
    Tags        []string
}

type RuleInfoLister interface {
    RuleInfos() []RuleInfo
}
```

Each adapter's `Rule` struct grows two optional fields next to `Name`:

```go
Description string
Tags        []string
```

`AddRule` ignores them for validation -- only `Name`, `Condition`, and the duplicate check apply. The fields default to empty (no validation tax on callers who don't set them).

Each adapter implements `RuleInfos() []RuleInfo` by walking its stored rules and mapping `Description`/`Tags` into the port type. The returned slice is a fresh copy with insertion order (same contract as `RuleNames()`). A nil `Tags` field on the source `Rule` becomes a nil `Tags` in the `RuleInfo` -- no defensive empty-slice allocation.

`enginetest.RunContractTests` gains an eleventh case: if the adapter satisfies `RuleInfoLister`, seeding a rule with a known name should make `RuleInfos()` return a slice containing a `RuleInfo` with that name. Adapters without `RuleInfoLister` auto-skip the case, same pattern as ADR-0010, ADR-0016, ADR-0017.

The contract case stops short of asserting description / tag round-trip because the existing `SeedFunc` signature (`name`, `condition`, `action`) does not carry those fields. Adapter-specific tests cover the round-trip; the contract case covers the capability.

## Consequences

The port grows by one value type and one interface; the engine package now has three optional capability interfaces (`ListenerHost`, `RuleLister`, `RuleInfoLister`) and a clear precedent that capabilities accumulate as parallel optional shapes, not as fattening of any one interface.

Each adapter's `Rule` struct gains two fields. This is a non-breaking additive change -- existing callers that construct `inmemory.Rule{Name: x, Condition: y}` continue to compile; the new fields default to empty.

`RuleNames()` becomes the lightweight introspection path (just the names, cheap); `RuleInfos()` becomes the richer one. Both return fresh slices, both walk insertion order, both are O(N). Callers who want only names use `RuleNames()` to avoid the per-rule struct allocation in `RuleInfos()`.

A future "richer-still" introspection (`Rule.Author`, `Rule.LastModified`, `Rule.Source`) would either land as `RuleInfo` field additions if they're universal, or as a separate `RuleAuditLister` if they're audit-specific. The "many small interfaces over one fat one" principle from ADR-0007 continues to govern.
