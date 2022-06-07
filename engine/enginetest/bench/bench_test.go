package bench_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest/bench"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

// captureSeed accumulates the registered rules so tests can inspect
// what Populate generated.
type captured struct {
	name string
	cond func(input interface{}) bool
}

func captureSeed() (bench.SeedFunc, *[]captured) {
	var rules []captured
	seed := func(name string, cond func(input interface{}) bool) error {
		rules = append(rules, captured{name: name, cond: cond})
		return nil
	}
	return seed, &rules
}

func TestPopulateRejectsNegativeRules(t *testing.T) {
	seed, _ := captureSeed()
	_, err := bench.Populate(seed, bench.Workload{Rules: -1, Dimensions: 1})
	if err == nil {
		t.Fatalf("Populate: expected error for negative Rules")
	}
}

func TestPopulateRejectsZeroDimensions(t *testing.T) {
	seed, _ := captureSeed()
	_, err := bench.Populate(seed, bench.Workload{Rules: 10, Dimensions: 0})
	if err == nil {
		t.Fatalf("Populate: expected error for Dimensions < 1")
	}
}

func TestPopulateReturnsErrorFromSeed(t *testing.T) {
	sentinel := errors.New("seed boom")
	seed := func(string, func(input interface{}) bool) error { return sentinel }
	_, err := bench.Populate(seed, bench.Workload{Rules: 1, Dimensions: 1, Position: bench.First})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Populate: want wrapped sentinel, got %v", err)
	}
}

func TestPopulateBuildsInputWithRequestedDimensions(t *testing.T) {
	seed, _ := captureSeed()
	input, err := bench.Populate(seed, bench.Workload{Rules: 1, Dimensions: 4, Position: bench.First})
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}
	if len(input) != 4 {
		t.Fatalf("input dims: want 4, got %d (keys=%v)", len(input), input)
	}
	for d := 0; d < 4; d++ {
		key := keyFor(d)
		if _, ok := input[key]; !ok {
			t.Fatalf("input missing key %q", key)
		}
	}
}

func TestPopulateRegistersRequestedRuleCount(t *testing.T) {
	seed, got := captureSeed()
	_, err := bench.Populate(seed, bench.Workload{Rules: 7, Dimensions: 2, Position: bench.First})
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}
	if len(*got) != 7 {
		t.Fatalf("rules registered: want 7, got %d", len(*got))
	}
}

func TestUniqueFirstPlacesMatchAtIndexZero(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       5,
		Dimensions:  2,
		Position:    bench.First,
		Selectivity: bench.Unique,
	})
	if !(*got)[0].cond(input) {
		t.Fatalf("First: rule 0 should match input")
	}
	for i := 1; i < len(*got); i++ {
		if (*got)[i].cond(input) {
			t.Fatalf("First: rule %d unexpectedly matched", i)
		}
	}
}

func TestUniqueMiddlePlacesMatchAtHalf(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       10,
		Dimensions:  2,
		Position:    bench.Middle,
		Selectivity: bench.Unique,
	})
	if !(*got)[5].cond(input) {
		t.Fatalf("Middle: rule 5 should match input")
	}
}

func TestUniqueLastPlacesMatchAtEnd(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       10,
		Dimensions:  2,
		Position:    bench.Last,
		Selectivity: bench.Unique,
	})
	if !(*got)[9].cond(input) {
		t.Fatalf("Last: rule 9 should match input")
	}
}

func TestNoHitProducesNoMatchingRule(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       5,
		Dimensions:  2,
		Position:    bench.NoHit,
		Selectivity: bench.Unique,
	})
	for i, r := range *got {
		if r.cond(input) {
			t.Fatalf("NoHit: rule %d unexpectedly matched", i)
		}
	}
}

func TestSparseGivesAtLeastOneMatchAndUnderTenPercent(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       1000,
		Dimensions:  3,
		Position:    bench.First,
		Selectivity: bench.Sparse,
	})
	matches := countMatching(*got, input)
	if matches < 1 {
		t.Fatalf("Sparse: expected >= 1 match, got 0")
	}
	if matches > 100 {
		t.Fatalf("Sparse: expected ~1%% (<=100), got %d", matches)
	}
}

