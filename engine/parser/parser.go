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
// String literals are double-quoted or single-quoted; the chosen
// delimiter must match on both sides, and the OTHER delimiter inside
// is a literal character (no escape needed). The active delimiter is
// escaped with a leading backslash (\" or \'); a literal backslash
// is \\. Identifiers match [A-Za-z_][A-Za-z0-9_]*. Whitespace
// separates tokens but is otherwise ignored.
//
// Single-quote support exists so expressions can live in CSV columns
// without colliding with CSV's own double-quote quoting convention:
//
//	"origin == 'DE'"   <- inside a CSV cell, no escaping required
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
	cond, err := ParseToCondition(expr)
	if err != nil {
		return nil, err
	}
	return AsPredicate(cond), nil
}

// ParseToCondition compiles expr into a typed Condition tree. The
// returned tree's concrete types (StringCondition, SetCondition,
// AndCondition, OrCondition, NotCondition) can be inspected,
// marshaled, and compared.
func ParseToCondition(expr string) (Condition, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, &ParseError{Pos: p.peek().pos, Message: "trailing tokens after expression"}
	}
	return cond, nil
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
		case c == '"' || c == '\'':
			t, advance, err := readString(s, i, c)
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

func readString(s string, start int, delim byte) (token, int, error) {
	// s[start] is delim (either '"' or '\'')
	var sb strings.Builder
	i := start + 1
	for i < len(s) {
		c := s[i]
		if c == delim {
			return token{kind: tokString, text: sb.String(), pos: start}, i - start + 1, nil
		}
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case delim, '\\':
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

func (p *parser) parseExpr() (Condition, error) { return p.parseOr() }

func (p *parser) parseOr() (Condition, error) {
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
		// Flatten chained ORs into a single OrCondition.
		if or, ok := left.(OrCondition); ok {
			left = OrCondition{Children: append(or.Children, right)}
		} else {
			left = OrCondition{Children: []Condition{left, right}}
		}
	}
	return left, nil
}

func (p *parser) parseAnd() (Condition, error) {
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
		if and, ok := left.(AndCondition); ok {
			left = AndCondition{Children: append(and.Children, right)}
		} else {
			left = AndCondition{Children: []Condition{left, right}}
		}
	}
	return left, nil
}

func (p *parser) parseNot() (Condition, error) {
	if p.peek().kind == tokNot {
		p.advance()
		inner, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		return NotCondition{Child: inner}, nil
	}
	return p.parseAtom()
}

func (p *parser) parseAtom() (Condition, error) {
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

func (p *parser) parseComparison() (Condition, error) {
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
		op := OpEq
		if opTok.kind == tokNeq {
			op = OpNeq
		}
		return StringCondition{Field: field, Op: op, Value: want}, nil
	case tokIn, tokNotIn:
		values, err := p.parseValueList()
		if err != nil {
			return nil, err
		}
		op := OpIn
		if opTok.kind == tokNotIn {
			op = OpNotIn
		}
		return SetCondition{Field: field, Op: op, Values: values}, nil
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

// Predicate factories removed: comparison evaluation now lives on
// the typed Condition methods in condition.go.
