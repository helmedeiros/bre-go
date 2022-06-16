package indexed_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest/bench"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// The four cells from BENCHMARKS.md the indexed adapter must clear to
// ship as v0.8.0. Each row codifies one bar in the table; each test
// runs the linear baseline AND the indexed run live, compares
// ratios, and fails the build if the multiplier falls short. Live
// comparison (versus frozen numbers from BENCHMARKS.md) is the
// honest version -- system load, Go version, and hardware all move,
// but the ratio between two adapters on the same machine in the same
// session is stable.

func benchFirstmatch(b *testing.B, w bench.Workload) {
	b.Helper()
	bench.Run(b, w, firstmatchFactory)
}

func benchIndexed(b *testing.B, w bench.Workload) {
	b.Helper()
	bench.RunStructured(b, w, indexedFactory)
}

func firstmatchFactory() (engine.Engine, bench.SeedFunc) {
	e := firstmatch.New()
	seed := func(name string, cond func(input interface{}) bool) error {
		return e.AddRule(firstmatch.Rule{Name: name, Condition: cond})
	}
	return e, seed
}

func indexedFactory() (engine.Engine, bench.StructuredSeedFunc) {
	e := indexed.New()
	seed := func(spec bench.RuleSpec) error {
		children := make([]parser.Condition, 0, len(spec.KeyValues))
		for k, v := range spec.KeyValues {
			children = append(children, parser.StringCondition{Field: k, Op: parser.OpEq, Value: v})
		}
		var match parser.Condition
		if len(children) == 1 {
			match = children[0]
		} else {
			match = parser.AndCondition{Children: children}
		}
		return e.AddRule(indexed.Rule{Name: spec.Name, Match: match})
	}
	return e, seed
}

// assertMultiplier runs both bench callbacks via testing.Benchmark and
// asserts the indexed run is at least minMultiplier x faster than the
// firstmatch run (or at most maxMultiplier x slower, for anti-regression).
//
// Strategy: minMultiplier > 1 means "indexed faster"; maxMultiplier > 1
// with sign reversed means "indexed at most that many times slower."
// We use a single Pass and pass the ratio in by name to keep the test
// reading like the BENCHMARKS.md table.
func assertSpeedup(t *testing.T, name string, w bench.Workload, minRatio float64) {
	t.Helper()
	fm := testing.Benchmark(func(b *testing.B) { benchFirstmatch(b, w) })
	ix := testing.Benchmark(func(b *testing.B) { benchIndexed(b, w) })

	if fm.N == 0 || ix.N == 0 {
		t.Fatalf("%s: benchmarks did not run (fm.N=%d, ix.N=%d)", name, fm.N, ix.N)
	}

	fmNs := float64(fm.NsPerOp())
	ixNs := float64(ix.NsPerOp())
	ratio := fmNs / ixNs

	t.Logf("%s: firstmatch=%.0fns indexed=%.0fns -> indexed is %.1fx faster (bar: >= %.1fx)",
		name, fmNs, ixNs, ratio, minRatio)

	if ratio < minRatio {
		t.Fatalf("%s SUCCESS BAR MISSED: indexed only %.2fx firstmatch, need >= %.2fx (firstmatch=%.0fns indexed=%.0fns)",
			name, ratio, minRatio, fmNs, ixNs)
	}
}

func assertAntiRegression(t *testing.T, name string, w bench.Workload, maxSlowdown float64) {
	t.Helper()
	fm := testing.Benchmark(func(b *testing.B) { benchFirstmatch(b, w) })
	ix := testing.Benchmark(func(b *testing.B) { benchIndexed(b, w) })

	if fm.N == 0 || ix.N == 0 {
		t.Fatalf("%s: benchmarks did not run (fm.N=%d, ix.N=%d)", name, fm.N, ix.N)
	}

	fmNs := float64(fm.NsPerOp())
	ixNs := float64(ix.NsPerOp())
	slowdown := ixNs / fmNs // > 1 means indexed slower

	t.Logf("%s: firstmatch=%.0fns indexed=%.0fns -> indexed slowdown %.2fx (bar: <= %.2fx)",
		name, fmNs, ixNs, slowdown, maxSlowdown)

	if slowdown > maxSlowdown {
		t.Fatalf("%s ANTI-REGRESSION BAR MISSED: indexed is %.2fx slower than firstmatch, need <= %.2fx (firstmatch=%.0fns indexed=%.0fns)",
			name, slowdown, maxSlowdown, fmNs, ixNs)
	}
}

func TestSuccessBar_1k5DimNoHit_AtLeast10x(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf gate under -short")
	}
	w := bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.NoHit, Selectivity: bench.Unique}
	assertSpeedup(t, "1k/5d/NoHit", w, 10.0)
}

func TestSuccessBar_1k5DimLast_AtLeast5x(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf gate under -short")
	}
	w := bench.Workload{Rules: 1000, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique}
	assertSpeedup(t, "1k/5d/Last", w, 5.0)
}

func TestSuccessBar_10k5DimNoHit_AtLeast50x(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf gate under -short")
	}
	w := bench.Workload{Rules: 10000, Dimensions: 5, Position: bench.NoHit, Selectivity: bench.Unique}
	assertSpeedup(t, "10k/5d/NoHit", w, 50.0)
}

func TestSuccessBar_10Rules5DimLast_WithinTwoX(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping perf gate under -short")
	}
	w := bench.Workload{Rules: 10, Dimensions: 5, Position: bench.Last, Selectivity: bench.Unique}
	assertAntiRegression(t, "10/5d/Last", w, 2.0)
}
