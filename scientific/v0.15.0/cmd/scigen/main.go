// scigen is a deterministic generator for the scientific harness.
// Given a seed and a rule count, it emits a CSV of expression-shaped
// source rules + a JSONL file of test inputs. Same seed -> byte-
// identical output, no matter the host architecture.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/helmedeiros/bre-go/scientific/v0.15.0/schema"
)

var (
	countries = []string{"BR", "AR", "UY", "PY", "CL", "PE", "CO", "MX", "DE", "FR", "ES", "IT", "PT", "NL", "BE", "PL", "US", "CA", "JP", "KR"}
	segments  = []string{"consumer", "smb", "enterprise", "platform"}
	channels  = []string{"web", "mobile", "store", "partner", "api"}
	tiers     = []string{"bronze", "silver", "gold", "platinum", "diamond"}
)

func main() {
	var (
		seed    = flag.Int64("seed", 1, "deterministic seed")
		rules   = flag.Int("rules", 10000, "number of rules to generate")
		inputs  = flag.Int("inputs", 10000, "number of inputs to generate")
		outDir  = flag.String("out", ".", "output directory")
	)
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		die(err)
	}

	rng := rand.New(rand.NewSource(*seed))

	srcRules := genRules(rng, *rules)
	if err := schema.WriteSourceCSV(filepath.Join(*outDir, "rules.csv"), srcRules); err != nil {
		die(err)
	}

	testInputs := genInputs(rng, *inputs)
	if err := schema.WriteInputsJSONL(filepath.Join(*outDir, "inputs.jsonl"), testInputs); err != nil {
		die(err)
	}

	fmt.Printf("scigen: wrote %d rules + %d inputs to %s (seed=%d)\n", *rules, *inputs, *outDir, *seed)
}

func genRules(rng *rand.Rand, n int) []schema.SourceRule {
	out := make([]schema.SourceRule, 0, n)
	for i := 0; i < n; i++ {
		shape := i % 6
		var expr string
		switch shape {
		case 0:
			expr = fmt.Sprintf("country == %q", pick(rng, countries))
		case 1:
			expr = fmt.Sprintf("country IN (%s)", quoteList(samples(rng, countries, 2+rng.Intn(3))))
		case 2:
			expr = fmt.Sprintf("country == %q AND segment == %q", pick(rng, countries), pick(rng, segments))
		case 3:
			expr = fmt.Sprintf("country IN (%s) AND segment IN (%s)",
				quoteList(samples(rng, countries, 2+rng.Intn(3))),
				quoteList(samples(rng, segments, 1+rng.Intn(3))))
		case 4:
			expr = fmt.Sprintf("country == %q AND segment == %q AND channel != %q",
				pick(rng, countries), pick(rng, segments), pick(rng, channels))
		case 5:
			expr = fmt.Sprintf("country IN (%s) AND tier NOT IN (%s)",
				quoteList(samples(rng, countries, 2+rng.Intn(3))),
				quoteList(samples(rng, tiers, 1+rng.Intn(2))))
		}
		out = append(out, schema.SourceRule{
			Name:       fmt.Sprintf("r-%06d", i),
			Expression: expr,
		})
	}
	return out
}

func genInputs(rng *rand.Rand, n int) []schema.Input {
	out := make([]schema.Input, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, schema.Input{Fact: map[string]string{
			"country": pick(rng, countries),
			"segment": pick(rng, segments),
			"channel": pick(rng, channels),
			"tier":    pick(rng, tiers),
		}})
	}
	return out
}

func pick(rng *rand.Rand, xs []string) string { return xs[rng.Intn(len(xs))] }

func samples(rng *rand.Rand, xs []string, k int) []string {
	if k >= len(xs) {
		out := make([]string, len(xs))
		copy(out, xs)
		return out
	}
	idx := rng.Perm(len(xs))[:k]
	out := make([]string, k)
	for i, j := range idx {
		out[i] = xs[j]
	}
	return out
}

func quoteList(xs []string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprintf("%q", x)
	}
	return strings.Join(parts, ", ")
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "scigen:", err)
	os.Exit(1)
}
