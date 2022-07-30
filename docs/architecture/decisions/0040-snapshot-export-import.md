# 40. Snapshot Export and Import for `engine/indexed`

## Status

Proposed — target v0.15.0. Adds `Engine.ExportSnapshot() (*Snapshot, error)`
and `LoadSnapshot(*Snapshot, map[string]RuleCallbacks) (*Engine, error)`
on `engine/indexed`. A `Snapshot` is the JSON-serializable
representation of a built engine's rule set; `LoadSnapshot`
reconstructs an already-built engine ready to `Execute`. Hook-bearing
engines refuse to export. Rule action callbacks are re-attached at
Load time via a name-keyed registry.

## Context

Today the indexed adapter has one path to operational readiness:
construct an `Engine`, call `AddRule` N times, then `Build`. For a
rule set loaded from CSV/JSON that means every process re-parses
the source, re-validates each shape, re-canonicalizes value sets,
re-allocates the bucket structures. For a 10k-rule engine that's
tens of milliseconds of CPU per process startup.

Three concrete operational problems this leaves unsolved:

- **Build-once, deploy-many.** A central build job that loads the
  authoritative rule source, validates it, and produces a single
  artifact every consumer process can mmap-equivalent into a
  pre-built engine. Today the only way to share is the source
  CSV/JSON, and every consumer re-pays the build cost.
- **Reproducibility across machines.** "Why is the same input
  matching `rule-47` on staging but `rule-42` on prod?" The answer
  is buried in whether the two processes loaded byte-identical
  sources. A diffable artifact representing the *compiled* state
  surfaces the divergence directly.
- **Offline diagnosis.** `Engine.Diagnose` (v0.14.0) needs a live
  engine to run against. With a snapshot file, the analyzer runs
  against any saved engine state — pull yesterday's prod snapshot
  into a dev tool, ask what shadowed what.

Four design questions.

### 1. What goes into a snapshot?

Three candidate scopes:

- **(a) Source-level.** Re-emit the original CSV/JSON. Useful for
  versioning, useless for skipping the build cost — consumers still
  parse + validate.
- **(b) Match-level.** The typed `parser.Condition` AST per rule,
  in insertion order. Consumers re-execute `AddRule`. Skips parsing
  but not validation or bucket-building.
- **(c) Compiled-level.** The internal `snapshot{buckets,
  keysetOrder, rulesInOrder, postFilterHook}` shape, serialized
  verbatim. Consumers skip everything.

