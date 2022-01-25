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

// CountingListener counts matches per rule name. The zero value is usable
// and safe for one execution at a time.
type CountingListener struct {
	counts map[string]int
}

// OnRuleMatched increments the counter for m.Rule.
func (c *CountingListener) OnRuleMatched(m Match) {
	if c.counts == nil {
		c.counts = map[string]int{}
	}
	c.counts[m.Rule]++
}

// Count returns the number of times rule has matched. Unknown rules return 0.
func (c *CountingListener) Count(rule string) int {
	return c.counts[rule]
}

// Total returns the sum of every rule's count.
func (c *CountingListener) Total() int {
	sum := 0
	for _, n := range c.counts {
		sum += n
	}
	return sum
}

// LoggingListener bridges ExecutionListener events to a Logger. It logs
// the rule name only; Input and Output payloads are deliberately left
// off the log line so callers do not accidentally leak PII.
type LoggingListener struct {
	logger Logger
}

// NewLoggingListener returns a listener that calls Info on logger for
// every rule match.
func NewLoggingListener(logger Logger) *LoggingListener {
	return &LoggingListener{logger: logger}
}

// OnRuleMatched logs the rule name through the wrapped Logger.
func (l *LoggingListener) OnRuleMatched(m Match) {
	l.logger.Info("rule matched", String("rule", m.Rule))
}
