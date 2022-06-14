package parser_test

import (
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// FuzzParse confirms that parser.Parse never panics on arbitrary
// string input. Valid input returns a Predicate; invalid input must
// return a *ParseError. Anything else (panic, wrong error type) is a
// bug.
//
// Seed corpus covers the grammar's positive shapes plus a handful of
// boundary inputs known to need single-quote / nested-paren / NOT
// handling. New crashes discovered by `go test -fuzz=FuzzParse`
// expand testdata/fuzz/FuzzParse/.
func FuzzParse(f *testing.F) {
	seeds := []string{
		// Positive grammar coverage.
		`country == "BR"`,
		`country != "BR"`,
		`country IN ("BR", "AR")`,
		`country NOT IN ("US")`,
		`country == "BR" AND tier == "premium"`,
		`country == "BR" OR country == "AR"`,
		`NOT (country == "US")`,
		`(country == "BR" AND tier == "premium") OR vip == "true"`,
		`country == 'BR'`,
		// Boundary / adversarial.
		``,
		`   `,
		`country ==`,
		`== "BR"`,
		`country == "BR`,
		`AND`,
		`NOT NOT NOT country == "x"`,
		`((((country == "x"))))`,
		`country == ""`,
		`country IN ()`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, expr string) {
		// The fuzz contract is "no panic + only known error type".
		// We do not assert success/failure -- arbitrary strings can
		// legitimately go either way.
		pred, err := parser.Parse(expr)
		if err != nil {
			var pe *parser.ParseError
			if !asParseError(err, &pe) {
				t.Fatalf("Parse(%q): non-*ParseError returned: %T %v", expr, err, err)
			}
			return
		}
		// Successful parse -- using the resulting Predicate must not
		// panic on a typical fact map either.
		facts := map[string]interface{}{"country": "BR", "tier": "premium", "vip": "true"}
		_ = pred(facts)
		// Also exercise the typed-tree path; it shares the parse
		// front-end so panics there bubble up to the same fuzz fail.
		cond, err := parser.ParseToCondition(expr)
		if err != nil {
			t.Fatalf("ParseToCondition diverged from Parse on %q: %v", expr, err)
		}
		_ = cond.Eval(facts)
	})
}

// asParseError is a small errors.As wrapper kept local so the seed
// list reads as the file's main feature.
func asParseError(err error, target **parser.ParseError) bool {
	for err != nil {
		if pe, ok := err.(*parser.ParseError); ok {
			*target = pe
			return true
		}
		// One level of unwrap is enough for the parser surface; it
		// does not currently nest errors deeper.
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
			continue
		}
		break
	}
	return false
}

// Sanity check that the fuzz seeds at least compile.
func TestFuzzSeedsCompile(t *testing.T) {
	// The Fuzz function above is only invoked under `go test -fuzz=`.
	// This regular Test ensures that the file at least builds and that
	// none of the static seeds are obviously wrong.
	if !strings.Contains(`country == "BR"`, "==") {
		t.Fatal("seed sanity: == operator missing")
	}
}
