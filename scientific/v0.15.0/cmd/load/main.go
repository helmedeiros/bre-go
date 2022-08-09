// load is the snapshot-path consumer. It reads snapshot.json,
// LoadSnapshot's it into an indexed.Engine, and times the entire
// path. Optionally runs the input set to produce a results.jsonl.
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
	"github.com/helmedeiros/bre-go/scientific/v0.15.0/schema"
)

func main() {
	var (
		dir       = flag.String("dir", ".", "directory containing snapshot.json (and inputs.jsonl if -exec)")
		snapshot  = flag.String("snapshot", "snapshot.json", "snapshot filename within -dir")
		trials    = flag.Int("trials", 1, "number of timing trials")
		exec      = flag.Bool("exec", false, "after timing, execute inputs.jsonl and emit results-snapshot.jsonl")
		timingOut = flag.String("timings", "", "if set, write per-trial timings (nanoseconds, one per line) to this path")
		label     = flag.String("label", "snapshot-load", "label printed on timing summary line")
	)
	flag.Parse()

	snapPath := filepath.Join(*dir, *snapshot)

	if *trials < 1 {
		*trials = 1
	}

	timings := make([]time.Duration, 0, *trials)
	var lastEngine *indexed.Engine
	for t := 0; t < *trials; t++ {
		start := time.Now()
		e, err := loadFromDisk(snapPath)
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

	if *exec {
		inputs, err := schema.ReadInputsJSONL(filepath.Join(*dir, "inputs.jsonl"))
		if err != nil {
			die(err)
		}
		results, err := runInputs(lastEngine, inputs)
		if err != nil {
			die(err)
		}
		outPath := filepath.Join(*dir, "results-snapshot.jsonl")
		if err := schema.WriteResultsJSONL(outPath, results); err != nil {
			die(err)
		}
		fmt.Printf("load: wrote %d results to results-snapshot.jsonl\n", len(results))
	}
}

func loadFromDisk(path string) (*indexed.Engine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var snap indexed.Snapshot
	if err := json.NewDecoder(f).Decode(&snap); err != nil {
		return nil, err
	}
	return indexed.LoadSnapshot(&snap, nil)
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
	fmt.Fprintln(os.Stderr, "load:", err)
	os.Exit(1)
}
