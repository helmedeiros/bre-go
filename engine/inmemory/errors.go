package inmemory

import "errors"

// errEmptyRuleName is returned by AddRule when the rule's Name is
// empty. Exported as ErrEmptyRuleName via a sentinel below so
// callers can use errors.Is for behavioral assertions.
var errEmptyRuleName = errors.New("inmemory: rule name must not be empty")

// ErrEmptyRuleName is the public sentinel for the empty-name case.
// Callers should compare with errors.Is, not value equality.
var ErrEmptyRuleName = errEmptyRuleName
