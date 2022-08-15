package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go-experimental/format"
)

func main() {
	var (
		dir  = flag.String("dir", ".", "directory containing rules.csv (and inputs.jsonl if -exec)")
		out  = flag.String("out", "snapshot.c5.bin", "snapshot output filename within -dir")
		exec = flag.Bool("exec", false, "execute inputs.jsonl and emit results-c5-source.jsonl")
	)
	flag.Parse()

	rules, err := format.ReadSourceCSV(filepath.Join(*dir, "rules.csv"))
	if err != nil {
		die(err)
	}
	e := indexed.New()
	for _, r := range rules {
		c, err := parser.ParseToCondition(r.Expression)
		if err != nil {
			die(fmt.Errorf("rule %q: %w", r.Name, err))
		}
		if err := e.AddRule(indexed.Rule{Name: r.Name, Match: c}); err != nil {
			die(fmt.Errorf("rule %q: %w", r.Name, err))
		}
	}
	if err := e.Build(); err != nil {
		die(err)
	}
	cs, err := e.ExportCompiledSnapshot()
	if err != nil {
		die(err)
	}
	f, err := os.Create(filepath.Join(*dir, *out))
	if err != nil {
		die(err)
	}
	if err := format.EncodeC5(f, cs); err != nil {
		_ = f.Close()
		die(err)
	}
	if err := f.Close(); err != nil {
		die(err)
	}

	if *exec {
		inputs, err := format.ReadInputsJSONL(filepath.Join(*dir, "inputs.jsonl"))
		if err != nil {
			die(err)
		}
		results := make([]format.MatchResult, 0, len(inputs))
		ctx := context.Background()
		for i, in := range inputs {
			res, err := e.Execute(ctx, engine.Request{Input: in.Fact})
			if err != nil {
				die(fmt.Errorf("input %d: %w", i, err))
			}
			results = append(results, format.MatchResult{ID: i, Matched: res.Matched})
		}
		if err := format.WriteResultsJSONL(filepath.Join(*dir, "results-c5-source.jsonl"), results); err != nil {
			die(err)
		}
	}
	fmt.Printf("build-c5: wrote %s (%d rules, %d buckets)\n", *out, len(cs.RulesInOrder), len(cs.Buckets))
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "build-c5:", err)
	os.Exit(1)
}
