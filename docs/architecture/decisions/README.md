# Architecture Decision Records

Each file in this folder captures one architecture decision made on the bre-go codebase, following the standard ADR shape (Status / Context / Decision / Consequences).

New decisions get the next number and a short kebab-case slug:

```
NNNN-short-decision-name.md
```

## Lifecycle

Every ADR carries one of these statuses on its `## Status` line:

- **Proposed** — written but not yet implemented on `main`. May still change.
- **Accepted** — implementation is on `main`. Treat the ADR as current truth.
- **Accepted (provisional)** — accepted, but with a known sunset condition spelled out in the ADR body. Will get a follow-up that either re-confirms or supersedes it.
- **Superseded by ADR-NNNN** — the decision was replaced. The file stays so the history reads as a sequence of choices; readers should jump to the newer ADR for current behavior.
- **Deprecated** — the decision was reversed and nothing replaces it. The file stays for the same historical reason.

When a new ADR replaces an old one, update the old ADR's `## Status` line to `Superseded by ADR-NNNN` and add a one-line "Superseded by [ADR-NNNN](NNNN-slug.md)" link at the top of the body. Also update this index so the status is visible without opening the file.

## Index

Status markers: 📝 Proposed · ✅ Accepted · ⏳ Accepted (provisional) · ♻️ Superseded · ⊘ Deprecated

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-bounded-goals.md) | Bounded Goals For This Project | ✅ Accepted |
| [0002](0002-go-as-the-language.md) | Go As The Implementation Language | ✅ Accepted |
| [0003](0003-engine-port.md) | The Engine Port | ✅ Accepted |
| [0004](0004-pre-generics-input-handling.md) | Pre-Generics Input Handling | ♻️ Superseded by [ADR-0013](0013-generic-executor-layer.md) |
| [0005](0005-engine-execute-returns-error.md) | engine.Engine.Execute Returns An Error | ✅ Accepted |
| [0006](0006-rename-context-to-request.md) | Rename engine.Context To engine.Request | ✅ Accepted |
| [0007](0007-execution-listener.md) | Execution Listener As An Observer Port | ✅ Accepted |
| [0008](0008-built-in-listeners.md) | Built-In Listener Implementations | ✅ Accepted |
| [0009](0009-reject-duplicate-rule-names.md) | Reject Duplicate Rule Names On AddRule | ✅ Accepted |
| [0010](0010-listener-host-optional-interface.md) | ListenerHost As An Optional Interface | ✅ Accepted |
| [0011](0011-adr-lifecycle-and-supersession.md) | ADR Lifecycle And Supersession Convention | ✅ Accepted |
| [0012](0012-reject-rules-with-nil-condition.md) | Reject Rules With A Nil Condition | ✅ Accepted |
| [0013](0013-generic-executor-layer.md) | Generic Executor Layer Over engine.Engine | ✅ Accepted |
| [0014](0014-firstmatch-adapter.md) | A First-Match Adapter Alongside Inmemory | ✅ Accepted |
| [0015](0015-boolean-condition-combinators.md) | Boolean Condition Combinators | ✅ Accepted |
| [0016](0016-rule-lister-optional-interface.md) | RuleLister As An Optional Interface | ✅ Accepted |
| [0017](0017-per-execution-lifecycle-listeners.md) | Per-Execution Lifecycle Listeners | ✅ Accepted |
| [0018](0018-action-panic-recovery.md) | Action Panic Recovery And The Errored Lifecycle Event | ✅ Accepted |
| [0019](0019-priority-adapter.md) | A Priority-Ordered Adapter | ✅ Accepted |
| [0020](0020-rule-info-introspection.md) | RuleInfo Introspection Beyond Names | ✅ Accepted |
| [0021](0021-release-versioning-policy.md) | Release Versioning Policy And Cutting v0.1.0 | ✅ Accepted |
| [0022](0022-context-propagation.md) | Propagate context.Context Through Execute | ✅ Accepted (v0.2.0) |
| [0023](0023-rule-config-provider.md) | RuleConfigProvider: Decouple Matchers From Loading | ✅ Accepted |
| [0024](0024-csv-loader.md) | The CSV Loader Sub-Package | ✅ Accepted |
| [0025](0025-multi-source-rule-composition.md) | Multi-Source Rule Composition | ✅ Accepted |
| [0026](0026-correlation-id-propagation.md) | Correlation-ID Propagation Through Execute | ✅ Accepted |
| [0027](0027-parser-package.md) | The Expression Parser Package | ✅ Accepted |
| [0028](0028-typed-condition-tree.md) | Typed Condition Tree | ✅ Accepted |
| [0029](0029-internal-adapter-notifier.md) | Internal Adapter Notifier (Listener Wiring Extraction) | ✅ Accepted |
| [0030](0030-json-loader.md) | The JSON Loader Sub-Package | ✅ Accepted |
| [0031](0031-adapter-benchmark-harness.md) | Adapter Performance Benchmark Harness | 🟡 Proposed |
