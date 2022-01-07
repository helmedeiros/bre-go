// Package engine declares the port that every rule-engine adapter
// must satisfy. The package owns the value types that travel across
// the boundary; adapters in sub-packages translate to and from each
// underlying engine.
//
// See docs/architecture/decisions/0003-engine-port.md.
package engine

// Context carries a single execution request from the caller to an
// Engine. The Input is whatever the caller's domain wants to assert
// as facts; until generics arrive (ADR-0004) it is carried as
// interface{} and accessed by adapters through type assertions.
type Context struct {
	Input interface{}
}

// Result carries what an Engine produced for a Context. Empty
// fields mean "no decision"; a populated Output and non-empty
// Matched together describe a successful evaluation.
type Result struct {
	Output  interface{}
	Matched []string
}

// Engine evaluates rules against a Context and returns a Result.
// Implementations live in sub-packages of this one and never
// expose adapter-specific types across the package boundary.
type Engine interface {
	Execute(ctx Context) Result
}
