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

// ErrNonIndexableCondition is returned by AddRule when Match is not a
// pure conjunction of equality (OpEq) string conditions. See
// ADR-0033 §"What is indexable".
var ErrNonIndexableCondition = errors.New("indexed: match is not a pure conjunction of equality conditions")

// ErrIncompatibleInput is returned by Execute when req.Input cannot
// be projected as a fact map (must be map[string]string or
// map[string]interface{}).
var ErrIncompatibleInput = errors.New("indexed: Execute input must be map[string]string or map[string]interface{}")

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
