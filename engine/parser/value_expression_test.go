package parser_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine/parser"
)

// ADR-0035 tests: ParseValueExpression maps CSV-shaped value cells
// to typed Conditions.

func TestParseValueExpressionPlainValueIsEquality(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "BR")
	if err != nil {
		t.Fatalf("Parse(BR): %v", err)
	}
	sc, ok := c.(parser.StringCondition)
	if !ok {
		t.Fatalf("want StringCondition, got %T", c)
	}
	if sc.Field != "country" || sc.Op != parser.OpEq || sc.Value != "BR" {
		t.Fatalf("StringCondition: %+v", sc)
	}
}

func TestParseValueExpressionTrimsWhitespace(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "  BR  ")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sc := c.(parser.StringCondition)
	if sc.Value != "BR" {
		t.Fatalf("trimmed value: want BR, got %q", sc.Value)
	}
}

func TestParseValueExpressionNegation(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "!BR")
	if err != nil {
		t.Fatalf("Parse(!BR): %v", err)
	}
	sc, ok := c.(parser.StringCondition)
	if !ok {
		t.Fatalf("want StringCondition, got %T", c)
	}
	if sc.Op != parser.OpNeq || sc.Value != "BR" {
		t.Fatalf("Negation: %+v", sc)
	}
}

func TestParseValueExpressionNegationTrimsOperand(t *testing.T) {
	c, _ := parser.ParseValueExpression("country", "!  BR  ")
	sc := c.(parser.StringCondition)
	if sc.Value != "BR" {
		t.Fatalf("negation trim: want BR, got %q", sc.Value)
	}
}

func TestParseValueExpressionAlternatives(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "BR|AR|CL")
	if err != nil {
		t.Fatalf("Parse(BR|AR|CL): %v", err)
	}
	sc, ok := c.(parser.SetCondition)
	if !ok {
		t.Fatalf("want SetCondition, got %T", c)
	}
	if sc.Op != parser.OpIn || len(sc.Values) != 3 {
		t.Fatalf("alternatives: %+v", sc)
	}
}

func TestParseValueExpressionAlternativesTrimsEachValue(t *testing.T) {
	c, _ := parser.ParseValueExpression("country", " BR | AR | CL ")
	sc := c.(parser.SetCondition)
	for i, want := range []string{"BR", "AR", "CL"} {
		if sc.Values[i] != want {
			t.Fatalf("Values[%d]: want %s, got %q", i, want, sc.Values[i])
		}
	}
}

func TestParseValueExpressionWildcardReturnsNil(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "*")
	if err != nil {
		t.Fatalf("Parse(*): %v", err)
	}
	if c != nil {
		t.Fatalf("wildcard: want nil, got %v", c)
	}
}

func TestParseValueExpressionEmptyReturnsNil(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "")
	if err != nil {
		t.Fatalf("Parse(empty): %v", err)
	}
	if c != nil {
		t.Fatalf("empty: want nil, got %v", c)
	}
}

func TestParseValueExpressionWhitespaceOnlyReturnsNil(t *testing.T) {
	c, err := parser.ParseValueExpression("country", "   ")
	if err != nil {
		t.Fatalf("Parse(whitespace): %v", err)
	}
	if c != nil {
		t.Fatalf("whitespace-only: want nil, got %v", c)
	}
}

// ----- Error cases ----------------------------------------------------

func TestParseValueExpressionMixedNegationAndAlternativesRejected(t *testing.T) {
	_, err := parser.ParseValueExpression("country", "!BR|AR")
	var vee *parser.ValueExpressionError
	if !errors.As(err, &vee) {
		t.Fatalf("want *ValueExpressionError, got %T (%v)", err, err)
	}
	if vee.Field != "country" || vee.Value != "!BR|AR" {
		t.Fatalf("error metadata: %+v", vee)
	}
}

func TestParseValueExpressionEmptyNegationOperandRejected(t *testing.T) {
	_, err := parser.ParseValueExpression("country", "!")
	var vee *parser.ValueExpressionError
	if !errors.As(err, &vee) {
		t.Fatalf("want *ValueExpressionError, got %T (%v)", err, err)
	}
}

func TestParseValueExpressionEmptyNegationOperandAfterTrimRejected(t *testing.T) {
	_, err := parser.ParseValueExpression("country", "!   ")
	var vee *parser.ValueExpressionError
	if !errors.As(err, &vee) {
		t.Fatalf("want *ValueExpressionError, got %T (%v)", err, err)
	}
}

func TestParseValueExpressionEmptyAlternativeRejected(t *testing.T) {
	cases := []string{"BR||AR", "|BR", "BR|"}
	for _, in := range cases {
		_, err := parser.ParseValueExpression("country", in)
		var vee *parser.ValueExpressionError
		if !errors.As(err, &vee) {
			t.Fatalf("Parse(%q): want *ValueExpressionError, got %T (%v)", in, err, err)
		}
	}
}

// ----- ValueExpressionError surface -----------------------------------

func TestValueExpressionErrorMessageIncludesFieldValueCause(t *testing.T) {
	err := &parser.ValueExpressionError{Field: "country", Value: "!BR|AR", Cause: "mixed"}
	msg := err.Error()
	for _, want := range []string{"country", "!BR|AR", "mixed"} {
		if !contains(msg, want) {
			t.Fatalf("Error() should include %q, got %q", want, msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
