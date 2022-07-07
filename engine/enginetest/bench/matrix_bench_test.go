package bench_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest/bench"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
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

// IndexedFactory is the structured factory for engine/indexed. It
// converts the harness's RuleSpec into an indexed.Rule whose Match
// is the equivalent typed AndCondition. KeyValues become OpEq
// StringConditions; InValues become OpIn SetConditions (v0.9.0+).
func IndexedFactory() (engine.Engine, bench.StructuredSeedFunc) {
	e := indexed.New()
	seed := func(spec bench.RuleSpec) error {
		match := indexedRuleMatch(spec)
		return e.AddRule(indexed.Rule{Name: spec.Name, Match: match})
	}
	return e, seed
}

// indexedRuleMatch composes an indexed.Rule.Match from a RuleSpec.
// Returns a single condition if only one field is constrained, else
// an AndCondition over all constraints. KeyValues become OpEq;
// InValues become OpIn; NeqValues become OpNeq; RangeBounds
// (v0.11.0, ADR-0036) become RangeCondition.
func indexedRuleMatch(spec bench.RuleSpec) parser.Condition {
	total := len(spec.KeyValues) + len(spec.InValues) + len(spec.NeqValues) + len(spec.RangeBounds)
	children := make([]parser.Condition, 0, total)
	for k, v := range spec.KeyValues {
		children = append(children, parser.StringCondition{Field: k, Op: parser.OpEq, Value: v})
	}
	for k, vs := range spec.InValues {
		children = append(children, parser.SetCondition{Field: k, Op: parser.OpIn, Values: vs})
	}
	for k, v := range spec.NeqValues {
		children = append(children, parser.StringCondition{Field: k, Op: parser.OpNeq, Value: v})
	}
	for k, b := range spec.RangeBounds {
		children = append(children, parser.RangeCondition{Field: k, Min: b[0], Max: b[1]})
	}
	if len(children) == 1 {
		return children[0]
	}
	return parser.AndCondition{Children: children}
}

// runMatrix runs w against every linear adapter as a sub-benchmark,
// plus the indexed adapter via the structured surface. Output of one
// BenchmarkMatrixXxx call thus contains four lines -- one per adapter
// -- directly comparable under benchstat.
func runMatrix(b *testing.B, w bench.Workload) {
	b.Helper()
	for _, a := range adapterFactories() {
		a := a
		b.Run(a.name, func(b *testing.B) {
			bench.Run(b, w, a.factory)
		})
	}
	b.Run("indexed", func(b *testing.B) {
		bench.RunStructured(b, w, IndexedFactory)
	})
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
