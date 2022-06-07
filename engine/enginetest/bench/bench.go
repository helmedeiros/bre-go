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
type Workload struct {
	Rules       int
	Dimensions  int
	Position    MatchPosition
	Selectivity Selectivity
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
		cond := makeCondition(w.Dimensions, i, matchSet[i])
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

// makeCondition returns a closure that matches the harness's Input
// across all dims. Non-matching rules differ on dim 0 (mismatch-first
// gives linear adapters their fastest reject path, which keeps the
// linear baseline tight rather than artificially slow).
func makeCondition(dims, ruleIdx int, shouldMatch bool) func(input interface{}) bool {
	expected := make([]string, dims)
	for d := 0; d < dims; d++ {
		expected[d] = matchValue(d)
	}
	if !shouldMatch {
		expected[0] = fmt.Sprintf("noise-%d-d0", ruleIdx)
	}
	return func(in interface{}) bool {
		m, ok := in.(Input)
		if !ok {
			return false
		}
		for d := 0; d < dims; d++ {
			if m[dimKey(d)] != expected[d] {
				return false
			}
		}
		return true
	}
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
