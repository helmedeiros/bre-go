package observability_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/observability"
)

func TestNopExecutionListenerSatisfiesExecutionListener(t *testing.T) {
	var _ observability.ExecutionListener = observability.NopExecutionListener{}
}

func TestNopExecutionListenerOnRuleMatchedDoesNotPanic(t *testing.T) {
	defer noPanic(t)

	var l observability.ExecutionListener = observability.NopExecutionListener{}
	l.OnRuleMatched(observability.Match{Rule: "any", Input: 1, Output: 2})
}
