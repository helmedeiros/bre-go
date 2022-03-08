package firstmatch

import (
	"errors"
	"fmt"
)

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("firstmatch: rule name must not be empty")

// ErrNilCondition is returned by AddRule when Rule.Condition is nil.
var ErrNilCondition = errors.New("firstmatch: rule condition must not be nil")

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered.
var ErrDuplicateRuleName = errors.New("firstmatch: rule name already registered")

// ActionPanicError is returned by Execute when a rule's Action panicked.
// The adapter recovered the panic; the matched rule name is in
// Result.Matched, but its Action did not complete.
type ActionPanicError struct {
	Rule  string
	Value interface{}
}

// Error implements the error interface.
func (e *ActionPanicError) Error() string {
	return fmt.Sprintf("firstmatch: action of rule %q panicked: %v", e.Rule, e.Value)
}

// RuleName returns the name of the rule whose Action panicked.
func (e *ActionPanicError) RuleName() string { return e.Rule }
