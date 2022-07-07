package parser

import (
	"strconv"
)

// RangeCondition is an inclusive numeric range over Field. Eval
// parses the input field's string value as a float64 and returns
// true iff Min <= value <= Max. Missing fields, non-string values,
// and non-numeric strings all return false.
//
// math.Inf(-1) and math.Inf(+1) are valid bound values for
// half-open intervals (e.g., Min: 100, Max: math.Inf(+1) for
// "at least 100"). Min > Max is a degenerate construction that
// never matches; the type accepts it without complaint so callers
// can build pathological cases for testing without special-casing.
//
// Added in v0.11.0 (ADR-0036) as the first indexable shape with
// non-equality semantics. The indexed adapter recognizes
// RangeCondition as a post-filter; it cannot contribute to the
// bucket key.
type RangeCondition struct {
	Field string
	Min   float64
	Max   float64
}

// Eval implements Condition. Returns false on missing field,
// non-string value, or unparseable numeric string.
func (c RangeCondition) Eval(fact map[string]interface{}) bool {
	got, ok := fact[c.Field]
	if !ok {
		return false
	}
	s, ok := got.(string)
	if !ok {
		return false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return false
	}
	return c.Min <= v && v <= c.Max
}
