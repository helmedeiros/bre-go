package parser_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/engine/parser"
)

func mustParse(t *testing.T, expr string) parser.Predicate {
	t.Helper()
	p, err := parser.Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", expr, err)
	}
	return p
}

func TestParseEqualityMatchesEqualString(t *testing.T) {
	p := mustParse(t, `origin == "DE"`)
	if !p(map[string]interface{}{"origin": "DE"}) {
		t.Fatalf("== should match equal string")
	}
}

func TestParseEqualityRejectsDifferentString(t *testing.T) {
	p := mustParse(t, `origin == "DE"`)
	if p(map[string]interface{}{"origin": "FR"}) {
		t.Fatalf("== should reject different string")
	}
}

func TestParseEqualityRejectsMissingField(t *testing.T) {
	p := mustParse(t, `origin == "DE"`)
	if p(map[string]interface{}{}) {
		t.Fatalf("== should reject when field absent")
	}
}

func TestParseNotEqualityMatchesDifferentString(t *testing.T) {
	p := mustParse(t, `origin != "DE"`)
	if !p(map[string]interface{}{"origin": "FR"}) {
		t.Fatalf("!= should match different string")
	}
}

func TestParseNotEqualityRejectsEqualString(t *testing.T) {
	p := mustParse(t, `origin != "DE"`)
	if p(map[string]interface{}{"origin": "DE"}) {
		t.Fatalf("!= should reject equal string")
	}
}

func TestParseNotEqualityRejectsMissingField(t *testing.T) {
	p := mustParse(t, `origin != "DE"`)
	if p(map[string]interface{}{}) {
		t.Fatalf("!= on absent field should be false")
	}
}

func TestParseInMatchesAnyValueInList(t *testing.T) {
	p := mustParse(t, `tier IN ("vip", "premium")`)
	if !p(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("IN should match listed value")
	}
}

func TestParseInRejectsValueNotInList(t *testing.T) {
	p := mustParse(t, `tier IN ("vip", "premium")`)
	if p(map[string]interface{}{"tier": "economy"}) {
		t.Fatalf("IN should reject unlisted value")
	}
}

func TestParseInRejectsMissingField(t *testing.T) {
	p := mustParse(t, `tier IN ("vip")`)
	if p(map[string]interface{}{}) {
		t.Fatalf("IN on absent field should be false")
	}
}

func TestParseNotInRejectsListedValue(t *testing.T) {
	p := mustParse(t, `tier NOT IN ("vip", "premium")`)
	if p(map[string]interface{}{"tier": "vip"}) {
		t.Fatalf("NOT IN should reject listed value")
	}
}

func TestParseNotInMatchesValueNotInList(t *testing.T) {
	p := mustParse(t, `tier NOT IN ("vip", "premium")`)
	if !p(map[string]interface{}{"tier": "economy"}) {
		t.Fatalf("NOT IN should match unlisted value")
	}
}

func TestParseNotInRejectsMissingField(t *testing.T) {
	p := mustParse(t, `tier NOT IN ("vip")`)
	if p(map[string]interface{}{}) {
		t.Fatalf("NOT IN on absent field should be false")
	}
}

func TestParseAndShortCircuitsOnFirstFalse(t *testing.T) {
	p := mustParse(t, `a == "x" AND b == "y"`)
	if p(map[string]interface{}{"a": "no", "b": "y"}) {
		t.Fatalf("AND should be false when first operand is false")
	}
}

func TestParseAndRequiresBothTrue(t *testing.T) {
	p := mustParse(t, `a == "x" AND b == "y"`)
	if !p(map[string]interface{}{"a": "x", "b": "y"}) {
		t.Fatalf("AND should be true when both operands are true")
	}
}

func TestParseOrAcceptsEitherTrue(t *testing.T) {
	p := mustParse(t, `a == "x" OR b == "y"`)
	if !p(map[string]interface{}{"a": "no", "b": "y"}) {
		t.Fatalf("OR should be true when one operand is true")
	}
}

func TestParseOrRejectsBothFalse(t *testing.T) {
	p := mustParse(t, `a == "x" OR b == "y"`)
	if p(map[string]interface{}{"a": "no", "b": "no"}) {
		t.Fatalf("OR should be false when both operands are false")
	}
}

