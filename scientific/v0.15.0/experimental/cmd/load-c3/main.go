package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go-experimental/format"
)

func main() {
	var (
		dir      = flag.String("dir", ".", "directory containing snapshot.c3.bin (and inputs.jsonl if -exec)")
		snap     = flag.String("snapshot", "snapshot.c3.bin", "snapshot filename within -dir")
		trials   = flag.Int("trials", 1, "number of timing trials")
		exec     = flag.Bool("exec", false, "execute inputs.jsonl and emit results-c3.jsonl")
		timings  = flag.String("timings", "", "write per-trial timings (ns, one per line) to this path")
		label    = flag.String("label", "load-c3", "label printed on timing summary line")
	)
	flag.Parse()

	path := filepath.Join(*dir, *snap)
	var lastEngine *indexed.Engine
	if *trials < 1 {
		*trials = 1
	}
	ts := make([]time.Duration, 0, *trials)
	for t := 0; t < *trials; t++ {
		start := time.Now()
		e, err := loadOnce(path)
		dur := time.Since(start)
		if err != nil {
			die(fmt.Errorf("trial %d: %w", t, err))
		}
		if !e.Built() {
			die(fmt.Errorf("trial %d: engine not Built", t))
		}
		ts = append(ts, dur)
		lastEngine = e
	}

	if *timings != "" {
		if err := writeTimings(*timings, ts); err != nil {
			die(err)
		}
	}
	printTimingSummary(*label, ts)

	if *exec {
		inputs, err := format.ReadInputsJSONL(filepath.Join(*dir, "inputs.jsonl"))
		if err != nil {
			die(err)
		}
		results := make([]format.MatchResult, 0, len(inputs))
		ctx := context.Background()
		for i, in := range inputs {
			res, err := lastEngine.Execute(ctx, engine.Request{Input: in.Fact})
			if err != nil {
				die(fmt.Errorf("input %d: %w", i, err))
			}
			results = append(results, format.MatchResult{ID: i, Matched: res.Matched})
		}
		if err := format.WriteResultsJSONL(filepath.Join(*dir, "results-c3.jsonl"), results); err != nil {
			die(err)
		}
	}
}

func loadOnce(path string) (*indexed.Engine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	snap, err := format.DecodeC3(f)
	if err != nil {
		return nil, err
	}
	return indexed.LoadSnapshot(snap, nil)
}

func writeTimings(path string, ts []time.Duration) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, d := range ts {
		if _, err := fmt.Fprintf(f, "%d\n", d.Nanoseconds()); err != nil {
			return err
		}
	}
	return nil
}

func printTimingSummary(label string, ts []time.Duration) {
	if len(ts) == 0 {
		return
	}
	var sum time.Duration
	min, max := ts[0], ts[0]
	for _, d := range ts {
		sum += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	fmt.Printf("%s: trials=%d mean=%s min=%s max=%s\n", label, len(ts), sum/time.Duration(len(ts)), min, max)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "load-c3:", err)
	os.Exit(1)
}