func TestDenseGivesAboutHalfMatching(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       1000,
		Dimensions:  3,
		Position:    bench.First,
		Selectivity: bench.Dense,
	})
	matches := countMatching(*got, input)
	if matches < 400 || matches > 600 {
		t.Fatalf("Dense: expected ~500 (400-600), got %d", matches)
	}
}

func TestSparseClusterStartsAtPosition(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       100,
		Dimensions:  2,
		Position:    bench.First,
		Selectivity: bench.Sparse,
	})
	if !(*got)[0].cond(input) {
		t.Fatalf("Sparse+First: rule 0 should match")
	}
}

func TestSparseClusterFitsInsideRuleRangeAtLast(t *testing.T) {
	seed, got := captureSeed()
	input, _ := bench.Populate(seed, bench.Workload{
		Rules:       1000,
		Dimensions:  2,
		Position:    bench.Last,
		Selectivity: bench.Sparse,
	})
	if !(*got)[len(*got)-1].cond(input) {
		t.Fatalf("Sparse+Last: last rule should match (cluster shifts inside range)")
	}
}

func TestBasicMatcherIsFiveDimSparseLast(t *testing.T) {
	w := bench.BasicMatcher(100)
	if w.Rules != 100 {
		t.Fatalf("Rules: %d", w.Rules)
	}
	if w.Dimensions != 5 {
		t.Fatalf("Dimensions: %d", w.Dimensions)
	}
	if w.Position != bench.Last {
		t.Fatalf("Position: %v", w.Position)
	}
	if w.Selectivity != bench.Sparse {
		t.Fatalf("Selectivity: %v", w.Selectivity)
	}
}

func TestPopulateAcceptsZeroRules(t *testing.T) {
	seed, got := captureSeed()
	input, err := bench.Populate(seed, bench.Workload{Rules: 0, Dimensions: 1, Position: bench.First})
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}
	if len(*got) != 0 {
		t.Fatalf("rules: want 0, got %d", len(*got))
	}
	if len(input) != 1 {
		t.Fatalf("input: want 1 dim, got %d", len(input))
	}
}

func TestConditionRejectsNonMapInput(t *testing.T) {
	seed, got := captureSeed()
	_, _ = bench.Populate(seed, bench.Workload{
		Rules:       1,
		Dimensions:  2,
		Position:    bench.First,
		Selectivity: bench.Unique,
	})
	if (*got)[0].cond("not-a-map") {
		t.Fatalf("rule condition should reject non-Input values")
	}
}

func TestRunDrivesExecuteAgainstPopulatedEngine(t *testing.T) {
	// Drive Run through the real testing.Benchmark machinery to
	// confirm it wires Populate -> ResetTimer -> Execute correctly.
	result := testing.Benchmark(func(b *testing.B) {
		bench.Run(b, bench.Workload{
			Rules:       10,
			Dimensions:  2,
			Position:    bench.Last,
			Selectivity: bench.Unique,
		}, inmemoryFactory)
	})
	if result.N < 1 {
		t.Fatalf("Run: expected Benchmark to execute at least once, got N=%d", result.N)
	}
}

// Helpers.

func keyFor(d int) string {
	// Mirror the harness's internal naming so tests can read the
	// generated input without exposing the helper externally.
	return "dim" + itoa(d)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func countMatching(rules []captured, input bench.Input) int {
	n := 0
	for _, r := range rules {
		if r.cond(input) {
			n++
		}
	}
	return n
}

func inmemoryFactory() (engine.Engine, bench.SeedFunc) {
	e := inmemory.New()
	seed := func(name string, cond func(input interface{}) bool) error {
		return e.AddRule(inmemory.Rule{Name: name, Condition: cond})
	}
	return e, seed
}

// Smoke check that Execute on the populated engine actually returns.
func TestPopulatedEngineExecutesWithoutError(t *testing.T) {
	eng, seed := inmemoryFactory()
	input, err := bench.Populate(seed, bench.Workload{
		Rules:       5,
		Dimensions:  3,
		Position:    bench.Middle,
		Selectivity: bench.Unique,
	})
	if err != nil {
		t.Fatalf("Populate: %v", err)
	}
	res, err := eng.Execute(context.Background(), engine.Request{Input: input})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Matched) != 1 || res.Matched[0] != "rule-2" {
		t.Fatalf("Matched: want [rule-2], got %v", res.Matched)
	}
}
