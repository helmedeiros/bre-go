# 14. A First-Match Adapter Alongside Inmemory

## Status

Accepted

## Context

The `inmemory` adapter walks every rule and lets the *last* matching action overwrite `Output`. That is one valid BRE policy (used for "accumulate all decisions"); it is not the only one. The other major policy is **first-match**: walk rules in insertion order, return on the first one whose condition is true. Decision tables, content classifiers, and "first applicable rate" pricing engines all behave that way.

A caller using `bre-go` today who wants first-match has two bad options:

1. Order their rules carefully and accept that *all* matching actions run anyway (since `inmemory.Execute` returns last-action-wins, not "stop after first match").
2. Wrap `engine.Engine.Execute` themselves and parse `Result.Matched[0]`, then re-run only that rule's action. Awkward and brittle.

The cleaner answer is a second adapter. Doing this also validates the port abstraction the project committed to in ADR-0003: the *whole point* of carving out `engine.Engine` was that a future second adapter could plug in without changing the port. Six weeks in, with the port still pristine, time to actually exercise it.

The shape choice: do we put first-match behind a *config flag* on `inmemory.Engine` (e.g., `Mode: FirstMatch | AllMatch`) or as its own adapter? We pick its own adapter:

- Each adapter is a single semantic. A reader looking at `firstmatch.Engine` knows what it does without grepping for a mode. Same for `inmemory.Engine`.
- The contract test suite gets a second wiring point, which catches port-level regressions a flag-based approach would miss.
- Listeners, validation, and rule shape are shared by composition (both adapters use the same `Rule` value type), not by overloading a single struct.

## Decision

Add `engine/firstmatch` as a sibling sub-package to `engine/inmemory`. The two adapters share:

- The same `Rule` value type, defined locally in each package (we do not lift it to `engine/`; ADR-0004 keeps the port surface minimal). The fields are identical: `Name`, `Condition`, `Action`.
- The same registration validation sentinels and check order (ADR-0009, ADR-0012): empty name, nil condition, duplicate name.
- The same `engine.ListenerHost` optional interface satisfaction. `firstmatch.Engine.AddListener` accepts `observability.ExecutionListener`s. `Execute` fires `OnRuleMatched` for the one rule that matched (and only that one).

The two adapters differ only in `Execute`:

- `inmemory`: walk all rules, accumulate `Matched`, last action wins on `Output`.
- `firstmatch`: walk in order, on the first matching rule append to `Matched`, run its action, return.

The contract test suite (`engine/enginetest`) is wired from `firstmatch/contract_test.go` exactly as it was for `inmemory`. The six existing contract cases all use single-rule scenarios, so they pin port-level behavior that both adapters must satisfy. Multi-rule semantics stay in adapter-specific tests.

## Consequences

The port abstraction proves out: two adapters, no `engine.Engine` change, same contract suite, same listener wiring. If the port had been wrong, this is where the friction would have surfaced.

The library now ships two policies. The README and CONTRIBUTING get a short "which adapter do I want?" section so callers do not have to read both packages to pick. The duplication of `Rule` and the validation sentinels is intentional -- a shared `engine/rules` package would couple every adapter to the same shape, which is the *opposite* of what the port pattern is for.

When a third adapter arrives (the gorules-backed one in mid-2023, per ADR-0001), it does not need to look like either of these. It implements `engine.Engine` and brings its own rule shape native to gorules' schema.
