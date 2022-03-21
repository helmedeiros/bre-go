# 19. A Priority-Ordered Adapter

## Status

Accepted

## Context

Two adapters ship today: `engine/inmemory` (insertion-order, all-match, last-action-wins) and `engine/firstmatch` (insertion-order, first-match). Both rely on **insertion order** as the source of precedence: the order a caller calls `AddRule` decides what gets evaluated first.

That works when the caller controls the call site. It does not work for two real patterns:

1. **Decision tables loaded from a config file**, where rule precedence is part of the data, not the loading code. A row's priority is a property of the rule, not a side effect of file-line order.
2. **Modular rule sets composed at runtime** from multiple sources (defaults plus tenant overrides plus environment guards). Each source builds its own slice of rules; the engine has no way to interleave them by importance without the caller pre-sorting.

A third adapter `engine/priority` resolves both. Rules carry an explicit `Priority int`. `Execute` evaluates in **descending priority** (highest first), returning on the first matching rule -- decision-table semantics. Ties (two rules with the same priority) break by insertion order, so callers loading from a stable source get deterministic behavior.

The "all-match in priority order" variant was considered and rejected for this cut. Priority-ordered all-match is rarely useful: priority *means* "this matters more than the others"; running every matching rule in importance order produces an `Output` chain that is hard to reason about. If a real caller appears wanting that semantic, a future adapter can add it.

The shape choice for `Priority`:

- `int`, not `uint` or a typed `Priority` enum. Negative priorities are useful for "always-last" catch-alls (`Priority: -1000` as the explicit default). A typed enum forces every caller to import constants for a value that is fundamentally a number.
- Defaults to `0` for any rule that does not set it. `0` is a meaningful priority bucket; rules adding to it are merely unprioritized in that bucket.
- Higher number = higher priority. Standard direction; matches how a reader scans "1, 2, 3" tables.

## Decision

Add `engine/priority` as a third sibling sub-package to `engine/inmemory` and `engine/firstmatch`. It exports:

```go
type Rule struct {
    Name      string
    Priority  int
    Condition func(input interface{}) bool
    Action    func(input interface{}) interface{}
}

func New() *Engine
func (e *Engine) AddRule(r Rule) error
func (e *Engine) AddListener(l observability.ExecutionListener)
func (e *Engine) Execute(req engine.Request) (engine.Result, error)
func (e *Engine) RuleNames() []string
```

Same registration validation as the existing adapters (empty name, nil condition, duplicate name -- shape-first then state-second), same panic recovery into a local `ActionPanicError`, same lifecycle and listener wiring (`engine.ListenerHost`, `engine.RuleLister` both satisfied), same contract suite wiring.

`AddRule` is O(N) over the registered rules to do duplicate-name detection; insertion stays append-only. `Execute` sorts a working copy at evaluation time (stable sort by descending Priority, ties by insertion order) and walks until the first match. The sort is per-`Execute` to keep `AddRule` cheap and to let callers add rules between executions without re-sorting state externally.

The `enginetest.RunContractTests` suite runs against `priority` via the same wiring `inmemory` and `firstmatch` use. The contract cases pass unchanged: they all use single-rule scenarios where priority does not affect behavior.

## Consequences

The port abstraction proves out a second time: three adapters, no `engine.Engine` change. The three differ in two orthogonal dimensions -- ordering (insertion vs priority) and match policy (all vs first) -- with three of the four cells now filled:

|              | All-match    | First-match    |
|--------------|--------------|----------------|
| Insertion    | `inmemory`   | `firstmatch`   |
| Priority     | (deliberately not shipped) | `priority` |

The empty cell stays empty until a real caller asks for it. The matrix view makes the omission explicit.

Per-execution sorting trades CPU for simpler state. For rule sets in the dozens (where this library is aimed), the sort is microseconds; for thousands, a caller can either pre-sort and use `firstmatch`, or a future ADR adds an "AddRule keeps the list sorted" variant. Either path is additive.

The README's "Which adapter do I want?" table grows a third row. The CONTRIBUTING adapter recipe stays unchanged -- the four-step recipe produced this adapter without amendment, which is the testimony that the recipe works.
