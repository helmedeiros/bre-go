# 27. The Expression Parser Package

## Status

Proposed — target release `v0.5.0`. Transitions to **Accepted** in the commit that lands the implementation. Opens Phase 3 (Expression DSL).

## Context

Today every `Rule.Condition` is a Go closure — written in code, compiled into the binary, deployed with the application. That's fine for in-code rules but blocks the use case the scientific test at the end of Phase 2 flagged: **rule conditions expressed as strings inside CSV / JSON files**.

A condition like "origin is Germany and platform is web" lives in code today as:

```go
Condition: conditions.And(
    func(in interface{}) bool { return in.(Request).Origin == "DE" },
    func(in interface{}) bool { return in.(Request).Platform == "web" },
),
```

What we want: that same condition written *as a string* in a CSV column, parsed at load time into a predicate, and dropped into `Rule.Condition`. Adding a new rule then means editing a row, not recompiling.

This is the gap Phase 3 closes. It comes in two steps:

- **v0.5.0 (this ADR)**: the parser machinery — turn an expression string into an evaluable form.
- **v0.6.0 (ADR-0029, planned)**: declarative rules loadable from JSON / CSV where the condition is a parsed string, plugging into the loader from Phase 2.

Three design questions for v0.5.0:

**1. What's the input shape of an evaluated expression?**

`Rule.Condition` is `func(input interface{}) bool`. The parser could produce predicates of the same shape. But that means each parsed predicate needs to know how to extract fields from `input` — which is `interface{}`. Three approaches:

- (a) **Reflection over the struct.** Magical, slow, brittle when struct fields rename.
- (b) **Map-based evaluation.** Parser produces `func(map[string]interface{}) bool`. Caller converts their struct → map before calling. Explicit and fast.
- (c) **Fact interface.** Parser produces `func(Fact) bool` where `Fact.Field(name) (value interface{}, ok bool)`. Caller writes a `Fact` adapter once for their struct.

Pick **(b) for v0.5.0** — simplest, no new interface needed, lets us ship the parser without bikeshedding the Fact abstraction. Callers writing `toFactMap(req)` once is a small cost. The Fact-interface variant earns its place when a real caller asks (likely once we have non-trivial struct types with nested fields).

**2. What grammar?**

Minimum viable for the decision-table use case:

```
expression  ::= or_expr
or_expr     ::= and_expr ( "OR" and_expr )*
and_expr    ::= not_expr ( "AND" not_expr )*
not_expr    ::= "NOT" atom | atom
atom        ::= "(" expression ")" | comparison
comparison  ::= field op value
field       ::= IDENT
op          ::= "==" | "!=" | "IN" | "NOT IN"
value       ::= STRING | "(" STRING ("," STRING)* ")"
```

Operators (in increasing precedence): `OR`, `AND`, `NOT`, `==` / `!=` / `IN` / `NOT IN`. Parens override.

String literals only. No integers, booleans, floats. Real callers encode booleans as `"true"` / `"false"` strings in their CSV today; the parser doesn't need to know they're "really" booleans. Numeric comparisons (`>`, `<`, `>=`, `<=`) wait for a follow-up ADR.

Whitespace-insensitive between tokens. Identifiers match `[A-Za-z_][A-Za-z0-9_]*`. Strings are double-quoted with backslash escaping for `\"` and `\\`.

**3. What's the public API?**

```go
package parser

// Predicate is the evaluated form of a parsed expression.
type Predicate func(fact map[string]interface{}) bool

// Parse compiles the expression string into a Predicate. Returns
// a *ParseError on syntax failures.
func Parse(expr string) (Predicate, error)

// ParseError carries the position of the syntax failure.
type ParseError struct {
    Pos     int    // 0-indexed byte offset into expr
    Message string
}
func (e *ParseError) Error() string { ... }
```

