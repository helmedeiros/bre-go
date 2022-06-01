# 30. The JSON Loader Sub-Package

## Status

Accepted — landed as the `engine/json` sub-package alongside the
CSV loader from ADR-0024. Ships with v0.7.0. Extends ADR-0023's
`RuleConfigProvider[RC]` abstraction.

## Context

ADR-0023 introduced `engine.RuleConfigProvider[RC]` so that any source
of rule definitions can produce a `[]RC` for `engine.Load`. ADR-0024
landed the first concrete provider — `engine/csv` — and called out that
the pattern was deliberately reusable:

> "When `engine/json` lands (no ADR yet), it gains a `Loader[RC]` with
> the same shape. The parser closure handles JSON-specific decoding;
> the abstraction is identical."

This ADR is that loader. By v0.7.0 the library has shipped four
releases of port-and-adapter discipline (`v0.1.0`–`v0.4.0`), an
expression parser (`v0.5.0`), and a typed Condition tree (`v0.6.0`).
JSON is the second-most-requested rule-definition format after CSV in
the parity target's deployments: ops teams who do not own a
spreadsheet pipeline ship rules as JSON blobs through config-service
or Kubernetes ConfigMaps.

Three questions:

**1. What is the top-level document shape?**

Two reasonable shapes:

- (a) **Array of objects**: `[{...}, {...}, {...}]`. Each element is a
  rule.
- (b) **Object with a named array**: `{"rules": [{...}, {...}]}`. The
  outer object can carry metadata (`"version"`, `"updated_at"`).

Pick (a). The CSV loader treats the file as a flat sequence of rows
with no envelope; the JSON loader should mirror that. Callers who want
an envelope wrap the loader: read the envelope themselves, hand the
inner array to a `NewLoaderFromReader`. This keeps the loader shape
minimal and symmetric with CSV.

NDJSON (one JSON object per line) is **deferred**. It is a streaming
format whose value is amortizing memory across millions of rules — not
a v0.7.0 problem. If we add it, it lands as `engine/json/ndjson` or as
a `Streaming()` toggle on `Loader`. The first cut reads the entire
document into memory (matching CSV's behavior).

**2. What's the per-item callback signature?**

CSV gives the parser a `[]string` and asks for an `RC`. JSON's natural
equivalent is `json.RawMessage`:

```go
type ItemParser[RC engine.RuleConfig] func(item json.RawMessage) (RC, error)
```

`json.RawMessage` is `encoding/json`'s "decode-it-yourself" hook: the
loader has already validated that the document parses as JSON and
isolated the per-rule chunk, but it has not committed to any per-item
schema. The caller's `ItemParser` calls `json.Unmarshal(item, &myRule)`
into whatever struct it wants.

Two alternatives rejected:

- (i) `ItemParser[RC] func(item map[string]interface{}) (RC, error)`.
  Forces every caller to type-assert their way through an
  `interface{}` tree. The whole reason to use JSON is to land in typed
  structs; this signature fights that.
- (ii) Generic decoding directly into `RC`: `func RuleConfigs[RC]()
  ([]RC, error)` with no parser callback. Cleaner in the simple case
  but it presumes the caller's `RC` struct *is* the JSON shape. Real
  configs often have a wire shape (snake_case fields, optional
  metadata, embedded condition strings) that maps to but is not equal
  to the engine-internal `RuleConfig`. The closure lets that mapping
  live in the caller's code where it belongs.

The same reasoning the CSV ADR used. The point of the abstraction is
that the *loader* handles I/O and document structure; the *caller*
handles their format's column-to-field (or field-to-field) mapping.

**3. What goes wrong, and how is it reported?**

Same three buckets as CSV:

- File-not-found / I/O failure.
- Malformed JSON (unterminated string, top-level value is not an
  array, etc.).
- Per-item parsing error (the caller's `ItemParser` returns an error).

A typed `LoadError`:

```go
type LoadError struct {
    Path  string // empty when constructed via NewLoaderFromReader
    Index int    // 0-indexed array position; -1 for document-level errors
    Err   error
}

func (e *LoadError) Error() string
func (e *LoadError) Unwrap() error
```

The `Index` field is 0-indexed (the natural JSON-array convention),
distinguishing it from CSV's `Row` (1-indexed, the natural
spreadsheet convention). Document-level failures (file open, malformed
JSON, top-level value is not an array) carry `Index: -1`. The sentinel
is unambiguous because legitimate per-item indices start at 0.

`LoadError` does not implement any interface declared by
`engine/csv.LoadError`. They are siblings, not subtypes. Callers who
load from multiple formats `errors.As` against each loader's package
type.

## Decision

Add a new sub-package `engine/json` exporting:

```go
type ItemParser[RC engine.RuleConfig] func(item json.RawMessage) (RC, error)

type Loader[RC engine.RuleConfig] struct { /* private fields */ }

func NewLoader[RC engine.RuleConfig](path string, parser ItemParser[RC]) *Loader[RC]
func NewLoaderFromReader[RC engine.RuleConfig](r io.Reader, parser ItemParser[RC]) *Loader[RC]

// engine.RuleConfigProvider satisfaction
func (l *Loader[RC]) RuleConfigs() ([]RC, error)

type LoadError struct {
    Path  string
    Index int
    Err   error
}
func (e *LoadError) Error() string
func (e *LoadError) Unwrap() error
```

No `SkipHeader` or `Comma` equivalents — JSON has no header rows and
no field delimiter to configure. The constructor surface stays minimal
on purpose.

The package depends only on `encoding/json`, `io`, `os`, `fmt`, and
`engine` (for the `RuleConfig` constraint). No third-party
dependencies; no vendoring; implementation expected at ~80 lines plus
tests.

Contract: `Loader[RC]` satisfies `engine.RuleConfigProvider[RC]`.
Wired through a compile-time witness in the loader's test file.

A runnable example in `example_test.go` shows the canonical pattern:
define an `RC` struct, write an `ItemParser` closure that
`json.Unmarshal`s into whatever wire-shape struct the JSON uses (then
maps to `RC`), construct a `Loader`, call `RuleConfigs()`.

## Consequences

The library gains its second declarative rule-loading format. The
public package count grows by one — `engine/json`. The pattern set by
ADR-0024 holds: one Loader type per format, both satisfy
`engine.RuleConfigProvider`, the per-format wire mapping is a caller
closure.

Future loaders (YAML, TOML, HTTP-backed remote configs) keep
following the same shape. If a real caller asks for NDJSON streaming,
that lands as a separate ADR and either a sibling sub-package or a
mode toggle on this Loader.

The choice to keep `engine/csv.LoadError` and `engine/json.LoadError`
as sibling types — rather than promoting a single
`engine.LoadError` — preserves locality: each package keeps its own
position semantics (CSV row index, JSON array index) without forcing a
unified vocabulary that does not quite fit either. A future
cross-loader error interface lands when more than one such loader
ships and a common consumer needs it.
