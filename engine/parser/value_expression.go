package parser

import (
	"fmt"
	"strings"
)

// valueExpressionEscape is the alternative separator. Used inline
// in ParseValueExpression's grammar definition; declared as a
// constant so the spec is in one place.
const (
	valueAlternativesSeparator = "|"
	valueNegationPrefix        = "!"
	valueWildcard              = "*"
)

// ValueExpressionError is returned by ParseValueExpression when the
// input value cannot be classified into one of the supported shapes.
// Carries the field name and the offending value so log lines name
// the failing rule.
type ValueExpressionError struct {
	Field string
	Value string
	Cause string
}

// Error implements the error interface.
func (e *ValueExpressionError) Error() string {
	return fmt.Sprintf("parser: value-expression error for field %q value %q: %s", e.Field, e.Value, e.Cause)
}

// ParseValueExpression turns a single value-cell string into a typed
// Condition over field. CSV-shaped rule loaders (each cell carries
// the rule's per-dimension constraint as one string) use this helper
// so the operator semantics live in one place.
//
// Recognized shapes:
//
//	"BR"       -> StringCondition{Field: field, Op: OpEq,  Value: "BR"}
//	"!BR"      -> StringCondition{Field: field, Op: OpNeq, Value: "BR"}
//	"BR|AR"    -> SetCondition{Field: field, Op: OpIn, Values: ["BR","AR"]}
//	"*"        -> nil, nil  (wildcard / no constraint)
//	""         -> nil, nil  (empty / no constraint)
//
// Whitespace around values is trimmed. Empty alternatives ("BR||AR"
// or "|BR" or "BR|") return *ValueExpressionError. Mixed forms
// ("!BR|AR") return *ValueExpressionError -- the grammar is
// deliberately conservative; v0.10.0 callers either negate OR list
// alternatives, never both.
//
// Returns (nil, nil) for wildcards / empty so callers can
// unconditionally append to an AndCondition's Children and drop
// nils. See ADR-0035 §3 for the grammar choice.
func ParseValueExpression(field, value string) (Condition, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == valueWildcard {
		return nil, nil
	}

	hasAlternatives := strings.Contains(trimmed, valueAlternativesSeparator)
	hasNegation := strings.HasPrefix(trimmed, valueNegationPrefix)

	if hasNegation && hasAlternatives {
		return nil, &ValueExpressionError{
			Field: field,
			Value: value,
			Cause: "mixed negation and alternatives not supported (negate OR list, not both)",
		}
	}

	if hasNegation {
		// Combination with alternatives is already caught above by
		// the hasNegation && hasAlternatives guard, so the operand
		// is known to be a single token.
		operand := strings.TrimSpace(trimmed[len(valueNegationPrefix):])
		if operand == "" {
			return nil, &ValueExpressionError{
				Field: field,
				Value: value,
				Cause: "negation operand is empty",
			}
		}
		return StringCondition{Field: field, Op: OpNeq, Value: operand}, nil
	}

	if hasAlternatives {
		parts := strings.Split(trimmed, valueAlternativesSeparator)
		values := make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" {
				return nil, &ValueExpressionError{
					Field: field,
					Value: value,
					Cause: "empty value inside alternatives",
				}
			}
			values = append(values, t)
		}
		return SetCondition{Field: field, Op: OpIn, Values: values}, nil
	}

	return StringCondition{Field: field, Op: OpEq, Value: trimmed}, nil
}
