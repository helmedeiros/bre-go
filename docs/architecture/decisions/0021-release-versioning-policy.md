# 21. Release Versioning Policy And Cutting v0.1.0

## Status

Accepted

## Context

`bre-go` has been on `main` since January 3, 2022 with no tagged release. ADR-0001 stated the project would "follow Semantic Versioning once a first tagged release is cut," and ADR-0011 codified the lifecycle of ADRs but not of releases. Today, fourteen weeks in:

- The engine port (ADR-0003) is stable: three concrete adapters all pass the same `enginetest` contract suite without port changes.
- Six public packages exist (`engine`, `engine/inmemory`, `engine/firstmatch`, `engine/priority`, `engine/conditions`, `engine/exec`, `engine/enginetest`, `observability`).
- Three optional capability interfaces (`ListenerHost`, `RuleLister`, `RuleInfoLister`) and three lifecycle listener role interfaces are in place.
- 20 ADRs, one supersession (0004 → 0013) cleanly executed.
- 100% per-package coverage; CI green on every commit since week one.

The architecture has reached the "feature-complete for ADR-0001's bounded goals" milestone documented in last week's README rewrite. Untagged code is awkward for downstream consumers: `go get` pins to a commit hash, the module proxy doesn't cache it as a release, and pkg.go.dev shows the latest commit as "v0.0.0-YYYYMMDD-hash". A tag turns the library into a real version downstream can depend on.

Three policy questions need answers before tagging:

**1. What version does the first release get?** Two options:

- `v0.1.0`: signals "first usable release, pre-1.0 means anything can change". The Go community treats `0.x.y` as "use at your own risk, but the maintainer thinks it's worth using".
- `v1.0.0`: signals "stable API, backward compatibility commitment". Premature today -- the library has no production users, real-world feedback hasn't shaped the surface yet, and a `v1.0.0` rolled back later would be embarrassing.

Pick `v0.1.0`.

**2. What does SemVer mean while we're pre-1.0?** Standard interpretation for `0.x.y`:

- `0.x.y` → `0.x.(y+1)`: bug fixes, doc updates, additive non-breaking changes. Existing imports continue to work.
- `0.x.y` → `0.(x+1).0`: any breaking change to the public surface. Adds, removes, signature changes, behavior changes all qualify. Callers may need code edits.
- `0.x.y` → `1.0.0`: when the surface is judged stable and a real-world user has shipped against it.

The README's "Stability" section enumerates what's stable today; any change to those items is a `0.(x+1).0` bump.

**3. How does the release flow work?** Per-release steps land in one commit each (the tag commit is the *next* commit after the prep commit):

1. CHANGELOG: move `[Unreleased]` entries into `[X.Y.Z] - YYYY-MM-DD` heading, leave an empty `[Unreleased]` on top.
2. README's Status / Stability sections update if anything material changed.
3. Run `make ci-local` from a clean checkout. All green.
4. Create an annotated tag with the release date and a one-line message: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`.
5. Push the tag: `git push --tags`.
6. The next commit reopens `[Unreleased]` for the in-flight work.

The tag is immutable in practice. Moving or deleting a tag after push breaks downstream consumers who have already resolved it. Re-cutting requires a new version number.

## Decision

Cut **v0.1.0** this week, dated when the tag actually lands (Apr 7, 2022 planned).

The CHANGELOG's existing `[Unreleased]` section converts to `[0.1.0] - 2022-04-07`. The `[Unreleased]` section reopens empty after the tag. The README Status line drops "Pre-1.0, but feature-complete" in favor of "v0.1.0 released" with the same Stability section unchanged.

The library uses standard Go module versioning: tags are `vMAJOR.MINOR.PATCH`. No prefixed sub-packages, no per-package versions. The entire module ships together.

No release notes file is added beyond the CHANGELOG; the CHANGELOG IS the release notes. GitHub's "Releases" UI can auto-render the relevant CHANGELOG section.

## Consequences

Downstream consumers gain a tagged version to depend on. `go get github.com/helmedeiros/bre-go@v0.1.0` becomes the recommended install path. pkg.go.dev indexes the package under the version. The Go module proxy caches the release.

The pre-1.0 framing means breaking changes remain allowed -- but each one is now a visible event (a minor bump) rather than a silent commit. ADR-0011's lifecycle convention extends naturally: a superseding ADR that breaks the public surface implies a `v0.(x+1).0` bump.

A `v1.0.0` decision waits for real-world usage. When a downstream user reports running this in production, the question reopens. Until then, every release stays `0.x.y`.

A `CHANGELOG` discipline is now load-bearing for releases. The Friday roll-up commit each week populates `[Unreleased]`; cutting a release converts that section into the version heading. The two-step (roll-up, then convert) keeps the discipline simple and the diff readable.
