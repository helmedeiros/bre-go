package observability

// Match is one rule firing as seen by a listener.
type Match struct {
	Rule   string
	Input  interface{}
	Output interface{}
}

// ExecutionListener observes rule matches as they happen.
type ExecutionListener interface {
	OnRuleMatched(m Match)
}

// NopExecutionListener discards every match.
type NopExecutionListener struct{}

// OnRuleMatched on a NopExecutionListener discards the match.
func (NopExecutionListener) OnRuleMatched(Match) {}
