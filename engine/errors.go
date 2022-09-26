package engine

import "errors"

// ErrEmptyRuleName is the port-level umbrella sentinel for "AddRule was
// called with an empty Rule.Name." Every adapter's adapter-specific
// `ErrEmptyRuleName` wraps this one, so callers can write a single
// `errors.Is(err, engine.ErrEmptyRuleName)` check that works across
// every adapter without importing each adapter's package.
//
// Backward compat: the per-adapter sentinels (inmemory.ErrEmptyRuleName,
// firstmatch.ErrEmptyRuleName, etc.) keep working unchanged. They now
// wrap this one via fmt.Errorf("%s: %w", adapter, engine.ErrEmptyRuleName).
var ErrEmptyRuleName = errors.New("rule name must not be empty")

// ErrDuplicateRuleName is the port-level umbrella sentinel for "AddRule
// was called with a name that's already registered." Same pattern as
// ErrEmptyRuleName: every adapter's per-adapter ErrDuplicateRuleName
// wraps this one.
var ErrDuplicateRuleName = errors.New("rule name already registered")
