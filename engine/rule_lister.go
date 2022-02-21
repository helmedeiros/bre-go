package engine

// RuleLister is an optional interface adapters implement when they can
// enumerate registered rule names. Callers detect support with a type
// assertion against engine.Engine.
//
// The returned slice is a fresh copy, ordered by insertion. A nil or
// empty slice is a valid return (no rules registered).
type RuleLister interface {
	RuleNames() []string
}
