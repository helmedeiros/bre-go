package inmemory

import (
	"errors"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
)

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
// Wraps engine.ErrEmptyRuleName so cross-adapter errors.Is(err,
// engine.ErrEmptyRuleName) checks work.
var ErrEmptyRuleName = fmt.Errorf("inmemory: %w", engine.ErrEmptyRuleName)

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered. Wraps engine.ErrDuplicateRuleName.
var ErrDuplicateRuleName = fmt.Errorf("inmemory: %w", engine.ErrDuplicateRuleName)

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