One exported function (`Parse`) plus one exported type (`ParseError`). The `Predicate` type alias makes function signatures readable. No `Parser` interface — there's exactly one implementation; introducing the interface premature.

A small helper (`AsCondition`) bridges parser output to `Rule.Condition`:

```go
// AsCondition wraps a Predicate as a func(interface{}) bool by
// applying factOf to the engine input before evaluating.
func AsCondition(p Predicate, factOf func(interface{}) map[string]interface{}) func(interface{}) bool
```

Caller writes `factOf` once for their struct type:

```go
func reqAsFact(in interface{}) map[string]interface{} {
    r := in.(Request)
    return map[string]interface{}{"origin": r.Origin, "platform": r.Platform}
}

pred, _ := parser.Parse(`origin == "DE" AND platform == "web"`)
rule := inmemory.Rule{
    Condition: parser.AsCondition(pred, reqAsFact),
    ...
}
```

## Decision

Add `engine/parser` as a new sub-package. Public surface:

```go
type Predicate func(fact map[string]interface{}) bool
type ParseError struct { Pos int; Message string }
func (e *ParseError) Error() string

func Parse(expr string) (Predicate, error)
func AsCondition(p Predicate, factOf func(interface{}) map[string]interface{}) func(interface{}) bool
```

Implementation is a hand-written recursive-descent parser. No third-party dependencies; targets ~200 lines plus tests. The lexer is a simple position-tracking byte scanner; the parser builds an AST and immediately compiles it to a closure (no intermediate AST exposed publicly — we can introduce that later if a need appears).

Test coverage targets 100% per the project's standing rule. Test categories:

- Tokenization (identifiers, strings with escapes, operators, parens, whitespace).
- Each grammar production (atoms, comparisons, AND, OR, NOT, parens, precedence).
- Each error case (unclosed quote, unknown operator, missing parenthesis, trailing garbage) with the `*ParseError.Pos` pointing at the right byte.
- Predicate evaluation (correct truth tables across all four comparison operators; short-circuit in AND/OR; missing fact key returns false for equality and false for IN).
- The `AsCondition` bridge in a real `inmemory.Engine` round-trip.

## Consequences

The library can now express rule conditions as strings. Combined with the v0.3.0 / v0.4.0 loader infrastructure, the standard wiring becomes:

```go
// CSV row: name, priority, condition_expr, percent
//   "vip-de", 100, "origin == \"DE\" AND tier IN (\"vip\")", 20

type RuleRow struct { Name string; Priority int; CondExpr string; Percent int }

func parseRow(cols []string) (RuleRow, error) { ... }
func toFact(in interface{}) map[string]interface{} { ... }

loader := csv.NewLoader[RuleRow](path, parseRow)
err := engine.Load(loader, func(r RuleRow) error {
    pred, err := parser.Parse(r.CondExpr)
    if err != nil {
        return fmt.Errorf("rule %q: %w", r.Name, err)
    }
    return eng.AddRule(priority.Rule{
        Name:      r.Name,
        Priority:  r.Priority,
        Condition: parser.AsCondition(pred, toFact),
        Action:    func(interface{}) interface{} { return r.Percent },
    })
})
```

Adding a new rule means adding a CSV row. No recompile.

What's deliberately deferred:

- **Numeric comparisons** (`>`, `<`, `>=`, `<=`). Real callers using these write conditions like `amount > 100`. ADR-0028 covers them when a caller asks.
- **Typed conditions** (`StringCondition`, `SetCondition` as separate types). The parser internally creates them as closures; surfacing them as exported types waits for v0.6.0.
- **Reflection-based or `Fact`-interface-based evaluation**. Map-based evaluation is the simplest thing that works; richer fact abstractions land if a caller's struct shape outgrows the map.
- **AST exposure**. The parser compiles directly to a closure. If a future need (rule rewriting, optimization, visualization) demands the AST, exposing it is additive.

The README Stability section adds the new types; the cookbook gains a "Write rule conditions as strings" section.