func TestParseNotInverts(t *testing.T) {
	p := mustParse(t, `NOT a == "x"`)
	if !p(map[string]interface{}{"a": "y"}) {
		t.Fatalf("NOT should invert a false comparison to true")
	}
}

func TestParseAndBindsTighterThanOr(t *testing.T) {
	// "a==x OR b==y AND c==z" should be a OR (b AND c).
	p := mustParse(t, `a == "x" OR b == "y" AND c == "z"`)
	// a=x, others irrelevant -> true via the OR branch.
	if !p(map[string]interface{}{"a": "x"}) {
		t.Fatalf("a==x should satisfy a OR (b AND c)")
	}
	// a=no, b=y, c=no -> false because the AND requires c=z.
	if p(map[string]interface{}{"a": "no", "b": "y", "c": "no"}) {
		t.Fatalf("a OR (b AND c) should be false when AND branch is false")
	}
}

func TestParseParensOverridePrecedence(t *testing.T) {
	// (a OR b) AND c.
	p := mustParse(t, `(a == "x" OR b == "y") AND c == "z"`)
	if !p(map[string]interface{}{"a": "x", "c": "z"}) {
		t.Fatalf("parens-grouped OR should satisfy left side of AND")
	}
	if p(map[string]interface{}{"a": "x", "c": "no"}) {
		t.Fatalf("AND should fail when right operand is false")
	}
}

func TestParseStringWithEscapedQuote(t *testing.T) {
	p := mustParse(t, `name == "he said \"hi\""`)
	if !p(map[string]interface{}{"name": `he said "hi"`}) {
		t.Fatalf("escaped quote should match literal quote in fact")
	}
}

func TestParseStringWithEscapedBackslash(t *testing.T) {
	p := mustParse(t, `path == "C:\\users"`)
	if !p(map[string]interface{}{"path": `C:\users`}) {
		t.Fatalf("escaped backslash should match single backslash in fact")
	}
}

func TestParseErrorOnUnterminatedString(t *testing.T) {
	_, err := parser.Parse(`a == "missing-end`)
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T (%v)", err, err)
	}
}

func TestParseErrorOnUnknownOperator(t *testing.T) {
	_, err := parser.Parse(`a ?? "x"`)
	if err == nil {
		t.Fatalf("expected error on unknown operator")
	}
}

func TestParseErrorOnMissingClosingParen(t *testing.T) {
	_, err := parser.Parse(`(a == "x"`)
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T (%v)", err, err)
	}
}

func TestParseErrorOnTrailingTokens(t *testing.T) {
	_, err := parser.Parse(`a == "x" garbage`)
	if err == nil {
		t.Fatalf("expected error on trailing tokens")
	}
}

func TestParseErrorOnMissingField(t *testing.T) {
	_, err := parser.Parse(`== "x"`)
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T (%v)", err, err)
	}
}

func TestParseErrorOnMissingValueAfterEq(t *testing.T) {
	_, err := parser.Parse(`a ==`)
	if err == nil {
		t.Fatalf("expected error on missing value")
	}
}

func TestParseErrorMissingOpenParenAfterIn(t *testing.T) {
	_, err := parser.Parse(`a IN "x"`)
	if err == nil {
		t.Fatalf("expected error: IN needs (")
	}
}

func TestParseErrorEmptyValueListClosesEarly(t *testing.T) {
	_, err := parser.Parse(`a IN ()`)
	if err == nil {
		t.Fatalf("expected error on empty value list")
	}
}

func TestParseErrorValueListMissingCommaOrCloseParen(t *testing.T) {
	_, err := parser.Parse(`a IN ("x" "y")`)
	if err == nil {
		t.Fatalf("expected error: missing , between IN values")
	}
}

func TestParseErrorOnUnexpectedCharacter(t *testing.T) {
	_, err := parser.Parse(`a @ "x"`)
	var pe *parser.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ParseError, got %T (%v)", err, err)
	}
}

func TestParseErrorPositionReportsCorrectOffset(t *testing.T) {
	_, err := parser.Parse(`a == "x" garbage`)
	var pe *parser.ParseError
	_ = errors.As(err, &pe)
	if pe == nil {
		t.Fatalf("expected *ParseError")
	}
	if pe.Pos != 9 {
		t.Fatalf("Pos: want 9 (start of 'garbage'), got %d", pe.Pos)
	}
}

