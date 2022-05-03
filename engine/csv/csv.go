// Package csv provides a CSV-backed engine.RuleConfigProvider. One
// Loader[RC] type covers the file-path and io.Reader cases via two
// constructors; the per-format column-to-field mapping is supplied
// by the caller as a LineParser closure.
package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"

	"github.com/helmedeiros/bre-go/engine"
)

// LineParser converts a row's column values into a typed RuleConfig.
// Called once per non-skipped row; return a non-nil error to abort
// loading with a LoadError carrying the row number.
type LineParser[RC engine.RuleConfig] func(columns []string) (RC, error)

// Loader reads a CSV source and produces []RC via the parser. Satisfies
// engine.RuleConfigProvider[RC].
type Loader[RC engine.RuleConfig] struct {
	path     string
	reader   io.Reader
	skipRows int
	comma    rune
	parser   LineParser[RC]
}

// NewLoader builds a Loader that reads from the file at path. The file
// is opened lazily on the first RuleConfigs call.
func NewLoader[RC engine.RuleConfig](path string, parser LineParser[RC]) *Loader[RC] {
	return &Loader[RC]{path: path, parser: parser, comma: ','}
}

// NewLoaderFromReader builds a Loader that reads from r. Useful for
// embed.FS, HTTP bodies, or tests with strings.NewReader.
func NewLoaderFromReader[RC engine.RuleConfig](r io.Reader, parser LineParser[RC]) *Loader[RC] {
	return &Loader[RC]{reader: r, parser: parser, comma: ','}
}

// SkipHeader sets the number of leading rows to skip before applying
// the parser. Default 0. Chainable.
func (l *Loader[RC]) SkipHeader(rows int) *Loader[RC] {
	l.skipRows = rows
	return l
}

// Comma sets the field delimiter. Default ','. Chainable.
func (l *Loader[RC]) Comma(c rune) *Loader[RC] {
	l.comma = c
	return l
}

// RuleConfigs reads the source, parses each row, and returns the
// collected configs. Returns a *LoadError on any failure (file open,
// CSV parse, per-row parser error). Implements engine.RuleConfigProvider.
func (l *Loader[RC]) RuleConfigs() ([]RC, error) {
	src, closer, err := l.open()
	if err != nil {
		return nil, &LoadError{Path: l.path, Row: 0, Err: err}
	}
	if closer != nil {
		defer closer()
	}

	reader := csv.NewReader(src)
	reader.Comma = l.comma

	var out []RC
	row := 0
	for {
		row++
		columns, readErr := reader.Read()
		if readErr == io.EOF {
			return out, nil
		}
		if readErr != nil {
			return nil, &LoadError{Path: l.path, Row: row, Err: readErr}
		}
		if row <= l.skipRows {
			continue
		}
		cfg, parseErr := l.parser(columns)
		if parseErr != nil {
			return nil, &LoadError{Path: l.path, Row: row, Err: parseErr}
		}
		out = append(out, cfg)
	}
}

func (l *Loader[RC]) open() (io.Reader, func(), error) {
	if l.reader != nil {
		return l.reader, nil, nil
	}
	f, err := os.Open(l.path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

// LoadError wraps an underlying error with the source path and the
// 1-indexed row where it occurred. Row is 0 for file-level failures
// (open, initial read).
type LoadError struct {
	Path string
	Row  int
	Err  error
}

// Error implements the error interface.
func (e *LoadError) Error() string {
	if e.Row == 0 {
		if e.Path == "" {
			return fmt.Sprintf("csv: load failed: %v", e.Err)
		}
		return fmt.Sprintf("csv: load failed for %q: %v", e.Path, e.Err)
	}
	if e.Path == "" {
		return fmt.Sprintf("csv: row %d: %v", e.Row, e.Err)
	}
	return fmt.Sprintf("csv: %s row %d: %v", e.Path, e.Row, e.Err)
}

// Unwrap supports errors.Is / errors.As against the underlying error.
func (e *LoadError) Unwrap() error { return e.Err }
