package priority

import (
	"errors"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
)

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
// Wraps engine.ErrEmptyRuleName.
var ErrEmptyRuleName = fmt.Errorf("priority: %w", engine.ErrEmptyRuleName)

// ErrNilCondition is returned by AddRule when Rule.Condition is nil.
var ErrNilCondition = errors.New("priority: rule condition must not be nil")

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered. Wraps engine.ErrDuplicateRuleName.
var ErrDuplicateRuleName = fmt.Errorf("priority: %w", engine.ErrDuplicateRuleName)

// ActionPanicError is returned by Execute when a rule's Action panicked.
// The adapter recovered the panic; the matched rule name is in
// Result.Matched, but its Action did not complete.
type ActionPanicError struct {
	Rule  string
	Value interface{}
}

// Error implements the error interface.
func (e *ActionPanicError) Error() string {
	return fmt.Sprintf("priority: action of rule %q panicked: %v", e.Rule, e.Value)
}

// RuleName returns the name of the rule whose Action panicked.
func (e *ActionPanicError) RuleName() string { return e.Rule }
