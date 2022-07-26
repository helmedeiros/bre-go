# 24. The CSV Loader Sub-Package

## Status

Accepted — landed as the engine/csv sub-package alongside ADR-0023's RuleConfigProvider abstraction. Ships with v0.3.0.

## Context

ADR-0023 defined the `RuleConfigProvider[RC]` abstraction every loader satisfies. This ADR specifies the **first concrete loader**: CSV.

CSV is chosen first for three reasons:

1. **Decision tables are the dominant rule-loading shape.** A spreadsheet-shaped file where each row is a rule and each column is a condition or output is the most common rule-definition pattern in real-world BREs.
2. **The format is universally supported** (`encoding/csv` is in the standard library; every spreadsheet tool exports CSV; ops teams already know it).
3. **The shape is well-suited to abstraction.** Different rule types have different column layouts, but the *loading mechanism* (open file, parse header, walk rows, apply column → field mapping, build typed config) is the same. An abstract base class with a per-format `parseLine` hook captures that.

Three design questions:

**1. Where does the package live?**

- (a) `engine/csv` -- sibling to `engine/inmemory`, `engine/firstmatch`, `engine/priority`.
- (b) `engine/load/csv` -- nested under a `load/` umbrella that future loaders share.

Pick (a). One level of nesting matches every other adapter. A `engine/json` or `engine/yaml` later would also sit at the top level. We want consistent depth across the loader sub-packages so callers don't have to memorize which formats live where.

**2. What's the loader's shape?**

The Java side uses an abstract base class:

```java
public abstract class CsvRulesLoader<RC extends RuleConfig> implements RuleConfigProvider<RC> {
    protected abstract RC parseLine(String[] columns) throws RuleConfigLoadException;
    public List<RC> getRulesConfigs() { /* opens file, walks rows, calls parseLine */ }
}
```

Go does not have abstract classes. The Go-idiomatic equivalent is a generic helper that takes the per-format mapping as a function parameter:

```go
type LineParser[RC engine.RuleConfig] func(columns []string) (RC, error)

type Loader[RC engine.RuleConfig] struct {
    path      string
    skipRows  int           // header rows to skip
    parseLine LineParser[RC]
}

func NewLoader[RC engine.RuleConfig](path string, parser LineParser[RC]) *Loader[RC]

func (l *Loader[RC]) RuleConfigs() ([]RC, error) {
    // open file, read with encoding/csv, skip header rows, call parser for each row
}
```

The `Loader` is the concrete provider; the per-format work happens in the caller's `LineParser` closure. No inheritance, no abstract methods -- just a function.

**3. What goes wrong, and how is it reported?**

- File-not-found: wrap and return.
- Malformed CSV (wrong column count, unparseable quotes): wrap and return.
- Per-row parsing error (the caller's `LineParser` returns an error for some row): wrap with the row number and return.

A typed `LoadError`:

```go
type LoadError struct {
    Path string
    Row  int    // 1-indexed; 0 for file-level errors
    Err  error
}

func (e *LoadError) Error() string  { ... }
func (e *LoadError) Unwrap() error  { return e.Err }
```

Callers can `errors.As` it and recover the path / row for log messages. The `Unwrap` lets the underlying error surface through `errors.Is` chains.

A `Skipped` outcome (return a `RC` and a special "skip this row" sentinel from the parser) is **deliberately not in this cut**. Callers who want to filter handle it inside their `LineParser` (return a sentinel they check, or build a wrapper provider that filters). Adding the sentinel to the loader API is feature creep until a real caller asks.

## Decision

Add a new sub-package `engine/csv` exporting:

```go
type LineParser[RC engine.RuleConfig] func(columns []string) (RC, error)

type Loader[RC engine.RuleConfig] struct { /* private fields */ }

func NewLoader[RC engine.RuleConfig](path string, parser LineParser[RC]) *Loader[RC]
func NewLoaderFromReader[RC engine.RuleConfig](r io.Reader, parser LineParser[RC]) *Loader[RC]

// Configuration setters (chainable for ergonomics, but option-pattern is also fine; pick chainable)
func (l *Loader[RC]) SkipHeader(rows int) *Loader[RC]
func (l *Loader[RC]) Comma(c rune) *Loader[RC]

// engine.RuleConfigProvider satisfaction
func (l *Loader[RC]) RuleConfigs() ([]RC, error)

type LoadError struct {
    Path string  // empty when constructed via NewLoaderFromReader
    Row  int
    Err  error
}
func (e *LoadError) Error() string
func (e *LoadError) Unwrap() error
```

Two constructors: `NewLoader(path, parser)` for the file path case (the common one), `NewLoaderFromReader(r, parser)` for testing and for callers feeding from non-file sources (embed.FS, HTTP body, etc.).

The package depends only on `encoding/csv`, `io`, `os`, `errors`, `fmt`, and `engine` (for the `RuleConfig` constraint). No third-party dependencies; no vendoring; the implementation should be ~80 lines plus tests.

Contract: `Loader[RC]` satisfies `engine.RuleConfigProvider[RC]`. Wired through a compile-time witness (`var _ engine.RuleConfigProvider[someRC] = (*Loader[someRC])(nil)`) in the loader's test file.

A runnable example in `example_test.go` shows the canonical pattern: define a `RuleConfig` struct, write a `LineParser` closure, construct a `Loader`, call `RuleConfigs()`.

## Consequences

The library gains the ability to load rules from any CSV source. The eight-package surface grows by one (`engine/csv`). Callers who don't need CSV loading don't import the package -- pure opt-in.

The pattern (one provider type per format, all satisfying `engine.RuleConfigProvider`) sets the template every future loader follows. When `engine/json` lands (no ADR yet), it gains a `Loader[RC]` with the same shape. The parser closure handles JSON-specific decoding; the abstraction is identical.

The `LoadError` shape is intentionally compact. Future fields (line content, raw bytes, character offset within row) land if a real caller needs them. Richer error types exist in adjacent ecosystems; we add fields as they earn their keep, not preemptively.

A future "watch this file for changes and reload" capability lands as a separate ADR -- not as part of this Loader. It would likely live in `engine/csv/watch` or be a wrapping `WatchableProvider[RC]` on top of `Loader[RC]`.
