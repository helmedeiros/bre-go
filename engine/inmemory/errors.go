package inmemory

import (
	"errors"
	"fmt"
)

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("inmemory: rule name must not be empty")

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered.
var ErrDuplicateRuleName = errors.New("inmemory: rule name already registered")

// ErrNilCondition is returned by AddRule when Rule.Condition is nil.
var ErrNilCondition = errors.New("inmemory: rule condition must not be nil")

// ActionPanicError is returned by Execute when a rule's Action panicked.
// The adapter recovered the panic and stopped execution after that rule.
type ActionPanicError struct {
	Rule  string
	Value interface{}
}

// Error implements the error interface.
func (e *ActionPanicError) Error() string {
	return fmt.Sprintf("inmemory: action of rule %q panicked: %v", e.Rule, e.Value)
}

// RuleName returns the name of the rule whose Action panicked.
func (e *ActionPanicError) RuleName() string { return e.Rule }
