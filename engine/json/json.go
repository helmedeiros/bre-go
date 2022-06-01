// Package json provides a JSON-backed engine.RuleConfigProvider. The
// document shape is a top-level array of objects; each element is
// passed to the caller's ItemParser as a json.RawMessage so the
// per-format wire-to-engine mapping stays in caller code.
package json

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/helmedeiros/bre-go/engine"
)

// ItemParser converts one JSON-array element into a typed RuleConfig.
// Called once per element; return a non-nil error to abort loading
// with a LoadError carrying the 0-indexed array position.
type ItemParser[RC engine.RuleConfig] func(item json.RawMessage) (RC, error)

// Loader reads a JSON source and produces []RC via the parser.
// Satisfies engine.RuleConfigProvider[RC].
type Loader[RC engine.RuleConfig] struct {
	path   string
	reader io.Reader
	parser ItemParser[RC]
}

// NewLoader builds a Loader that reads from the file at path. The
// file is opened lazily on the first RuleConfigs call.
func NewLoader[RC engine.RuleConfig](path string, parser ItemParser[RC]) *Loader[RC] {
	return &Loader[RC]{path: path, parser: parser}
}

// NewLoaderFromReader builds a Loader that reads from r. Useful for
// embed.FS, HTTP bodies, or tests with strings.NewReader.
func NewLoaderFromReader[RC engine.RuleConfig](r io.Reader, parser ItemParser[RC]) *Loader[RC] {
	return &Loader[RC]{reader: r, parser: parser}
}

// RuleConfigs reads the source, decodes the top-level JSON array,
// and applies the parser to each element. Returns a *LoadError on any
// failure (file open, malformed JSON, top-level value not an array,
// per-item parser error). Implements engine.RuleConfigProvider.
func (l *Loader[RC]) RuleConfigs() ([]RC, error) {
	src, closer, err := l.open()
	if err != nil {
		return nil, &LoadError{Path: l.path, Index: -1, Err: err}
	}
	if closer != nil {
		defer closer()
	}

	var items []json.RawMessage
	if err := json.NewDecoder(src).Decode(&items); err != nil {
		return nil, &LoadError{Path: l.path, Index: -1, Err: err}
	}

	out := make([]RC, 0, len(items))
	for i, item := range items {
		cfg, perr := l.parser(item)
		if perr != nil {
			return nil, &LoadError{Path: l.path, Index: i, Err: perr}
		}
		out = append(out, cfg)
	}
	return out, nil
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
// 0-indexed position in the JSON array where it occurred. Index is
// -1 for document-level failures (file open, malformed JSON,
// top-level value not an array).
type LoadError struct {
	Path  string
	Index int
	Err   error
}

// Error implements the error interface.
func (e *LoadError) Error() string {
	if e.Index < 0 {
		if e.Path == "" {
			return fmt.Sprintf("json: load failed: %v", e.Err)
		}
		return fmt.Sprintf("json: load failed for %q: %v", e.Path, e.Err)
	}
	if e.Path == "" {
		return fmt.Sprintf("json: item %d: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("json: %s item %d: %v", e.Path, e.Index, e.Err)
}

// Unwrap supports errors.Is / errors.As against the underlying error.
func (e *LoadError) Unwrap() error { return e.Err }
