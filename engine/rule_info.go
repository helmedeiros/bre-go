package engine

// RuleInfo is a port-level snapshot of a registered rule's metadata.
// Adapters that satisfy RuleInfoLister produce these by mapping their
// internal Rule struct's Description and Tags fields.
type RuleInfo struct {
	Name        string
	Description string
	Tags        []string
}

// RuleInfoLister is an optional interface adapters implement when they
// can expose registered rules with metadata. Returns a fresh slice in
// insertion order; mutating the returned slice does not affect engine
// state. Sits alongside RuleLister -- the two are parallel capabilities,
// not v1/v2 of the same one.
type RuleInfoLister interface {
	RuleInfos() []RuleInfo
}
