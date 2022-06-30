// Package bench is the shared engine.Engine performance benchmark
// harness. Adapter benchmark code wires its factory and SeedFunc into
// Run, the harness builds a Workload-shaped rule set, and the timer
// runs against the populated engine.
//
// The harness is the sibling of engine/enginetest: that one verifies
// behavior against any engine.Engine, this one measures performance.
// Together they let adapter authors check both correctness and cost
// without writing per-adapter scaffolding.
package bench

import (
	"context"
	"fmt"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

// MatchPosition tells the harness where, in insertion order, the
// targeted matching rule sits.
type MatchPosition int

const (
	// First places the (or the first) matching rule at index 0.
	First MatchPosition = iota
	// Middle places it at Rules/2.
	Middle
	// Last places it at Rules-1.
	Last
	// NoHit leaves the input matching nothing.
	NoHit
)

// Selectivity tells the harness how many rules out of N match the
// input. Position locates the *first* matching rule under Sparse and
// Dense; under Unique, Position locates the sole matching rule.
type Selectivity int

const (
	// Unique: exactly one rule matches.
	Unique Selectivity = iota
	// Sparse: roughly 1% of rules match (at least one).
	Sparse
	// Dense: roughly 50% of rules match.
	Dense
)

// Workload encodes one matrix cell. Zero-value is invalid (Rules and
// Dimensions must be >= 1 in the common case; Populate validates).
//
// OpInDims and OpInValuesPer are v0.9.0+ extensions (ADR-0034) that
// turn the first OpInDims dimensions into set-membership constraints
// instead of plain equality. OpInValuesPer is the number of values
// in each such set; the matching input value is the first value of
// the set, so the rule still matches.
//
// OpNeqDims (v0.10.0+, ADR-0035) turns the LAST OpNeqDims dimensions
// into negation constraints (post-filter in the indexed adapter).
// For the structured factory these become StringCondition{Op:
// OpNeq}; for the closure factory the equivalent set-difference
// is implemented in the closure body. The rule remains matchable:
// the negated value is a synthetic "blocked" value that is not
// equal to the input's matching value.
type Workload struct {
	Rules         int
	Dimensions    int
	Position      MatchPosition
	Selectivity   Selectivity
	OpInDims      int
	OpInValuesPer int
	OpNeqDims     int
}

// BasicMatcher is the canonical workload: 5-dimensional equality
// rules with Sparse selectivity and the matching cluster placed
// Last. Scales by Rules.
func BasicMatcher(rules int) Workload {
	return Workload{
		Rules:       rules,
		Dimensions:  5,
		Position:    Last,
		Selectivity: Sparse,
	}
}

// SeedFunc registers a single rule on the adapter under test.
// Adapter benchmark code wires this to its AddRule, closing over any
// adapter-specific Rule fields (Priority, ActionContext, etc.).
type SeedFunc func(name string, condition func(input interface{}) bool) error

// Factory builds a fresh empty engine and its SeedFunc.
type Factory func() (engine.Engine, SeedFunc)

// Input is the value the harness feeds to Execute. Exported so
// adapter SeedFuncs can read the same key shape when wiring
// adapter-local helpers.
type Input = map[string]string

// Run benchmarks w against the adapter built by factory. Population
// happens outside the timed section (b.ResetTimer is called after
// rules are loaded). Every iteration calls Execute once; the
// benchmark fails fast on a non-nil error.
func Run(b *testing.B, w Workload, factory Factory) {
	b.Helper()
	eng, input, err := setup(w, factory)
	if err != nil {
		b.Fatalf("bench setup: %v", err)
	}
	req := engine.Request{Input: input}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, execErr := eng.Execute(ctx, req); execErr != nil {
			b.Fatalf("Execute: %v", execErr)
		}
	}
}

// Populate registers rules on the adapter via seed and returns the
// input the benchmark should feed. Exposed so unit tests can verify
// the rule shape without driving testing.B.
func Populate(seed SeedFunc, w Workload) (Input, error) {
	if w.Rules < 0 {
		return nil, fmt.Errorf("bench: Rules must be >= 0, got %d", w.Rules)
	}
	if w.Dimensions < 1 {
		return nil, fmt.Errorf("bench: Dimensions must be >= 1, got %d", w.Dimensions)
	}

	matching := matchingIndices(w)
	matchSet := make(map[int]bool, len(matching))
	for _, i := range matching {
		matchSet[i] = true
	}

	input := make(Input, w.Dimensions)
	for d := 0; d < w.Dimensions; d++ {
		input[dimKey(d)] = matchValue(d)
	}

	for i := 0; i < w.Rules; i++ {
		name := fmt.Sprintf("rule-%d", i)
		cond := makeCondition(w, i, matchSet[i])
		if err := seed(name, cond); err != nil {
			return nil, fmt.Errorf("bench: seed rule %d: %w", i, err)
		}
	}

	return input, nil
}

