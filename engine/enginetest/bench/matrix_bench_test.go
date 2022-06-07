package bench_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest/bench"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/priority"
)

// adapterFactories returns one Factory per built-in adapter. The
// matrix benchmarks iterate this list as a sub-benchmark, so each
// matrix cell produces three lines of output -- one per adapter --
// directly comparable side-by-side under benchstat.
func adapterFactories() []struct {
	name    string
	factory bench.Factory
} {
	return []struct {
		name    string
		factory bench.Factory
	}{
		{name: "firstmatch", factory: firstmatchFactory},
		{name: "inmemory", factory: inmemoryFactory},
		{name: "priority", factory: priorityFactory},
	}
}

func firstmatchFactory() (engine.Engine, bench.SeedFunc) {
	e := firstmatch.New()
	seed := func(name string, cond func(input interface{}) bool) error {
		return e.AddRule(firstmatch.Rule{Name: name, Condition: cond})
	}
	return e, seed
}

func priorityFactory() (engine.Engine, bench.SeedFunc) {
	e := priority.New()
	seed := func(name string, cond func(input interface{}) bool) error {
		return e.AddRule(priority.Rule{Name: name, Condition: cond, Priority: 1})
	}
	return e, seed
}

// runMatrix runs w against every adapter as a sub-benchmark.
func runMatrix(b *testing.B, w bench.Workload) {
	b.Helper()
	for _, a := range adapterFactories() {
		a := a
		b.Run(a.name, func(b *testing.B) {
			bench.Run(b, w, a.factory)
		})
	}
}

// The matrix cells are deliberately curated, not the full Cartesian
// product. Each cell is one b.Run line per adapter; benchstat groups
// them when comparing v0.7.1 baselines against a future indexed
// adapter.

// ----- BasicMatcher canonical sizes -------------------------------------

func BenchmarkMatrixBasicMatcher10(b *testing.B)    { runMatrix(b, bench.BasicMatcher(10)) }
func BenchmarkMatrixBasicMatcher100(b *testing.B)   { runMatrix(b, bench.BasicMatcher(100)) }
func BenchmarkMatrixBasicMatcher1000(b *testing.B)  { runMatrix(b, bench.BasicMatcher(1000)) }
func BenchmarkMatrixBasicMatcher10000(b *testing.B) { runMatrix(b, bench.BasicMatcher(10000)) }

// ----- 1k rules / 5 dims / Unique selectivity ---------------------------
// These are the cells the v0.7.1 BENCHMARKS.md success bar refers to.

func BenchmarkMatrix1k5DimUniqueNoHit(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.NoHit, Selectivity: bench.Unique})
}

func BenchmarkMatrix1k5DimUniqueLast(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}

func BenchmarkMatrix1k5DimUniqueFirst(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.First, Selectivity: bench.Unique})
}

// ----- 10k rules / 5 dims / Unique selectivity --------------------------

func BenchmarkMatrix10k5DimUniqueNoHit(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.NoHit, Selectivity: bench.Unique})
}

func BenchmarkMatrix10k5DimUniqueLast(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}

// ----- 10 rules / 5 dims (anti-regression cell) -------------------------
// Indexed adapter must stay within 2x of firstmatch here.

func BenchmarkMatrix10Rules5DimUniqueLast(b *testing.B) {
	runMatrix(b, bench.Workload{Rules: 10, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique})
}
