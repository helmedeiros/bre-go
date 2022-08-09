// adversarial constructs corner-case rules programmatically (deep
// AndCondition nesting, unicode names, all-Inf ranges, mixed
// pointer/value shapes) and verifies they round-trip Export -> Load
// -> Execute against a fixed adversarial input table. Exits 0 on
// full equivalence, 2 on any mismatch.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

type advCase struct {
	name   string
	rule   indexed.Rule
	inputs []map[string]string
}

func main() {
	var (
		dump = flag.String("dump", "", "if set, write the snapshot JSON used during the run here (for inspection)")
	)
	flag.Parse()

	cases := buildCases()
	totalChecks := 0
	failures := 0
	for _, c := range cases {
		orig := indexed.New()
		if err := orig.AddRule(c.rule); err != nil {
			fmt.Fprintf(os.Stderr, "%s: AddRule: %v\n", c.name, err)
			failures++
			continue
		}
		snap, err := orig.ExportSnapshot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: ExportSnapshot: %v\n", c.name, err)
			failures++
			continue
		}
		raw, err := json.Marshal(snap)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: Marshal: %v\n", c.name, err)
			failures++
			continue
		}
		if *dump != "" {
			f, ferr := os.OpenFile(*dump, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if ferr == nil {
				fmt.Fprintf(f, "=== %s ===\n%s\n", c.name, raw)
				f.Close()
			}
		}
		var reread indexed.Snapshot
		if err := json.Unmarshal(raw, &reread); err != nil {
			fmt.Fprintf(os.Stderr, "%s: Unmarshal: %v\n", c.name, err)
			failures++
			continue
		}
		loaded, err := indexed.LoadSnapshot(&reread, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: LoadSnapshot: %v\n", c.name, err)
			failures++
			continue
		}
		for i, in := range c.inputs {
			totalChecks++
			ro, err := orig.Execute(context.Background(), engine.Request{Input: in})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: input %d orig Execute: %v\n", c.name, i, err)
				failures++
				continue
			}
			rl, err := loaded.Execute(context.Background(), engine.Request{Input: in})
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: input %d loaded Execute: %v\n", c.name, i, err)
				failures++
				continue
			}
			if !sameSlice(ro.Matched, rl.Matched) {
				fmt.Fprintf(os.Stderr, "%s: input %d (%v): orig=%v loaded=%v\n", c.name, i, in, ro.Matched, rl.Matched)
				failures++
			}
		}
	}

	fmt.Printf("adversarial: cases=%d checks=%d failures=%d\n", len(cases), totalChecks, failures)
	if failures > 0 {
		os.Exit(2)
	}
}

func buildCases() []advCase {
	var cases []advCase

	cases = append(cases, advCase{
		name: "deep-and-depth-10",
		rule: indexed.Rule{
			Name:  "deep",
			Match: nestAnd(10),
		},
		inputs: []map[string]string{
			{"f": "v"},
			{"f": "other"},
			{},
		},
	})

	cases = append(cases, advCase{
		name: "unicode-rule-name",
		rule: indexed.Rule{
			Name:  "regra-brasileira-ção-☃-\U0001F1E7\U0001F1F7",
			Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
		},
		inputs: []map[string]string{
			{"country": "BR"},
			{"country": "AR"},
		},
	})

	cases = append(cases, advCase{
		name: "unicode-field-and-value",
		rule: indexed.Rule{
			Name:  "u",
			Match: parser.StringCondition{Field: "país", Op: parser.OpEq, Value: "Brésił"},
		},
		inputs: []map[string]string{
			{"país": "Brésił"},
			{"país": "other"},
		},
	})

	cases = append(cases, advCase{
		name: "range-finite-inclusive",
		rule: indexed.Rule{
			Name: "r",
			Match: parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				parser.RangeCondition{Field: "n", Min: 0, Max: 0},
			}},
		},
		inputs: []map[string]string{
			{"k": "v", "n": "0"},
			{"k": "v", "n": "-0.0001"},
			{"k": "v", "n": "0.0001"},
		},
	})

	cases = append(cases, advCase{
		name: "range-neg-inf-to-finite",
		rule: indexed.Rule{
			Name: "r",
			Match: parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				parser.RangeCondition{Field: "n", Min: math.Inf(-1), Max: 100},
			}},
		},
		inputs: []map[string]string{
			{"k": "v", "n": "-1e308"},
			{"k": "v", "n": "100"},
			{"k": "v", "n": "100.01"},
		},
	})

	cases = append(cases, advCase{
		name: "range-finite-to-pos-inf",
		rule: indexed.Rule{
			Name: "r",
			Match: parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				parser.RangeCondition{Field: "n", Min: 100, Max: math.Inf(+1)},
			}},
		},
		inputs: []map[string]string{
			{"k": "v", "n": "99.99"},
			{"k": "v", "n": "100"},
			{"k": "v", "n": "1e308"},
		},
	})

	cases = append(cases, advCase{
		name: "range-both-inf",
		rule: indexed.Rule{
			Name: "r",
			Match: parser.AndCondition{Children: []parser.Condition{
				parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				parser.RangeCondition{Field: "n", Min: math.Inf(-1), Max: math.Inf(+1)},
			}},
		},
		inputs: []map[string]string{
			{"k": "v", "n": "0"},
			{"k": "v", "n": "-1e308"},
			{"k": "v", "n": "1e308"},
		},
	})

	cases = append(cases, advCase{
		name: "pointer-shapes",
		rule: indexed.Rule{
			Name: "r",
			Match: parser.AndCondition{Children: []parser.Condition{
				&parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
				&parser.SetCondition{Field: "t", Op: parser.OpIn, Values: []string{"a", "b"}},
				&parser.RangeCondition{Field: "n", Min: 0, Max: 100},
			}},
		},
		inputs: []map[string]string{
			{"k": "v", "t": "a", "n": "50"},
			{"k": "v", "t": "c", "n": "50"},
		},
	})

	cases = append(cases, advCase{
		name: "huge-set-fanout",
		rule: indexed.Rule{
			Name:  "big",
			Match: parser.SetCondition{Field: "k", Op: parser.OpIn, Values: stringRange("v", 500)},
		},
		inputs: []map[string]string{
			{"k": "v0"},
			{"k": "v499"},
			{"k": "v500"},
		},
	})

	cases = append(cases, advCase{
		name: "value-with-special-chars",
		rule: indexed.Rule{
			Name:  "spec",
			Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "\"quote\" \\ \\n\\t"},
		},
		inputs: []map[string]string{
			{"k": "\"quote\" \\ \\n\\t"},
			{"k": "other"},
		},
	})

	cases = append(cases, advCase{
		name: "description-and-tags-preserved",
		rule: indexed.Rule{
			Name:        "r",
			Description: strings.Repeat("X", 1024),
			Tags:        []string{"a", "b", "c", "d", "e", "f", "g", "h"},
			Match:       parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
		},
		inputs: []map[string]string{{"k": "v"}, {"k": "other"}},
	})

	return cases
}

func nestAnd(depth int) parser.Condition {
	if depth <= 1 {
		return parser.StringCondition{Field: "f", Op: parser.OpEq, Value: "v"}
	}
	return parser.AndCondition{Children: []parser.Condition{nestAnd(depth - 1)}}
}

func stringRange(prefix string, n int) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return out
}

func sameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