func setup(w Workload, factory Factory) (engine.Engine, Input, error) {
	eng, seed := factory()
	input, err := Populate(seed, w)
	if err != nil {
		return nil, nil, err
	}
	return eng, input, nil
}

// RuleSpec is the structural form of a workload-generated rule. Where
// SeedFunc gives the adapter an opaque condition closure, RuleSpec
// gives it the field-to-value map the closure encodes. Adapters that
// introspect rule shape (engine/indexed onward) use this surface.
//
// KeyValues lists fields the rule constrains under equality.
// InValues (v0.9.0, ADR-0034) lists fields under OpIn set-membership.
// NeqValues (v0.10.0, ADR-0035) lists fields under OpNeq negation
// (post-filter in the indexed adapter). All three maps have
// disjoint keys by construction.
//
// A non-matching rule constrains the same dimensions but shifts one
// field's expected value to a synthetic noise marker; the rule shape
// is unchanged, just the values move.
type RuleSpec struct {
	Name      string
	KeyValues map[string]string
	InValues  map[string][]string
	NeqValues map[string]string
}

// StructuredSeedFunc registers a structurally-described rule on the
// adapter under test. Adapters that need rule shape (indexed,
// hybrid, future) wire this to their typed AddRule.
type StructuredSeedFunc func(spec RuleSpec) error

// StructuredFactory builds a fresh empty engine and its
// StructuredSeedFunc.
type StructuredFactory func() (engine.Engine, StructuredSeedFunc)

// PopulateStructured is the structured counterpart of Populate. Same
// Workload, same Input, but the per-rule callback receives the
// rule's field-to-value map instead of a closure.
func PopulateStructured(seed StructuredSeedFunc, w Workload) (Input, error) {
	if w.Rules < 0 {
		return nil, fmt.Errorf("bench: Rules must be >= 0, got %d", w.Rules)
	}
	if w.Dimensions < 1 {
		return nil, fmt.Errorf("bench: Dimensions must be >= 1, got %d", w.Dimensions)
	}

	matching := matchingIndices(w)
	matchSet := make(map[int]bool, len(matching))
	for _, i := range matching {
		matchSet[i] = true
	}

	input := make(Input, w.Dimensions)
	for d := 0; d < w.Dimensions; d++ {
		input[dimKey(d)] = matchValue(d)
	}

	for i := 0; i < w.Rules; i++ {
		eq, in, neq := makeSpecValues(w, i, matchSet[i])
		spec := RuleSpec{
			Name:      fmt.Sprintf("rule-%d", i),
			KeyValues: eq,
			InValues:  in,
			NeqValues: neq,
		}
		if err := seed(spec); err != nil {
			return nil, fmt.Errorf("bench: seed rule %d: %w", i, err)
		}
	}

	return input, nil
}

// RunStructured benchmarks w against a structured-rule adapter built
// by factory. Mirror of Run for adapters that introspect rule shape.
func RunStructured(b *testing.B, w Workload, factory StructuredFactory) {
	b.Helper()
	eng, seed := factory()
	input, err := PopulateStructured(seed, w)
	if err != nil {
		b.Fatalf("bench setup: %v", err)
	}
	req := engine.Request{Input: input}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, execErr := eng.Execute(ctx, req); execErr != nil {
			b.Fatalf("Execute: %v", execErr)
		}
	}
}

// makeSpecValues mirrors makeCondition's logic but produces the
// structural form: equality, OpIn, and OpNeq maps. The three stay in
// lockstep with the closure form so a rule built from each side
// encodes the same predicate.
//
// Dim layout (left to right by index d):
//   - [0, opInDims):                       OpIn  -> InValues
//   - [opInDims, dims-opNeqDims):          OpEq  -> KeyValues
//   - [dims-opNeqDims, dims):              OpNeq -> NeqValues
//
// Returns (KeyValues, InValues, NeqValues). nil maps are returned
// for empty dim ranges so callers can append children
// unconditionally.
func makeSpecValues(w Workload, ruleIdx int, shouldMatch bool) (map[string]string, map[string][]string, map[string]string) {
	dims, opInDims, opNeqDims := dimLayout(w)

	eq := map[string]string{}
	var in map[string][]string
	var neq map[string]string
	if opInDims > 0 {
		in = make(map[string][]string, opInDims)
	}
	if opNeqDims > 0 {
		neq = make(map[string]string, opNeqDims)
	}

	for d := 0; d < dims; d++ {
		switch {
		case d < opInDims:
			in[dimKey(d)] = makeOpInValues(d, w.OpInValuesPer)
		case d >= dims-opNeqDims:
			// Negate a synthetic value that the input never carries,
			// so the post-filter passes by construction.
			neq[dimKey(d)] = fmt.Sprintf("blocked-d%d", d)
		default:
			eq[dimKey(d)] = matchValue(d)
		}
	}
	if !shouldMatch {
		// Flip dim 0's indexable side to a noise value so the rule
		// drops out at the bucket lookup. OpNeq fields stay as-is;
		// they were never going to fire the mismatch anyway.
		if opInDims > 0 {
			in[dimKey(0)] = []string{fmt.Sprintf("noise-%d-d0", ruleIdx)}
		} else {
			eq[dimKey(0)] = fmt.Sprintf("noise-%d-d0", ruleIdx)
		}
	}
	if len(eq) == 0 {
		eq = nil
	}
	return eq, in, neq
}

