package indexed

import (
	"errors"
	"fmt"
)

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("indexed: rule name must not be empty")

// ErrNilMatch is returned by AddRule when Rule.Match is nil.
var ErrNilMatch = errors.New("indexed: rule match must not be nil")

// ErrDuplicateRuleName is returned by AddRule when a rule with the
// same name is already registered.
var ErrDuplicateRuleName = errors.New("indexed: rule name already registered")

// ErrNonIndexableCondition is returned by AddRule when Match contains
// a shape the engine does not recognize as indexable or as a valid
// post-filter (e.g. a SetCondition with an unsupported Op, an empty
// value list, a duplicate field across the conjunction, or a shape
// outside the StringCondition / SetCondition / AndCondition family).
// See ADR-0033 / ADR-0035.
var ErrNonIndexableCondition = errors.New("indexed: match contains a shape the indexed adapter does not understand")

// ErrNoIndexableTerms is returned by AddRule when Match contains
// only non-indexable terms (e.g. pure-negation rules like
// `country != BR`). Each rule must have at least one OpEq or OpIn
// term so the engine can bucket it. Pure-negation shapes are
// slated for the IndexDimension framework in v0.11.0; today they
// must use one of the linear adapters. See ADR-0035 §2.
var ErrNoIndexableTerms = errors.New("indexed: match has no indexable terms (at least one OpEq or OpIn required)")

// ErrIncompatibleInput is returned by Execute when req.Input cannot
// be projected as a fact map (must be map[string]string or
// map[string]interface{}).
var ErrIncompatibleInput = errors.New("indexed: Execute input must be map[string]string or map[string]interface{}")

// FanoutTooLargeError is returned by AddRule when a rule's OpIn
// expansion would produce more bucket entries than maxFanout. See
// ADR-0034 §"What about the OpIn empty-set edge?". Caller fixes
// the rule (reduce OpIn cardinality, split into multiple rules)
// rather than the engine eating unbounded memory.
type FanoutTooLargeError struct {
	Rule        string
	Cardinality int
	Limit       int
}

// Error implements the error interface.
func (e *FanoutTooLargeError) Error() string {
	return fmt.Sprintf("indexed: rule %q fan-out %d exceeds limit %d", e.Rule, e.Cardinality, e.Limit)
}

// RuleName returns the name of the rejected rule.
func (e *FanoutTooLargeError) RuleName() string { return e.Rule }

// ActionPanicError is returned by Execute when a rule's Action panicked.
// The adapter recovered the panic; the matched rule name is in
// Result.Matched, but its Action did not complete.
type ActionPanicError struct {
	Rule  string
	Value interface{}
}

// Error implements the error interface.
func (e *ActionPanicError) Error() string {
	return fmt.Sprintf("indexed: action of rule %q panicked: %v", e.Rule, e.Value)
}

// RuleName returns the name of the rule whose Action panicked.
func (e *ActionPanicError) RuleName() string { return e.Rule }
