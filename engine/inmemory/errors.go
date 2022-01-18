package inmemory

import "errors"

// ErrEmptyRuleName is returned by AddRule when Rule.Name is empty.
var ErrEmptyRuleName = errors.New("inmemory: rule name must not be empty")
