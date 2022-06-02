# Cookbook

Realistic patterns for using `bre-go`. The [Quickstart in the README](../README.md#quickstart) covers the smallest happy-path example; this document is for everything past that.

All examples target `v0.2.0` or later. `context.Context` is the first parameter of every `Execute` call.

## Contents

- [Patterns](#patterns)
  - [Pick the right adapter](#pick-the-right-adapter)
  - [Compose conditions declaratively](#compose-conditions-declaratively)
  - [Handle cancellation and timeouts](#handle-cancellation-and-timeouts)
  - [Compose multiple listeners on one engine](#compose-multiple-listeners-on-one-engine)
  - [Introspect at runtime](#introspect-at-runtime)
  - [Handle errors from Execute](#handle-errors-from-execute)
  - [Use the typed Executor](#use-the-typed-executor)
  - [Write adapter-agnostic helpers](#write-adapter-agnostic-helpers)
  - [Load rules from a CSV file](#load-rules-from-a-csv-file)
  - [Load rules from a JSON file](#load-rules-from-a-json-file)
  - [Compose rules from multiple sources](#compose-rules-from-multiple-sources)
  - [Propagate correlation IDs](#propagate-correlation-ids)
  - [Write rule conditions as strings](#write-rule-conditions-as-strings)
  - [Inspect parsed conditions as typed trees](#inspect-parsed-conditions-as-typed-trees)

## Patterns

### Pick the right adapter

The library ships three adapters with different semantics. Match the adapter to the policy your rule set encodes.

| Policy | Adapter |
|--------|---------|
| Evaluate every rule, every matching action runs, last action wins on `Output` | `engine/inmemory` |
| Walk in insertion order, return on the first matching rule | `engine/firstmatch` |
| Walk in descending `Priority` order, return on the first matching rule | `engine/priority` |

A useful starting heuristic: if your rule set comes from a config file where one row should always win for a given input, you want `firstmatch` or `priority` (decision-table semantics). If you're computing per-rule effects that *accumulate* (counters, audit trails), you want `inmemory`.

```go
// firstmatch: positional precedence -- order of AddRule wins
e := firstmatch.New()
_ = e.AddRule(firstmatch.Rule{Name: "premium", Condition: isPremium, Action: chargePremium})
_ = e.AddRule(firstmatch.Rule{Name: "standard", Condition: isStandard, Action: chargeStandard})
_ = e.AddRule(firstmatch.Rule{Name: "default", Condition: conditions.Always(), Action: chargeDefault})

// priority: explicit precedence in the data
p := priority.New()
_ = p.AddRule(priority.Rule{Name: "blocklist", Priority: 1000, Condition: onBlocklist, Action: deny})
_ = p.AddRule(priority.Rule{Name: "vip",       Priority: 100,  Condition: isVIP,        Action: approve})
_ = p.AddRule(priority.Rule{Name: "default",   Priority: 0,    Condition: conditions.Always(), Action: standard})
```

Both expressions of the same logic. Pick the one where the precedence lives where it belongs: in the call site (`firstmatch`) or in the data (`priority`).

### Compose conditions declaratively

`engine/conditions` lets you build predicates as trees instead of one fat closure. The same combinators (`And`, `Or`, `Not`, `Always`, `Never`) drop into any adapter's `Rule.Condition` field.

```go
import "github.com/helmedeiros/bre-go/engine/conditions"

approveCondition := conditions.And(
    conditions.Or(
        amountGreaterThan(100),
        currencyEquals("USD"),
    ),
    conditions.Not(flagged),
    conditions.Not(onBlocklist),
)

_ = e.AddRule(inmemory.Rule{
    Name:      "approve",
    Condition: approveCondition,
    Action:    approve,
})
```

Why this matters: each leaf predicate (`amountGreaterThan`, `currencyEquals`, `flagged`, `onBlocklist`) is independently testable, independently reusable across rules, and named. The composition itself reads in English order.

Short-circuit evaluation in argument order: `And` returns false on the first false, `Or` returns true on the first true. Place expensive predicates (DB lookups, remote calls) *after* cheap ones.

### Handle cancellation and timeouts

Since `v0.2.0`, `Execute` takes `context.Context` as the first parameter (ADR-0022). A cancelled context causes the engine to:

1. Fire `OnExecutionErrored(input, ctx.Err())` on any listener that supports it.
2. Fire `OnExecutionFinished(...)` so the lifecycle pair stays balanced.
3. Return the partial `Result` plus `ctx.Err()`.

The typical wiring in a request handler:

```go
func handle(w http.ResponseWriter, r *http.Request) {
    ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
    defer cancel()

    res, err := eng.Execute(ctx, engine.Request{Input: parseRequest(r)})
    if errors.Is(err, context.DeadlineExceeded) {
        http.Error(w, "rule evaluation timed out", http.StatusGatewayTimeout)
        return
    }
    if err != nil {
        // ActionPanicError or anything else
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    write(w, res)
}
```

For predicates or actions that themselves do I/O (a DB lookup, a remote call), use the `ConditionContext` / `ActionContext` fields on `Rule` so the ctx flows through:

```go
_ = e.AddRule(inmemory.Rule{
    Name: "remote-eligible",
    ConditionContext: func(ctx context.Context, in interface{}) bool {
        return remoteService.IsEligible(ctx, in.(Order).CustomerID)
    },
    Action: approve,
})
```

A nil ctx is treated as `context.Background()` for test ergonomics, but production callers should always pass a real ctx.

### Compose multiple listeners on one engine

Listeners stack -- one engine can have many. Each one observes its slice of the lifecycle.

```go
e := inmemory.New()

counter := &observability.CountingListener{}                // per-rule hit counts
timing  := &observability.TimingListener{}                  // duration of the last execute
logger  := observability.NewLoggingListener(structuredLog)  // bridges to your Logger

e.AddListener(counter)
e.AddListener(timing)
e.AddListener(logger)

_, _ = e.Execute(ctx, engine.Request{Input: order})

// After Execute:
counter.Count("approve-fast")           // how many times this rule fired in total
counter.Total()                          // total matches across all rules
timing.LastDuration()                    // how long the last Execute took
timing.MatchesInLastExecution()          // how many matches in the last Execute (resets on next Started)
```

Listeners discover their capabilities through type assertions at notify time, so a plain `ExecutionListener` (only `OnRuleMatched`) and a full four-method listener can coexist on the same engine without either knowing the other exists.

For testing, `SnapshotListener` captures every lifecycle event for later assertion -- no need to hand-roll a recorder type:

```go
snap := &observability.SnapshotListener{}
e.AddListener(snap)

_, _ = e.Execute(ctx, engine.Request{Input: x})

if len(snap.Matches) != 2     { t.Fatalf(...) }
if snap.Finished[0].Duration  > deadline { t.Fatalf(...) }
if len(snap.Errored) != 0     { t.Fatalf(...) }
```

### Introspect at runtime

Adapters expose their rule set through two optional capability interfaces. Callers ask via type assertion -- the engine port itself does not require introspection.

```go
var eng engine.Engine = inmemory.New()
// ... register rules ...

// Just names (cheap):
if lister, ok := eng.(engine.RuleLister); ok {
    for _, name := range lister.RuleNames() {
        log.Printf("rule registered: %s", name)
    }
}

// Names + Description + Tags (richer; one allocation per rule):
if lister, ok := eng.(engine.RuleInfoLister); ok {
    for _, info := range lister.RuleInfos() {
        log.Printf("%s [%s]: %s", info.Name, strings.Join(info.Tags, ","), info.Description)
    }
}
```

A `/rules` debug endpoint is two lines:

```go
http.HandleFunc("/rules", func(w http.ResponseWriter, r *http.Request) {
    if lister, ok := eng.(engine.RuleInfoLister); ok {
        _ = json.NewEncoder(w).Encode(lister.RuleInfos())
    }
})
```

Both methods return fresh copies; the caller can mutate the returned slices (or `Tags` within them) without corrupting engine state.

### Handle errors from Execute

`Execute` returns three error categories. Use `errors.Is` for sentinel comparison, `errors.As` for typed errors carrying details:

```go
res, err := eng.Execute(ctx, engine.Request{Input: x})
switch {
case errors.Is(err, context.Canceled):
    // caller cancelled the request
case errors.Is(err, context.DeadlineExceeded):
    // request timed out
case isActionPanic(err):
    // a rule's Action panicked; the rule name is in the typed error
    var pe *inmemory.ActionPanicError
    _ = errors.As(err, &pe)
    log.Printf("rule %q action panicked: %v", pe.RuleName(), pe.Value)
case err != nil:
    // unexpected
    log.Printf("execute: %v", err)
}

func isActionPanic(err error) bool {
    var pe *inmemory.ActionPanicError
    return errors.As(err, &pe)
}
```

Note: each adapter has its own `ActionPanicError` type (`inmemory.ActionPanicError`, `firstmatch.ActionPanicError`, `priority.ActionPanicError`). They all expose `RuleName() string`, so callers that need to handle panics across adapters can define a small local interface:

```go
type ruleNamer interface{ RuleName() string }
if rn, ok := err.(ruleNamer); ok {
    log.Printf("rule %q failed", rn.RuleName())
}
```

`Result.Matched` still contains the panicking rule's name -- the rule *did* match its condition; the action is what failed. `OnExecutionErrored` fires for the panicking rule; `OnRuleMatched` does not (the match event is reserved for successful Action completion).

### Use the typed Executor

`engine/exec.Executor[In, Out]` wraps any `engine.Engine` and hides the `interface{}` cast at the call boundary. The underlying adapter does not change:

```go
import "github.com/helmedeiros/bre-go/engine/exec"

type Decision string
type Order struct { Amount int; Currency string }

eng := inmemory.New()
// ... register rules whose Action returns Decision ...

ex := exec.New[Order, Decision](eng)
decision, matched, err := ex.Execute(ctx, Order{Amount: 250, Currency: "USD"})
// decision is typed Decision -- no type assertion at the call site
```

If a rule's Action returns a value not assignable to `Out`, `Execute` returns an `*exec.OutputTypeMismatchError` carrying the expected and actual type names:

```go
var mismatch *exec.OutputTypeMismatchError
if errors.As(err, &mismatch) {
    log.Printf("expected %s, got %s", mismatch.Expected, mismatch.Got)
}
```

When no rule matches (or no matching rule had an action), the zero value of `Out` is returned with a nil error -- "no decision" is not a mismatch.

### Write adapter-agnostic helpers

Code that accepts `engine.Engine` works with every shipped adapter (and any future one) without modification. This is the whole point of the port abstraction from ADR-0003.

```go
// Backend-agnostic helper. Works with inmemory, firstmatch, priority,
// and any future adapter that satisfies engine.Engine.
func evaluateAll(ctx context.Context, eng engine.Engine, inputs []Order) (approved, rejected int, err error) {
    for _, in := range inputs {
        res, ferr := eng.Execute(ctx, engine.Request{Input: in})
        if ferr != nil {
            return approved, rejected, ferr
        }
        if res.Output == "approve" {
            approved++
            continue
        }
        rejected++
    }
    return approved, rejected, nil
}
```

To opt into optional capabilities, type-assert at call time:

```go
func describeEngine(eng engine.Engine) {
    if lister, ok := eng.(engine.RuleInfoLister); ok {
        log.Printf("loaded %d rule(s)", len(lister.RuleInfos()))
    }
    if _, ok := eng.(engine.ListenerHost); ok {
        log.Printf("supports listeners")
    }
}
```

This pattern lets a caller wire a single `evaluateAll` once and ship code that:

- Works with `engine/inmemory` for tests and dev.
- Works with `engine/priority` once rules are loaded from a config file.
- Will work with a future `engine/gorules` adapter when GoRules Zen ships -- no change to `evaluateAll`.

If your helper needs a richer capability than the bare `engine.Engine` port provides, take the capability interface in the signature instead:

```go
func auditRuleSet(lister engine.RuleInfoLister) error { ... }
```

Callers pass the engine (it satisfies the interface implicitly via the type assertion the compiler does at the call site).

### Load rules from a CSV file

Since `v0.3.0`, `engine/csv` provides a `Loader[RC]` that reads a CSV source and yields typed `RuleConfig`s. The caller writes a `LineParser` (column-to-field mapping) and a small bridging closure between the loader and the adapter's `AddRule`.

```go
import (
    "github.com/helmedeiros/bre-go/engine"
    "github.com/helmedeiros/bre-go/engine/csv"
    "github.com/helmedeiros/bre-go/engine/priority"
)

// Caller-defined config struct. Must satisfy engine.RuleConfig.
type TierConfig struct {
    Name      string
    Priority  int
    Threshold int
    Decision  string
}

func (c TierConfig) RuleName() string { return c.Name }

// Caller-defined parser. Maps a CSV row's columns to the struct's fields.
func parseTier(columns []string) (TierConfig, error) {
    if len(columns) < 4 {
        return TierConfig{}, fmt.Errorf("expected 4 columns")
    }
    priority, _ := strconv.Atoi(columns[1])
    threshold, _ := strconv.Atoi(columns[2])
    return TierConfig{Name: columns[0], Priority: priority, Threshold: threshold, Decision: columns[3]}, nil
}

// Wire it together
loader := csv.NewLoader("rules.csv", parseTier).SkipHeader(1)
eng := priority.New()

err := engine.Load[TierConfig](loader, func(c TierConfig) error {
    return eng.AddRule(priority.Rule{
        Name:      c.Name,
        Priority:  c.Priority,
        Condition: func(in interface{}) bool { return in.(int) >= c.Threshold },
        Action:    func(interface{}) interface{} { return c.Decision },
    })
})
if err != nil {
    var le *csv.LoadError
    if errors.As(err, &le) {
        log.Fatalf("rules.csv row %d: %v", le.Row, le.Err)
    }
    log.Fatal(err)
}
```

Other source shapes (embed.FS, HTTP bodies, tests with `strings.NewReader`) use `csv.NewLoaderFromReader(r, parser)` instead. The bridging closure stays identical -- the only thing that changes is where the bytes come from.

`csv.LoadError` carries `Path` and 1-indexed `Row` for diagnostics. `Unwrap()` exposes the underlying error so `errors.Is` chains work normally.

### Load rules from a JSON file

Since `v0.7.0`, `engine/json` provides a `Loader[RC]` for JSON sources whose top level is an array of objects. The caller writes an `ItemParser` (one `json.RawMessage` in, one typed `RuleConfig` out) and the same bridging closure used with CSV.

```go
import (
    encjson "encoding/json"

    "github.com/helmedeiros/bre-go/engine"
    bjson "github.com/helmedeiros/bre-go/engine/json"
    "github.com/helmedeiros/bre-go/engine/priority"
)

// Caller-defined config struct. Must satisfy engine.RuleConfig.
type TierConfig struct {
    Name      string
    Priority  int
    Threshold int
    Decision  string
}

func (c TierConfig) RuleName() string { return c.Name }

// Caller-defined parser. Unmarshals each array element into a wire
// shape, then maps it to the engine-internal struct.
func parseTier(item encjson.RawMessage) (TierConfig, error) {
    var wire struct {
        Name      string `json:"name"`
        Priority  int    `json:"priority"`
        Threshold int    `json:"threshold"`
        Decision  string `json:"decision"`
    }
    if err := encjson.Unmarshal(item, &wire); err != nil {
        return TierConfig{}, err
    }
    return TierConfig{Name: wire.Name, Priority: wire.Priority, Threshold: wire.Threshold, Decision: wire.Decision}, nil
}

loader := bjson.NewLoader("rules.json", parseTier)
eng := priority.New()

err := engine.Load[TierConfig](loader, func(c TierConfig) error {
    return eng.AddRule(priority.Rule{
        Name:      c.Name,
        Priority:  c.Priority,
        Condition: func(in interface{}) bool { return in.(int) >= c.Threshold },
        Action:    func(interface{}) interface{} { return c.Decision },
    })
})
if err != nil {
    var le *bjson.LoadError
    if errors.As(err, &le) {
        log.Fatalf("rules.json item %d: %v", le.Index, le.Err)
    }
    log.Fatal(err)
}
```

The expected document shape is a top-level array: `[{...}, {...}]`. The element schema is whatever the `ItemParser` decodes -- the loader stays format-agnostic.

`bjson.LoadError` carries `Path` and 0-indexed `Index` (natural JSON-array position); document-level failures (file open, malformed JSON, top-level value not an array) use `Index: -1`. As with `csv.LoadError`, `Unwrap()` exposes the underlying error so `errors.Is` chains work.

Use `bjson.NewLoaderFromReader(r, parser)` for `embed.FS`, HTTP bodies, or tests with `strings.NewReader`. The bridging closure stays identical.

### Compose rules from multiple sources

Real applications often layer rule sets: a baseline ships with the application, per-tenant rules override it, an experiments file adds short-lived rules on top. `engine.ChainProviders` combines multiple providers into one without bespoke plumbing.

```go
defaults := csv.NewLoader[TierConfig]("defaults.csv", parseTier)
tenant   := csv.NewLoader[TierConfig](tenantPath, parseTier)
experiments := csv.NewLoader[TierConfig]("experiments.csv", parseTier)

combined := engine.ChainProviders(defaults, tenant, experiments)

eng := priority.New()
err := engine.Load[TierConfig](combined, func(c TierConfig) error {
    return eng.AddRule(toPriorityRule(c))
})
```

Order matters: `priority.Engine` resolves precedence by the `Priority` field, but registration order breaks ties (ADR-0019). If two sources define rules with the same priority, the later source's rule loses the tie. Order the chain accordingly.

First-error-wins: if any provider returns an error from `RuleConfigs()`, `ChainProviders` short-circuits and returns that error. Earlier providers' configs are not partially applied -- the bridging closure passed to `engine.Load` never runs until every provider has returned its slice successfully.

Empty providers (zero `RuleConfigs`) are fine. A "feature-flag gate" can return an empty slice when disabled:

```go
type GuardedProvider[RC engine.RuleConfig] struct {
    enabled bool
    inner   engine.RuleConfigProvider[RC]
}

func (g *GuardedProvider[RC]) RuleConfigs() ([]RC, error) {
    if !g.enabled {
        return nil, nil
    }
    return g.inner.RuleConfigs()
}
```

### Propagate correlation IDs

Since `v0.4.0`, `engine.WithCorrelationID` and `engine.CorrelationIDFromContext` standardize how a request-scoped identifier flows through `Execute`. Callbacks that already receive `context.Context` (`ConditionContext`, `ActionContext`) read the ID directly:

```go
// In the HTTP handler:
ctx := r.Context()
ctx = engine.WithCorrelationID(ctx, r.Header.Get("X-Request-ID"))
_, _ = eng.Execute(ctx, engine.Request{Input: order})

// In a rule's ActionContext:
_ = e.AddRule(inmemory.Rule{
    Name: "audit-trail",
    Condition: conditions.Always(),
    ActionContext: func(ctx context.Context, in interface{}) interface{} {
        id := engine.CorrelationIDFromContext(ctx)
        log.Printf("request=%s rule=audit-trail input=%v", id, in)
        return nil
    },
})
```

The key used internally is an unexported type, so no other middleware can collide with it or read the value through `ctx.Value(string)("correlation-id")` guesses.

Listeners do not yet receive `context.Context`; for now, observers wanting per-execution correlation either (a) attach a per-request listener that captures the ID in its closure or (b) emit the correlation via an `ActionContext` callback that runs on every relevant rule. A future ADR will add ctx-aware listener interfaces (`OnRuleMatchedCtx` and the lifecycle variants) once a real caller shapes the requirements.

### Write rule conditions as strings

Since `v0.5.0`, `engine/parser` turns expression strings into evaluable predicates. Combined with the v0.3.0 / v0.4.0 loaders, rule conditions can live in CSV / JSON files instead of compiled Go closures.

Supported grammar:

| Construct | Example |
|---|---|
| Equality | `origin == "DE"` |
| Inequality | `tier != "economy"` |
| Set membership | `tier IN ("vip", "premium")` |
| Set exclusion | `partner NOT IN ("blocked-a", "blocked-b")` |
| Combinator | `a == "x" AND b == "y" OR NOT c == "z"` |
| Parens | `(a == "x" OR b == "y") AND c == "z"` |

Precedence is `OR < AND < NOT < comparison`. String literals only; numeric, boolean and float literals are out of scope for `v0.5.0` (encode booleans as `"true"` / `"false"` strings).

Wiring with a CSV-loaded rule set:

```go
type RuleRow struct {
    Name     string
    Priority int
    CondExpr string
    Percent  int
}

func (r RuleRow) RuleName() string { return r.Name }

func parseRow(cols []string) (RuleRow, error) {
    // cols: name, priority, condition_expression, percent
    prio, _ := strconv.Atoi(cols[1])
    pct, _ := strconv.Atoi(cols[3])
    return RuleRow{Name: cols[0], Priority: prio, CondExpr: cols[2], Percent: pct}, nil
}

func reqAsFact(in interface{}) map[string]interface{} {
    r := in.(Request)
    return map[string]interface{}{
        "origin":  r.Origin,
        "tier":    r.Tier,
        "partner": r.Partner,
    }
}

loader := csv.NewLoader[RuleRow]("rules.csv", parseRow)
eng := priority.New()

err := engine.Load[RuleRow](loader, func(r RuleRow) error {
    pred, parseErr := parser.Parse(r.CondExpr)
    if parseErr != nil {
        return fmt.Errorf("rule %q: %w", r.Name, parseErr)
    }
    return eng.AddRule(priority.Rule{
        Name:      r.Name,
        Priority:  r.Priority,
        Condition: parser.AsCondition(pred, reqAsFact),
        Action:    func(interface{}) interface{} { return r.Percent },
    })
})
```

Parse errors surface as `*parser.ParseError` with a `Pos` byte offset, making operator-level diagnostics easy:

```go
var pe *parser.ParseError
if errors.As(parseErr, &pe) {
    log.Printf("syntax error at offset %d: %s", pe.Pos, pe.Message)
}
```

Adding a new rule is now a CSV row edit. No recompile.

**Performance**: parsing is ~100-700 ns per expression depending on complexity (do it at load time, once). Evaluating a parsed `Predicate` is ~20 ns/call with zero allocations.

**Single-quoted strings in CSV**: rule conditions written inside CSV cells should use **single quotes** for string literals, not double quotes — CSV reserves `"` for its own field-quoting convention. The parser accepts both `'...'` and `"..."`, so:

```csv
de-vip,100,"origin == 'DE' AND tier IN ('vip', 'premium')",25
```

The condition field itself is wrapped in CSV double quotes (required when the field contains commas — IN lists trigger this), and single quotes inside denote string literals to the parser. No escaping gymnastics.

The reverse — double-quoted strings with CSV-style `""` escaping — also works (`"origin == ""DE"""`) but is harder to read.

### Inspect parsed conditions as typed trees

Since `v0.6.0`, `parser.ParseToCondition` returns a typed tree instead of an opaque closure. The tree's concrete types -- `StringCondition`, `SetCondition`, `AndCondition`, `OrCondition`, `NotCondition` -- can be inspected, marshalled, or transformed.

Use the tree for runtime introspection ("what fields does this rule reference?"):

```go
import "github.com/helmedeiros/bre-go/engine/parser"

cond, _ := parser.ParseToCondition(`origin == 'DE' AND tier IN ('vip', 'premium')`)

var fields []string
var walk func(c parser.Condition)
walk = func(c parser.Condition) {
    switch v := c.(type) {
    case parser.StringCondition:
        fields = append(fields, v.Field)
    case parser.SetCondition:
        fields = append(fields, v.Field)
    case parser.AndCondition:
        for _, child := range v.Children {
            walk(child)
        }
    case parser.OrCondition:
        for _, child := range v.Children {
            walk(child)
        }
    case parser.NotCondition:
        walk(v.Child)
    }
}
walk(cond)
// fields == ["origin", "tier"]
```

Use `parser.AsRuleCondition(cond, factOf)` instead of `parser.AsCondition` when you've kept the Condition tree (rather than going through `Parse`). Same shape; the bridge takes the typed tree directly:

```go
rule := inmemory.Rule{
    Condition: parser.AsRuleCondition(cond, factOf),
    Action:    func(interface{}) interface{} { return "approve" },
}
```

`AndCondition` and `OrCondition` chains flatten -- three rules combined with `AND` produce one `AndCondition` with three children, not nested binaries. Easier to walk; sets up the indexed-matcher work in a future release.

When in doubt, `Parse` still works -- it internally calls `ParseToCondition` then `AsPredicate`. Use `Parse` for evaluation, `ParseToCondition` when you need the tree.
