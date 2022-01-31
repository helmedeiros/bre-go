# 11. ADR Lifecycle And Supersession Convention

## Status

Accepted

## Context

The first ten ADRs in this repository all sit at "Accepted" today, but several are predictably temporary. ADR-0004 (pre-generics input handling) already self-flags as provisional pending Go 1.18 GA in March 2022. ADR-0005's `(Result, error)` return on `Execute` could be revisited if a richer execution outcome type ever emerges. ADR-0007's listener shape will likely grow lifecycle events (`Started`, `Finished`) as a follow-up.

Without an explicit convention, readers will discover "ADR-0004 says use `interface{}`" while the code uses generics, and have no breadcrumb pointing at the newer ADR that replaced it. The history of decisions becomes lossy precisely where it most needs to be readable.

A "delete the old ADR" policy is wrong: ADRs document the *reasoning at the time*, and the reasoning is what a future reader needs to understand the codebase. The right model is the one Michael Nygard's original ADR proposal used -- mark the old one superseded, leave the file in place, and link forward.

## Decision

Every ADR's `## Status` line uses one of these exact values:

- `Proposed` -- written, not yet on `main`.
- `Accepted` -- implementation on `main`. Current truth.
- `Accepted (provisional; <sunset condition>)` -- accepted, with a known follow-up trigger spelled out.
- `Superseded by ADR-NNNN` -- replaced. Old file stays.
- `Deprecated` -- reversed, no replacement. Old file stays.

When ADR-N supersedes ADR-M:

1. Edit ADR-M's `## Status` line to read `Superseded by ADR-N`.
2. Add a one-line callout at the top of ADR-M's body: `> Superseded by [ADR-N <slug>](NNNN-slug.md).`
3. Update `docs/architecture/decisions/README.md`'s status column for ADR-M.
4. Ship all three edits in the *same commit as ADR-N's implementation*, so `main` is never internally inconsistent.

The README index carries a status table with the five markers as the single source of truth for "what is still in effect". A small CI check (added in a follow-up commit) verifies every ADR file in the folder is indexed in the README, so a new ADR cannot silently drift out of the index.

## Consequences

The history of decisions becomes a navigable chain: a reader landing on a superseded ADR sees the forward link in the first paragraph and the status line, both pointing at the current ADR. The lifecycle stays explicit, not implicit-by-recency.

The cost is a small editing discipline when introducing a superseding ADR (one extra status edit, one README edit). The CI check makes the index drift impossible, so the discipline is enforced rather than vibes-based.

The five statuses are deliberately few. `Rejected` is not on the list -- rejected proposals never become ADRs in this repo; they live in PR descriptions or design docs and disappear. ADRs in this folder are the *accepted history*, not the proposal queue.