Pick **(b)**. The match-level encoding gives the build-once /
diff / offline-Diagnose benefits without locking the adapter's
internal layout into a public contract. (c) is tempting for
absolute startup-time wins, but every change to the bucket shape
(ADR-0036 §2's per-field interval index is one likely candidate)
would either break old snapshots or force forward-compat shims
forever. (b) holds the contract at the `parser.Condition` AST —
which is already the stable public surface — and lets the bucket
layout evolve freely. Re-running `AddRule` per rule is fast
(microseconds per rule); the parse cost we save is the dominant
piece, not the bucket-build.

### 2. What about non-serializable rule fields?

`indexed.Rule` carries `Action` and `ActionContext` callbacks (Go
funcs) and `Description` / `Tags` (strings). Funcs cannot round-trip
through JSON.

Three options:

- **(a) Refuse to export rules with callbacks** (ErrSnapshotCallbacksPresent).
  Loud but limits the feature to match-only rule sets.
- **(b) Drop callbacks silently on export.** Quiet — fails badly
  when the loaded engine doesn't fire the expected action.
- **(c) Drop callbacks on export, re-attach at Load via a registry
  keyed by rule name.** Caller code owns the action implementations;
  the snapshot owns the matching shape.

Pick **(c)**. This is the operational contract that maps to real
deployments: action code lives in the binary, rules live in the
artifact. `LoadSnapshot` accepts a `map[string]RuleCallbacks` and
attaches the matching pair to each rule by name. Rules whose name
is absent from the map load without callbacks (legal — that rule
becomes a no-op-on-match, useful for engines used purely for
matched-name reporting).

`Description` and `Tags` are pure data — they round-trip cleanly.

### 3. How does the format handle hook-classified conditions?

`Engine.WithPostFilterHook(h)` (v0.11.0) lets callers admit
caller-defined `parser.Condition` types as post-filters. The
adapter never sees the concrete type at AddRule time beyond
"hook said yes"; the hook itself is a Go func that can't be
encoded.

Two options:

- **(a) Refuse to export hook-bearing engines** (ErrSnapshotIncompatibleHook).
  Restricts the feature to engines using only built-in shapes
  (StringCondition, SetCondition, RangeCondition).
- **(b) Define a registration protocol where callers pre-register
  named encoders/decoders for their custom Condition types** and
  the snapshot carries `"type": "custom:<name>", "payload": ...`.
  Forward-compat-extensible but a much wider surface.

Pick **(a)** for v0.15.0. The built-in shapes cover the dominant
use case; the custom-condition protocol is its own ADR with
worked examples from real consumers. Refuse-with-clear-error
beats half-built encoding.

### 4. How is the format versioned?

Strict integer `FormatVersion` on the `Snapshot` struct. `LoadSnapshot`
rejects any value other than the current `SnapshotFormatVersion`
with `ErrSnapshotFormatVersionMismatch`. No migration shims, no
"try to read the old shape" — refuse, loud and clear.

Same-version compat across patch releases is the contract. Minor
releases may bump the version if the AST gains a new Condition
shape (e.g., StringRangeCondition); v0.15.0 ships
`SnapshotFormatVersion = 1`.

This is the safe default: the moment a consumer needs cross-version
compatibility a future ADR adds a migration protocol. v0.15.0
ships the minimum viable shape.

## Decision

Add to `engine/indexed`:

```go
// SnapshotFormatVersion identifies the on-disk schema. LoadSnapshot
// refuses snapshots with any other value.
const SnapshotFormatVersion = 1

// Snapshot is the JSON-serializable representation of a built
// engine's rule set. Capture order matters: rules are loaded back
// in the same order they were originally added (first-match
// semantics depend on insertion order).
type Snapshot struct {
    FormatVersion int            `json:"formatVersion"`
    Rules         []SnapshotRule `json:"rules"`
}

// SnapshotRule is a serialized rule: name + description + tags +
// the typed Match AST. Action and ActionContext are not encoded;
// callers re-attach them at Load via the rebuild map.
type SnapshotRule struct {
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Tags        []string          `json:"tags,omitempty"`
    Match       SnapshotCondition `json:"match"`
}

// SnapshotCondition is a tagged-union encoding of parser.Condition.
// Type is one of: "string", "set", "range", "and". The indexed
// adapter only accepts these four shapes; Or / Not are rejected at
// AddRule time and never appear in a snapshot.
//
// Min and Max are encoded as decimal strings to support IEEE-754
// infinity values, which encoding/json refuses to marshal.
type SnapshotCondition struct {
    Type     string              `json:"type"`
    Field    string              `json:"field,omitempty"`
    Op       string              `json:"op,omitempty"`
    Value    string              `json:"value,omitempty"`
    Values   []string            `json:"values,omitempty"`
    Min      string              `json:"min,omitempty"`
    Max      string              `json:"max,omitempty"`
    Children []SnapshotCondition `json:"children,omitempty"`
}

// RuleCallbacks carries the per-rule action callbacks that
// LoadSnapshot re-attaches by name.
type RuleCallbacks struct {
    Action        func(input interface{}) interface{}
    ActionContext func(ctx context.Context, input interface{}) interface{}
}

// ExportSnapshot serializes the engine's rule set. Triggers
// implicit Build if not yet built. Returns:
//   - ErrSnapshotIncompatibleHook if WithPostFilterHook was installed.
//   - ErrSnapshotEmpty if the engine has no rules.
//   - ErrSnapshotUnsupportedCondition if any rule's match contains
//     a shape outside the four supported tagged-union variants
//     (theoretically unreachable for a built engine, but checked
//     defensively).
func (e *Engine) ExportSnapshot() (*Snapshot, error)

// LoadSnapshot reconstructs an engine. Returns:
//   - ErrSnapshotFormatVersionMismatch if snap.FormatVersion != current.
//   - ErrSnapshotMalformed if a SnapshotCondition fails to decode
//     (unknown type, missing required field, unparseable Min/Max).
//   - The same AddRule errors if a rule fails validation
//     (ErrEmptyRuleName, ErrDuplicateRuleName, ErrNonIndexableCondition, ...).
//
// rebuild may be nil. Rules whose name is absent from rebuild load
// without callbacks. The returned engine is already Built and ready
// to Execute.
func LoadSnapshot(snap *Snapshot, rebuild map[string]RuleCallbacks) (*Engine, error)
```

### Encoding rules

- `parser.StringCondition{Field: f, Op: op, Value: v}` →
  `{type: "string", field: f, op: op, value: v}`.
- `parser.SetCondition{Field: f, Op: op, Values: vs}` →
  `{type: "set", field: f, op: op, values: vs}`.
- `parser.RangeCondition{Field: f, Min: lo, Max: hi}` →
  `{type: "range", field: f, min: strconv.FormatFloat(lo, 'g', -1, 64), max: ...}`.
  Infinity bounds emit `"+Inf"` / `"-Inf"`.
- `parser.AndCondition{Children: cs}` →
  `{type: "and", children: [...]}` (recursive).

Pointer variants (`*StringCondition` etc.) marshal as their value
form. The encoder dereferences before classifying.

Or / Not conditions never reach the encoder: a rule containing
them would have been rejected at `AddRule` time with
`ErrNonIndexableCondition`, so a built engine never carries them.
The encoder treats them as `ErrSnapshotUnsupportedCondition` for
defense.

### Decoding rules

`LoadSnapshot` walks each `SnapshotRule` and builds an
`indexed.Rule` to feed back through `AddRule`:

1. Decode the `SnapshotCondition` tree into a `parser.Condition`.
2. Look up `RuleCallbacks` in `rebuild[rule.Name]` if non-nil.
3. Construct `indexed.Rule{Name, Description, Tags, Match, Action,
   ActionContext}` and call `AddRule`.

After every rule is added, call `Build()` on the loaded engine.
Any error short-circuits and returns immediately — partial loads
are not supported (a failure mid-load would leave the caller with
an engine in an indeterminate state; failing the whole load and
returning the error is the right contract).

### File-format example

A two-rule engine with one equality rule and one range rule
serializes as:

```json
{
  "formatVersion": 1,
  "rules": [
    {
      "name": "br-domestic",
      "description": "Brazilian domestic",
      "tags": ["geo"],
      "match": {
        "type": "and",
        "children": [
          {"type": "string", "field": "country", "op": "==", "value": "BR"},
          {"type": "range", "field": "amount", "min": "100", "max": "+Inf"}
        ]
      }
    },
    {
      "name": "eu-set",
      "match": {"type": "set", "field": "country", "op": "IN", "values": ["DE", "FR", "ES"]}
    }
  ]
}
```

### Tests

Standard battery in `engine/indexed/snapshot_test.go`:

- Empty engine → ErrSnapshotEmpty.
- Hook-bearing engine → ErrSnapshotIncompatibleHook.
- Round-trip for each of the four condition shapes (single-shape rules).
- Round-trip for a multi-condition AndCondition.
- Round-trip with RangeCondition at every infinity combination
  (`-Inf`/`+Inf` both bounds, mixed, finite both).
- Round-trip preserves Description and Tags (and that Tags is a
  fresh slice, not aliased with the original).
- Callbacks attach via the rebuild map; rules absent from the map
  load without callbacks; Execute still matches and returns the
  default no-callback output.
- Insertion-order preservation: load → Execute matches the same
  rule as the original engine.
- FormatVersion mismatch → ErrSnapshotFormatVersionMismatch.
- Malformed conditions (unknown type, unparseable Min, etc.) →
  ErrSnapshotMalformed.
- JSON round-trip: marshal → unmarshal → LoadSnapshot produces a
  byte-identical engine state (verified via behavioral equivalence,
  not internal-state comparison).
- Snapshot of a snapshot (Export → Load → Export) produces an
  identical JSON encoding.

### Cookbook section

A new "Build-once, deploy-many with snapshots" entry:

- Show ExportSnapshot in a build-job context, writing JSON to disk.
- Show LoadSnapshot in a consumer process, with the rebuild map
  attaching real callbacks by rule name.
- Note the hook restriction and the callback-by-name contract.
- Cross-reference Diagnose: "run Diagnose against the loaded
  engine to catch dead rules before the consumer process serves
  traffic."

## Consequences

### Closed by v0.15.0

- Build-once, deploy-many: a build process produces a single
  snapshot artifact; consumer processes load it and skip every
  parse / validate / canonicalize cost.
- Operationally-diffable compiled state: two consumers on
  different machines can diff their snapshot files directly to
  reason about behavioral divergence.
- Offline Diagnose: a snapshot loaded into a dev tool can be
  Diagnose'd against the same rules production runs without
  attaching to a live engine.

### Still open after v0.15.0

- **Hook-bearing engines.** Custom typed Condition serialization
  requires a named encoder/decoder protocol; lands in a follow-up
  ADR with worked examples.
- **Cross-version migration.** Today: refuse-on-mismatch. If a real
  consumer needs to load v1 snapshots in a binary at
  SnapshotFormatVersion=2, a future ADR adds a migration helper.
- **Compiled-level encoding.** Worth revisiting if startup-time
  microbenchmarks show that AddRule replay dominates real
  workloads; today's hypothesis is that it doesn't, but
  measurement on a 10k-rule engine will settle it.
- **Snapshot-format compatibility with `engine/firstmatch` /
  `engine/priority` / `engine/inmemory`.** Those adapters store
  opaque closures, not typed Conditions, so they have no path to
  this feature. A future ADR could add a typed-rule constructor
  to them as a precursor.

### Performance impact

- ExportSnapshot is O(N × depth-of-Match). For a 10k-rule, depth-3
  engine: ~30k tree-node visits, single-millisecond total. Run at
  build time, not request time.
- LoadSnapshot is O(N × AddRule-cost). Each rule replays the
  AddRule path (shape extraction + canonicalization + bucket
  insertion). Same per-rule cost as constructing the engine
  imperatively, minus the source-parse cost. The win is
  proportional to the parse + validate cost of the original
  source format.
- Per-Execute hot path is unchanged. Snapshot is build-time
  machinery; loaded engines Execute identically to imperatively-
  built ones.

### Memory impact

- Snapshot structs hold one `SnapshotCondition` tree per rule.
  Comparable to the in-memory `parser.Condition` tree (interface
  pointers vs. struct fields; comparable allocation count).
- JSON marshaling produces one `[]byte` per export; callers manage
  the buffer.

### Validation strategy

- Unit tests covering every condition shape, every error path,
  every infinity-bound combination, every malformed-input case.
- Round-trip equivalence tests: original engine and loaded engine
  must match identically across a fixed input table.
- A pre-tag external scientific test exports a 100-rule mixed
  engine, writes JSON to a temp file, loads it in a fresh process
  context, and verifies behavioral parity against a reference
  in-process engine.

### What this validates for v0.16.0+

The tagged-union JSON encoding is the first "external contract"
shape in `engine/indexed` beyond the Go-level Engine surface.
Future ADRs that extend the parser's Condition vocabulary
(StringRangeCondition, hierarchical conditions, etc.) must
either fit into the existing tagged-union slot or motivate a
SnapshotFormatVersion bump. The version policy is established
here.

The hook-as-blocker decision documents the boundary: until the
named-encoder protocol lands, the snapshot feature and the
custom-condition feature are mutually exclusive. Engines pick
one or the other.
