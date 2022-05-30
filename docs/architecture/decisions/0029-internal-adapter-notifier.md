# 29. Internal Adapter Notifier (Listener Wiring Extraction)

## Status

Accepted

## Context

A code-smell audit after v0.6.0 quantified the duplication across the three adapter packages: **101 lines byte-identical** across `engine/inmemory/inmemory.go`, `engine/firstmatch/firstmatch.go`, and `engine/priority/priority.go` -- roughly 16% of 624 total adapter LOC.

The duplication concentrates in the listener-wiring side:

- `listeners []observability.ExecutionListener` field
- `AddListener(l)` method
- `notify(rule, input, output)` helper
- `notifyStarted(input)` helper
- `notifyFinished(input, output, matched, duration)` helper
- `notifyErrored(input, err)` helper

These six items have zero per-adapter variation. Every adapter walks the listener slice, type-asserts for the relevant capability interface (`ExecutionStartedListener`, `ExecutionFinishedListener`, `ExecutionErroredListener`), and calls through. The duplication exists for ADR-0014's reason -- "each adapter is its own package, owns its types" -- but the *listener mechanics* don't need to be per-adapter to honor that.

ADR-0014's core promise was: each adapter owns its **Rule type** and its **Execute semantics**. Neither of those is touched by the listener wiring. Sharing the listener mechanics doesn't violate the port-per-adapter principle.

Three design choices:

**1. Where does the shared code live?**

- (a) `engine/internal/adapter` (or similar internal sub-package).
- (b) Promote to a public `engine/adapter` package.
- (c) Add methods to a new public type in the existing `engine` package.

Pick (a). Go's `internal/` convention prevents external imports, so the helper stays a private implementation detail of bre-go itself. Callers cannot build their own adapter on top of the shared notifier directly; future external adapter authors would either inline the listener mechanics themselves (free to do, since the surface is small) or wait for a follow-up ADR that promotes the helper.

**2. How do adapters consume the shared code?**

- (a) Embedded value: `type Engine struct { adapter.Notifier; rules []Rule }`.
- (b) Embedded pointer: `type Engine struct { *adapter.Notifier; ... }` with a constructor.
- (c) Free functions taking a `*[]observability.ExecutionListener` slice pointer.

Pick (a). Embedded value is the simplest Go pattern: methods on `*adapter.Notifier` get promoted onto `*Engine`, and `e.AddListener(l)` works via method promotion. No constructor change required -- the zero value of the Notifier is usable. Pointer-embedding would force every adapter's `New()` to construct the Notifier explicitly; that's churn for no benefit.

**3. What does `Notifier` expose?**

```go
type Notifier struct {
    listeners []observability.ExecutionListener
}

func (n *Notifier) AddListener(l observability.ExecutionListener)
func (n *Notifier) NotifyMatched(rule string, input, output interface{})
func (n *Notifier) NotifyStarted(input interface{})
func (n *Notifier) NotifyFinished(input, output interface{}, matched []string, duration time.Duration)
func (n *Notifier) NotifyErrored(input interface{}, err error)
```

`NotifyMatched` rather than `Notify` -- the verb is explicit, no ambiguity at the call site. Method names use the `Notify*` prefix to mirror the listener-side `On*` prefix.

## Decision

Add `engine/internal/adapter` package with the `Notifier` type and its five methods. Each adapter:

1. Embeds `adapter.Notifier` by value.
2. Removes its local `listeners` field.
3. Removes its local `AddListener`, `notify`, `notifyStarted`, `notifyFinished`, `notifyErrored` methods.
4. Updates its `Execute` to call the embedded methods: `e.NotifyStarted(input)`, `e.NotifyMatched(name, input, output)`, etc.

The public API of each adapter does not change -- `AddListener` still exists (now via method promotion). Tests do not need updates beyond the helper-name change inside `Execute`. The `engine.ListenerHost` compile-time witnesses (`var _ engine.ListenerHost = (*Engine)(nil)`) continue to compile.

The four other duplicated helpers (`evaluateCondition`, `hasAction`, `runAction`, `copyTags`) stay per-adapter for now. They're pure functions over the adapter's local `Rule` type; sharing them would require either (i) generics with a `Rule` constraint interface, or (ii) reflection. Neither is justified at this duplication level. Re-audit after Phase 4 lands more adapters.

## Consequences

Adapter file sizes drop by ~50 LOC each (~25% of each adapter). The remaining adapter code is what's truly per-adapter: the `Rule` struct, the `AddRule` validation, the `Execute` semantics (insertion-order all-match / first-match / priority-ordered), and the `ActionPanicError` type.

Compile-time witnesses on each adapter (`var _ engine.ListenerHost = (*Engine)(nil)`) keep verifying that the promoted methods satisfy the optional capability interface. If the embedded method signature drifts, the build breaks immediately.

The `internal/` placement means future adapter authors outside this repo cannot import the shared Notifier. That's deliberate: a future caller who needs to ship a new adapter can either inline the ~30 lines themselves or open an ADR to promote the helper. Premature promotion would lock in the shape before more adapters exist to validate it.

Test coverage holds at 100% per package -- the shared methods get covered by the same test cases that previously covered the duplicate copies, just now reaching them through the embedded receiver.
