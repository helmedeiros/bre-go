# 28. Typed Condition Tree

## Status

Accepted — landed as the new exported types in `engine/parser`. Ships with v0.6.0. Closes Phase 3 (Expression DSL).

## Context

ADR-0027 shipped `engine/parser.Parse` returning a `Predicate` -- a `func(map[string]interface{}) bool` closure. That works for evaluation but is opaque to anything that wants to *inspect* the parsed expression:

1. **JSON serialization.** A closure cannot be marshalled. A typed tree of structs can.
2. **Introspection.** "What fields does this rule reference?" requires walking the expression tree. A closure hides the structure.
3. **Optimization.** Phase 4's indexed matcher needs to extract bucket keys from conditions -- "this rule's `tier IN ('vip', 'premium')` becomes bucket `tier=vip` and bucket `tier=premium`." Closures expose nothing.
4. **Equality / dedup.** Two rule sets containing the same condition expression can be deduped if conditions are values, not closures.

ADR-0027 deliberately deferred this:

> **Typed conditions** (`StringCondition`, `SetCondition` as separate types). The parser internally creates them as closures; surfacing them as exported types waits for v0.6.0.

This is that follow-up.

Three design questions:

**1. Where do the types live?**

- (a) New `engine/cond` (or `engine/condition`) package.
- (b) In `engine/parser` alongside `Parse`.
- (c) In the existing `engine/conditions` package (alongside the `And`/`Or`/`Not` combinator functions).

Pick **(b)**. The Condition tree is the parser's output; putting it in the same package keeps the import surface tight. Option (c) is rejected because `engine/conditions` already houses combinator *functions* that return closures -- mixing in typed *structs* with overlapping names (`AndCondition` vs `And`) would confuse readers.

**2. What's the interface shape?**

```go
type Condition interface {
    Eval(fact map[string]interface{}) bool
}
```

Single method. Same fact-map input as `Predicate` so callers can swap a Condition into any code path that took a Predicate before. Concrete types:

```go
type StringCondition struct {
    Field string
    Op    string  // "==" or "!="
    Value string
}

type SetCondition struct {
    Field string
    Op    string  // "IN" or "NOT IN"
    Values []string
}

type AndCondition struct { Children []Condition }
type OrCondition  struct { Children []Condition }
type NotCondition struct { Child Condition }
```

Op as a `string` rather than a custom typed enum: simpler, JSON-friendly, no import needed to compare. Constants (`OpEq = "=="`) are exported for callers who prefer named references.

**3. How does the parser surface this?**

Two new exported functions in `engine/parser`:

```go
// ParseToCondition compiles expr into a typed Condition tree.
func ParseToCondition(expr string) (Condition, error)

// AsPredicate wraps a Condition as a Predicate (same shape as Parse returns).
func AsPredicate(c Condition) Predicate
```

The existing `Parse` continues to work unchanged -- internally it now calls `ParseToCondition` and then `AsPredicate`, but the public signature is preserved. Callers using `Parse` see no change.

`AsCondition` (the existing `func(interface{}) bool` bridge) stays as-is. A new convenience `ConditionAsRuleCondition` wires a typed Condition to `Rule.Condition` shape:

```go
func ConditionAsRuleCondition(c Condition, factOf func(interface{}) map[string]interface{}) func(interface{}) bool {
    return func(in interface{}) bool { return c.Eval(factOf(in)) }
}
```

(Names worth bikeshedding -- the final ADR can settle on shorter names once the implementation is in flight.)

## Decision

Add to `engine/parser`:

```go
// Condition is a Boolean predicate over a fact map. Concrete
// implementations (StringCondition, SetCondition, And/Or/NotCondition)
// can be inspected, marshalled, and compared.
type Condition interface {
    Eval(fact map[string]interface{}) bool
}

const (
    OpEq    = "=="
    OpNeq   = "!="
    OpIn    = "IN"
    OpNotIn = "NOT IN"
)

type StringCondition struct {
    Field string
    Op    string
    Value string
}

type SetCondition struct {
    Field  string
    Op     string
    Values []string
}

type AndCondition struct{ Children []Condition }
type OrCondition  struct{ Children []Condition }
type NotCondition struct{ Child Condition }

func ParseToCondition(expr string) (Condition, error)
func AsPredicate(c Condition) Predicate
```

Each concrete type implements `Eval(fact) bool` with the same semantics as the closures it replaces. `AndCondition.Eval` short-circuits on the first false child; `OrCondition.Eval` short-circuits on the first true; both return the identity (true for And-empty, false for Or-empty) when their child list is empty, matching the existing parser behavior and `conditions.And`/`conditions.Or` identity elements.

The internal parser is refactored to build the Condition tree directly. `Parse(expr)` becomes `ParseToCondition(expr)` then `AsPredicate`. Coverage stays at 100% on `engine/parser`; existing tests remain valid because they test the public `Parse` -> `Predicate` -> evaluation path.

Adapter `Rule.Condition` fields still take `func(interface{}) bool`. A helper bridges typed Conditions:

```go
// AsRuleCondition wraps c with the caller's factOf converter for use
// in any adapter's Rule.Condition field.
func AsRuleCondition(c Condition, factOf func(interface{}) map[string]interface{}) func(interface{}) bool
```

This is `parser.AsCondition` (which takes a `Predicate`) for the typed-tree case.

## Consequences

`engine/parser` exports grow from 4 to 12 symbols. None of the existing identifiers change. Callers using `Parse` continue to work unchanged.

JSON marshalling becomes possible for any Condition tree the parser produces, unlocking the JSON loader work that follows in v0.7.0 (a separate ADR will specify the JSON shape). The encoding/decoding work itself lives in `engine/json` -- not in `engine/parser`. Keeping parser dependency-free preserves its zero-third-party-imports property.

Phase 4's indexed matcher gains a clean dependency surface: it can take a `Condition` tree and extract bucket dimensions by pattern-matching on `StringCondition` and `SetCondition` nodes. That work was previously blocked by the closure-based representation; this ADR unblocks it.

What's deliberately deferred:

- **Field-naming convention enforcement.** The parser accepts any identifier as a field name. Whether the field exists in the fact map is checked at `Eval` time (returns false). A future ADR could add compile-time field-name validation if callers want stricter rule loading.
- **Numeric, boolean, and float comparisons.** Still strings only. Adding `>` / `<` / `>=` / `<=` waits for a real caller asking; the typed Condition tree makes adding them easier (a new `NumericCondition` type) but the grammar work alone is non-trivial.
- **AST visitor pattern.** The Condition interface has only `Eval`. Future visitors (for printing, transformation, optimization) can be added as free functions matching on concrete types, or as a future `Visit(Visitor)` method on the interface.
