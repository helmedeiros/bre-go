package firstmatch

import "errors"

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("firstmatch: rule name must not be empty")

// ErrNilCondition is returned by AddRule when Rule.Condition is nil.
var ErrNilCondition = errors.New("firstmatch: rule condition must not be nil")

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered.
var ErrDuplicateRuleName = errors.New("firstmatch: rule name already registered")
