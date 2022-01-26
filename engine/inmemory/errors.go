package inmemory

import "errors"

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("inmemory: rule name must not be empty")

// ErrDuplicateRuleName is returned by AddRule when a rule with the same
// name is already registered.
var ErrDuplicateRuleName = errors.New("inmemory: rule name already registered")
