package enginetest

import (
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

type lifecycleRecorder struct {
	startedCalls  int
	finishedCalls int
}

func (r *lifecycleRecorder) OnRuleMatched(observability.Match) {}

func (r *lifecycleRecorder) OnExecutionStarted(interface{}) {
	r.startedCalls++
}

func (r *lifecycleRecorder) OnExecutionFinished(interface{}, interface{}, []string, time.Duration) {
	r.finishedCalls++
}
