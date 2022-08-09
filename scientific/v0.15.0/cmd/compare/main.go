// compare diffs two results.jsonl files line-by-line.
// Exits 0 iff every (id, matched) pair is identical; exits 2 on
// mismatch, 1 on I/O / format errors.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/helmedeiros/bre-go/scientific/v0.15.0/schema"
)

func main() {
	var (
		a = flag.String("a", "", "first results file")
		b = flag.String("b", "", "second results file")
	)
	flag.Parse()

	if *a == "" || *b == "" {
		fmt.Fprintln(os.Stderr, "compare: -a and -b are required")
		os.Exit(1)
	}

	ra, err := schema.ReadResultsJSONL(*a)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compare:", err)
		os.Exit(1)
	}
	rb, err := schema.ReadResultsJSONL(*b)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compare:", err)
		os.Exit(1)
	}

	if len(ra) != len(rb) {
		fmt.Fprintf(os.Stderr, "compare: length mismatch: %s=%d vs %s=%d\n", *a, len(ra), *b, len(rb))
		os.Exit(2)
	}

	mismatches := 0
	for i := range ra {
		if ra[i].ID != rb[i].ID {
			fmt.Fprintf(os.Stderr, "compare: row %d id %d vs %d\n", i, ra[i].ID, rb[i].ID)
			mismatches++
			continue
		}
		if !sameStringSlice(ra[i].Matched, rb[i].Matched) {
			fmt.Fprintf(os.Stderr, "compare: row %d (id=%d): %v vs %v\n", i, ra[i].ID, ra[i].Matched, rb[i].Matched)
			mismatches++
		}
	}

	if mismatches > 0 {
		fmt.Fprintf(os.Stderr, "compare: %d mismatches across %d rows\n", mismatches, len(ra))
		os.Exit(2)
	}
	fmt.Printf("compare: %d rows match byte-for-byte\n", len(ra))
}

func sameStringSlice(a, b []string) bool {
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