func TestParseErrorMessageIncludesPositionAndReason(t *testing.T) {
	pe := &parser.ParseError{Pos: 5, Message: "boom"}
	msg := pe.Error()
	if !strings.Contains(msg, "boom") || !strings.Contains(msg, "5") {
		t.Fatalf("Error message missing details: %q", msg)
	}
}

func TestParseLowercaseKeywordsAccepted(t *testing.T) {
	p := mustParse(t, `a == "x" and b == "y"`)
	if !p(map[string]interface{}{"a": "x", "b": "y"}) {
		t.Fatalf("lowercase 'and' should be accepted")
	}
}

func TestAsConditionWrapsForRuleConditionShape(t *testing.T) {
	p := mustParse(t, `origin == "DE"`)
	factOf := func(in interface{}) map[string]interface{} {
		return map[string]interface{}{"origin": in.(string)}
	}
	cond := parser.AsCondition(p, factOf)
	if !cond("DE") {
		t.Fatalf("AsCondition: want true for matching input")
	}
	if cond("FR") {
		t.Fatalf("AsCondition: want false for non-matching input")
	}
}

func TestParserIntegratesWithInmemoryEngine(t *testing.T) {
	type req struct {
		Origin string
		Tier   string
	}
	factOf := func(in interface{}) map[string]interface{} {
		r := in.(req)
		return map[string]interface{}{"origin": r.Origin, "tier": r.Tier}
	}

	pred, _ := parser.Parse(`origin == "DE" AND tier IN ("vip", "premium")`)
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "from-string-dsl",
		Condition: parser.AsCondition(pred, factOf),
		Action:    func(interface{}) interface{} { return "match" },
	})

	res, err := e.Execute(context.Background(), engine.Request{Input: req{Origin: "DE", Tier: "vip"}})
	if err != nil || res.Output != "match" {
		t.Fatalf("integration: err=%v output=%v", err, res.Output)
	}
}

func TestParseErrorOnRightOperandOfOr(t *testing.T) {
	_, err := parser.Parse(`a == "x" OR garbage`)
	if err == nil {
		t.Fatalf("expected error when OR right operand is malformed")
	}
}

func TestParseErrorOnRightOperandOfAnd(t *testing.T) {
	// Tokenizable but unparseable RHS so the error bubbles through parseAnd.
	_, err := parser.Parse(`a == "x" AND b ==`)
	if err == nil {
		t.Fatalf("expected error when AND right operand is malformed")
	}
}

func TestParseErrorOnNotMalformedAtom(t *testing.T) {
	// `NOT` followed by a bare identifier (no operator) -- parseComparison fails.
	_, err := parser.Parse(`NOT b ==`)
	if err == nil {
		t.Fatalf("expected error when NOT inner atom is malformed")
	}
}

func TestParseErrorOnMalformedInsideParens(t *testing.T) {
	// Tokenizable but unparseable expression inside parens.
	_, err := parser.Parse(`(a ==)`)
	if err == nil {
		t.Fatalf("expected error when paren inner expression is malformed")
	}
}

func TestParseInRejectsNonStringValueInFact(t *testing.T) {
	p := mustParse(t, `tier IN ("vip")`)
	if p(map[string]interface{}{"tier": 42}) {
		t.Fatalf("IN should reject when fact value is not a string")
	}
}

func TestParseErrorOnUnexpectedOperatorAfterField(t *testing.T) {
	// Field followed by a non-comparison operator token (AND directly).
	_, err := parser.Parse(`a AND b == "x"`)
	if err == nil {
		t.Fatalf("expected error when comparison operator missing after field")
	}
}

func TestAsConditionIntegrationRejectsWhenPredicateFalse(t *testing.T) {
	type req struct{ Tier string }
	factOf := func(in interface{}) map[string]interface{} {
		return map[string]interface{}{"tier": in.(req).Tier}
	}

	pred, _ := parser.Parse(`tier == "vip"`)
	cond := parser.AsCondition(pred, factOf)
	if cond(req{Tier: "economy"}) {
		t.Fatalf("AsCondition: want false for non-matching tier")
	}
}
