package indexed_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/enginetest/bench"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// Load-time benchmarks. The Execute-only benchmarks
// (matrix_bench_test.go, success_bar_test.go) deliberately reset
// the timer after rules are populated -- they measure steady-state
// per-request latency. These benchmarks complete the picture by
// timing AddRule itself. See BENCHMARKS.md §"Load-time profile"
// for the takeaways.

func loadFirstmatch(b *testing.B, w bench.Workload) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := firstmatch.New()
		seed := func(name string, cond func(input interface{}) bool) error {
			return e.AddRule(firstmatch.Rule{Name: name, Condition: cond})
		}
		if _, err := bench.Populate(seed, w); err != nil {
			b.Fatalf("Populate: %v", err)
		}
	}
}

func loadIndexed(b *testing.B, w bench.Workload) {
	b.Helper()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := indexed.New()
		seed := func(spec bench.RuleSpec) error {
			children := make([]parser.Condition, 0, len(spec.KeyValues)+len(spec.InValues))
			for k, v := range spec.KeyValues {
				children = append(children, parser.StringCondition{Field: k, Op: parser.OpEq, Value: v})
			}
			for k, vs := range spec.InValues {
				children = append(children, parser.SetCondition{Field: k, Op: parser.OpIn, Values: vs})
			}
			var match parser.Condition
			if len(children) == 1 {
				match = children[0]
			} else {
				match = parser.AndCondition{Children: children}
			}
			return e.AddRule(indexed.Rule{Name: spec.Name, Match: match})
		}
		if _, err := bench.PopulateStructured(seed, w); err != nil {
			b.Fatalf("PopulateStructured: %v", err)
		}
	}
}

// Equality-only loads.
func BenchmarkLoad_Firstmatch_1k5dEq(b *testing.B) {
	loadFirstmatch(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}
func BenchmarkLoad_Indexed_1k5dEq(b *testing.B) {
	loadIndexed(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}

func BenchmarkLoad_Firstmatch_10k5dEq(b *testing.B) {
	loadFirstmatch(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}
func BenchmarkLoad_Indexed_10k5dEq(b *testing.B) {
	loadIndexed(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}

// OpIn loads (2 of 5 dims OpIn with 3 values each => 9x fan-out per rule).
func BenchmarkLoad_Firstmatch_1k5d2OpIn(b *testing.B) {
	loadFirstmatch(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique, OpInDims: 2, OpInValuesPer: 3})
}
func BenchmarkLoad_Indexed_1k5d2OpIn(b *testing.B) {
	loadIndexed(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique, OpInDims: 2, OpInValuesPer: 3})
}

func BenchmarkLoad_Firstmatch_10k5d2OpIn(b *testing.B) {
	loadFirstmatch(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique, OpInDims: 2, OpInValuesPer: 3})
}
func BenchmarkLoad_Indexed_10k5d2OpIn(b *testing.B) {
	loadIndexed(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique, OpInDims: 2, OpInValuesPer: 3})
}
