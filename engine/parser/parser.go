// Package parser turns expression strings into evaluable Predicates.
// The grammar (minimal subset for decision-table use):
//
//	expr   = or
//	or     = and ("OR" and)*
//	and    = not ("AND" not)*
//	not    = "NOT" atom | atom
//	atom   = "(" expr ")" | comparison
//	cmp    = field op value
//	op     = "==" | "!=" | "IN" | "NOT IN"
//	value  = STRING | "(" STRING ("," STRING)* ")"
//
// String literals are double-quoted with \" and \\ escapes. Identifiers
// match [A-Za-z_][A-Za-z0-9_]*. Whitespace separates tokens but is
// otherwise ignored.
package parser

import (
	"fmt"
	"strings"
)

// Predicate is the evaluated form of a parsed expression. The fact
// map carries field-name -> value pairs that the expression's
// comparisons read.
type Predicate func(fact map[string]interface{}) bool

// ParseError reports a syntax failure with the byte position into the
// original expression.
type ParseError struct {
	Pos     int
	Message string
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	return fmt.Sprintf("parser: %s at offset %d", e.Message, e.Pos)
}

// Parse compiles expr into a Predicate.
func Parse(expr string) (Predicate, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	pred, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, &ParseError{Pos: p.peek().pos, Message: "trailing tokens after expression"}
	}
	return pred, nil
}

// AsCondition wraps a Predicate as a func(interface{}) bool by applying
// factOf to the engine input before evaluating. Drops directly into
// any adapter's Rule.Condition field.
func AsCondition(p Predicate, factOf func(interface{}) map[string]interface{}) func(interface{}) bool {
	return func(in interface{}) bool { return p(factOf(in)) }
}

// ===== tokenizer =====

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokString
	tokEq
	tokNeq
	tokIn
	tokNotIn
	tokAnd
	tokOr
	tokNot
	tokLParen
	tokRParen
	tokComma
)

type token struct {
	kind tokKind
	text string
	pos  int
}

func tokenize(s string) ([]token, error) {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, token{kind: tokLParen, text: "(", pos: i})
			i++
		case c == ')':
			out = append(out, token{kind: tokRParen, text: ")", pos: i})
			i++
		case c == ',':
			out = append(out, token{kind: tokComma, text: ",", pos: i})
			i++
		case c == '=' && i+1 < len(s) && s[i+1] == '=':
			out = append(out, token{kind: tokEq, text: "==", pos: i})
			i += 2
		case c == '!' && i+1 < len(s) && s[i+1] == '=':
			out = append(out, token{kind: tokNeq, text: "!=", pos: i})
			i += 2
		case c == '"':
			t, advance, err := readString(s, i)
			if err != nil {
				return nil, err
			}
			out = append(out, t)
			i += advance
		case isIdentStart(c):
			t, advance := readIdent(s, i)
			out = append(out, t)
			i += advance
		default:
			return nil, &ParseError{Pos: i, Message: fmt.Sprintf("unexpected character %q", c)}
		}
	}
	out = append(out, token{kind: tokEOF, pos: len(s)})
	return out, nil
}

func readString(s string, start int) (token, int, error) {
	// s[start] is '"'
	var sb strings.Builder
	i := start + 1
	for i < len(s) {
		c := s[i]
		if c == '"' {
			return token{kind: tokString, text: sb.String(), pos: start}, i - start + 1, nil
		}
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '"', '\\':
				sb.WriteByte(s[i+1])
				i += 2
				continue
			}
		}
		sb.WriteByte(c)
		i++
	}
	return token{}, 0, &ParseError{Pos: start, Message: "unterminated string literal"}
}

func readIdent(s string, start int) (token, int) {
	i := start
	for i < len(s) && isIdentCont(s[i]) {
		i++
	}
	text := s[start:i]
	switch strings.ToUpper(text) {
	case "AND":
		return token{kind: tokAnd, text: text, pos: start}, i - start
	case "OR":
		return token{kind: tokOr, text: text, pos: start}, i - start
	case "NOT":
		// Look ahead for "NOT IN".
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j+1 < len(s) && (s[j] == 'I' || s[j] == 'i') && (s[j+1] == 'N' || s[j+1] == 'n') &&
			(j+2 == len(s) || !isIdentCont(s[j+2])) {
			return token{kind: tokNotIn, text: "NOT IN", pos: start}, j + 2 - start
		}
		return token{kind: tokNot, text: text, pos: start}, i - start
	case "IN":
		return token{kind: tokIn, text: text, pos: start}, i - start
	}
	return token{kind: tokIdent, text: text, pos: start}, i - start
}

func isIdentStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// ===== parser =====

type parser struct {
	tokens []token
	cur    int
}

func (p *parser) peek() token    { return p.tokens[p.cur] }
func (p *parser) advance() token { t := p.tokens[p.cur]; p.cur++; return t }

func (p *parser) parseExpr() (Predicate, error) { return p.parseOr() }

func (p *parser) parseOr() (Predicate, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(f map[string]interface{}) bool { return l(f) || r(f) }
	}
	return left, nil
}

func (p *parser) parseAnd() (Predicate, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(f map[string]interface{}) bool { return l(f) && r(f) }
	}
	return left, nil
}

func (p *parser) parseNot() (Predicate, error) {
	if p.peek().kind == tokNot {
		p.advance()
		inner, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		return func(f map[string]interface{}) bool { return !inner(f) }, nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (Predicate, error) {
	if p.peek().kind == tokLParen {
		p.advance()
		inner, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, &ParseError{Pos: p.peek().pos, Message: "expected )"}
		}
		p.advance()
		return inner, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (Predicate, error) {
	if p.peek().kind != tokIdent {
		return nil, &ParseError{Pos: p.peek().pos, Message: "expected field identifier"}
	}
	field := p.advance().text

	opTok := p.advance()
	switch opTok.kind {
	case tokEq, tokNeq:
		if p.peek().kind != tokString {
			return nil, &ParseError{Pos: p.peek().pos, Message: "expected string literal"}
		}
		want := p.advance().text
		if opTok.kind == tokEq {
			return makeEq(field, want), nil
		}
		return makeNeq(field, want), nil
	case tokIn, tokNotIn:
		values, err := p.parseValueList()
		if err != nil {
			return nil, err
		}
		if opTok.kind == tokIn {
			return makeIn(field, values), nil
		}
		return makeNotIn(field, values), nil
	default:
		return nil, &ParseError{Pos: opTok.pos, Message: "expected ==, !=, IN, or NOT IN"}
	}
}

func (p *parser) parseValueList() ([]string, error) {
	if p.peek().kind != tokLParen {
		return nil, &ParseError{Pos: p.peek().pos, Message: "expected ( after IN"}
	}
	p.advance()
	var out []string
	for {
		if p.peek().kind != tokString {
			return nil, &ParseError{Pos: p.peek().pos, Message: "expected string literal in value list"}
		}
		out = append(out, p.advance().text)
		if p.peek().kind == tokRParen {
			p.advance()
			return out, nil
		}
		if p.peek().kind != tokComma {
			return nil, &ParseError{Pos: p.peek().pos, Message: "expected , or ) in value list"}
		}
		p.advance()
	}
}

// ===== predicate factories =====

func makeEq(field, want string) Predicate {
	return func(f map[string]interface{}) bool {
		got, ok := f[field]
		if !ok {
			return false
		}
		s, ok := got.(string)
		return ok && s == want
	}
}

func makeNeq(field, want string) Predicate {
	return func(f map[string]interface{}) bool {
		got, ok := f[field]
		if !ok {
			return false
		}
		s, ok := got.(string)
		return ok && s != want
	}
}

func makeIn(field string, values []string) Predicate {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		set[v] = struct{}{}
	}
	return func(f map[string]interface{}) bool {
		got, ok := f[field]
		if !ok {
			return false
		}
		s, ok := got.(string)
		if !ok {
			return false
		}
		_, hit := set[s]
		return hit
	}
}

func makeNotIn(field string, values []string) Predicate {
	in := makeIn(field, values)
	return func(f map[string]interface{}) bool {
		if _, ok := f[field]; !ok {
			return false
		}
		return !in(f)
	}
}