// dimLayout returns the trio (dims, opInDims, opNeqDims) clamped so
// the OpIn and OpNeq ranges don't overlap and fit inside [0, dims).
// Equality dims are whatever remains.
func dimLayout(w Workload) (dims, opInDims, opNeqDims int) {
	dims = w.Dimensions
	opInDims = w.OpInDims
	opNeqDims = w.OpNeqDims
	if opInDims < 0 {
		opInDims = 0
	}
	if opNeqDims < 0 {
		opNeqDims = 0
	}
	if opInDims > dims {
		opInDims = dims
	}
	if opInDims+opNeqDims > dims {
		opNeqDims = dims - opInDims
	}
	return dims, opInDims, opNeqDims
}

// makeCondition returns a closure that matches the harness's Input
// across all dims. Non-matching rules differ on dim 0 (mismatch-first
// gives linear adapters their fastest reject path, which keeps the
// linear baseline tight rather than artificially slow).
//
// Dim layout matches makeSpecValues: OpIn dims at the head, OpEq
// dims in the middle, OpNeq dims at the tail.
func makeCondition(w Workload, ruleIdx int, shouldMatch bool) func(input interface{}) bool {
	dims, opInDims, opNeqDims := dimLayout(w)

	eqExpected := make([]string, dims)
	inExpected := make([][]string, opInDims)
	neqExpected := make([]string, dims) // empty string at non-OpNeq slots
	for d := 0; d < dims; d++ {
		switch {
		case d < opInDims:
			inExpected[d] = makeOpInValues(d, w.OpInValuesPer)
		case d >= dims-opNeqDims:
			neqExpected[d] = fmt.Sprintf("blocked-d%d", d)
		default:
			eqExpected[d] = matchValue(d)
		}
	}
	if !shouldMatch {
		if opInDims > 0 {
			inExpected[0] = []string{fmt.Sprintf("noise-%d-d0", ruleIdx)}
		} else {
			eqExpected[0] = fmt.Sprintf("noise-%d-d0", ruleIdx)
		}
	}

	eqStart := opInDims
	eqEnd := dims - opNeqDims
	return func(in interface{}) bool {
		m, ok := in.(Input)
		if !ok {
			return false
		}
		for d := 0; d < opInDims; d++ {
			actual := m[dimKey(d)]
			hit := false
			for _, v := range inExpected[d] {
				if v == actual {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		}
		for d := eqStart; d < eqEnd; d++ {
			if m[dimKey(d)] != eqExpected[d] {
				return false
			}
		}
		for d := eqEnd; d < dims; d++ {
			if m[dimKey(d)] == neqExpected[d] {
				return false
			}
		}
		return true
	}
}

// makeOpInValues returns the OpIn value set for dim d. First entry
// is the matching value (matchValue(d)); the rest are synthetic
// non-matching alternatives so the set has the requested cardinality.
func makeOpInValues(d, n int) []string {
	if n < 1 {
		n = 1
	}
	out := make([]string, n)
	out[0] = matchValue(d)
	for i := 1; i < n; i++ {
		out[i] = fmt.Sprintf("opin-%d-alt%d", d, i)
	}
	return out
}

func matchingIndices(w Workload) []int {
	if w.Rules == 0 || w.Position == NoHit {
		return nil
	}
	switch w.Selectivity {
	case Unique:
		return []int{positionIndex(w)}
	case Sparse:
		return spreadIndices(w, atLeastOne(w.Rules/100))
	case Dense:
		return spreadIndices(w, atLeastOne(w.Rules/2))
	default:
		return []int{positionIndex(w)}
	}
}

func positionIndex(w Workload) int {
	switch w.Position {
	case First:
		return 0
	case Middle:
		return w.Rules / 2
	case Last:
		return w.Rules - 1
	default:
		return 0
	}
}

// spreadIndices clusters k matching rules adjacent to each other,
// starting at Position. Position thus reads as "where the first
// matching rule sits" under Sparse and Dense.
func spreadIndices(w Workload, k int) []int {
	if k > w.Rules {
		k = w.Rules
	}
	start := positionIndex(w)
	if start+k > w.Rules {
		start = w.Rules - k
	}
	if start < 0 {
		start = 0
	}
	out := make([]int, k)
	for i := 0; i < k; i++ {
		out[i] = start + i
	}
	return out
}

func dimKey(d int) string     { return fmt.Sprintf("dim%d", d) }
func matchValue(d int) string { return fmt.Sprintf("match%d", d) }

func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
