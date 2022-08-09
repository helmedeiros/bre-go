// build is the baseline source-load path. It reads rules.csv,
// parses each expression with parser.ParseToCondition, AddRule's
// them into an indexed.Engine, Build's the engine, and times the
// entire path. Optionally exports the resulting snapshot and runs
// the input set to produce a results.jsonl.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/scientific/v0.15.0/schema"
)

func main() {
	var (
		dir       = flag.String("dir", ".", "directory containing rules.csv (and inputs.jsonl if -exec)")
		trials    = flag.Int("trials", 1, "number of timing trials; -1 disables timing emission")
		snapshot  = flag.String("snapshot", "", "if set, write the built engine's snapshot to this path")
		exec      = flag.Bool("exec", false, "after timing, execute inputs.jsonl and emit results.jsonl")
		timingOut = flag.String("timings", "", "if set, write per-trial timings (nanoseconds, one per line) to this path")
		label     = flag.String("label", "source-build", "label printed on timing summary line")
	)
	flag.Parse()

	rules, err := schema.ReadSourceCSV(filepath.Join(*dir, "rules.csv"))
	if err != nil {
		die(err)
	}

	timings := make([]time.Duration, 0, *trials)
	var lastEngine *indexed.Engine

	if *trials < 1 {
		*trials = 1
	}
	for t := 0; t < *trials; t++ {
		start := time.Now()
		e, err := buildEngine(rules)
		dur := time.Since(start)
		if err != nil {
			die(fmt.Errorf("trial %d: %w", t, err))
		}
		if !e.Built() {
			die(fmt.Errorf("trial %d: engine not Built", t))
		}
		timings = append(timings, dur)
		lastEngine = e
	}

	if *timingOut != "" {
		if err := writeTimings(*timingOut, timings); err != nil {
			die(err)
		}
	}

	printTimingSummary(*label, timings)

	if *snapshot != "" {
		snap, err := lastEngine.ExportSnapshot()
		if err != nil {
			die(fmt.Errorf("ExportSnapshot: %w", err))
		}
		if err := writeSnapshot(*snapshot, snap); err != nil {
			die(err)
		}
	}

	if *exec {
		inputs, err := schema.ReadInputsJSONL(filepath.Join(*dir, "inputs.jsonl"))
		if err != nil {
			die(err)
		}
		results, err := runInputs(lastEngine, inputs)
		if err != nil {
			die(err)
		}
		if err := schema.WriteResultsJSONL(filepath.Join(*dir, "results-source.jsonl"), results); err != nil {
			die(err)
		}
		fmt.Printf("build: wrote %d results to results-source.jsonl\n", len(results))
	}
}

func buildEngine(rules []schema.SourceRule) (*indexed.Engine, error) {
	e := indexed.New()
	for _, r := range rules {
		c, err := parser.ParseToCondition(r.Expression)
		if err != nil {
			return nil, fmt.Errorf("rule %q: parse %q: %w", r.Name, r.Expression, err)
		}
		if err := e.AddRule(indexed.Rule{Name: r.Name, Match: c}); err != nil {
			return nil, fmt.Errorf("rule %q: AddRule: %w", r.Name, err)
		}
	}
	if err := e.Build(); err != nil {
		return nil, err
	}
	return e, nil
}

func runInputs(e *indexed.Engine, inputs []schema.Input) ([]schema.MatchResult, error) {
	ctx := context.Background()
	out := make([]schema.MatchResult, 0, len(inputs))
	for i, in := range inputs {
		res, err := e.Execute(ctx, engine.Request{Input: in.Fact})
		if err != nil {
			return nil, fmt.Errorf("input %d: %w", i, err)
		}
		out = append(out, schema.MatchResult{ID: i, Matched: res.Matched})
	}
	return out, nil
}

func writeTimings(path string, timings []time.Duration) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, d := range timings {
		if _, err := fmt.Fprintf(f, "%d\n", d.Nanoseconds()); err != nil {
			return err
		}
	}
	return nil
}

func writeSnapshot(path string, snap *indexed.Snapshot) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(snap)
}

func printTimingSummary(label string, timings []time.Duration) {
	if len(timings) == 0 {
		return
	}
	var sum time.Duration
	min, max := timings[0], timings[0]
	for _, d := range timings {
		sum += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	mean := sum / time.Duration(len(timings))
	fmt.Printf("%s: trials=%d mean=%s min=%s max=%s\n", label, len(timings), mean, min, max)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "build:", err)
	os.Exit(1)
}
