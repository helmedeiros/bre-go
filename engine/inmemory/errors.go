package inmemory

import "errors"

// ErrEmptyRuleName is returned by AddRule when the rule's Name is
// empty. Callers compare with errors.Is, not value equality.
var ErrEmptyRuleName = errors.New("inmemory: rule name must not be empty")
