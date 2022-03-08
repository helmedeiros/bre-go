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

type erroredRecorder struct {
	erroredCalls []error
}

func (r *erroredRecorder) OnRuleMatched(observability.Match) {}
func (r *erroredRecorder) OnExecutionStarted(interface{})    {}
func (r *erroredRecorder) OnExecutionFinished(interface{}, interface{}, []string, time.Duration) {
}
func (r *erroredRecorder) OnExecutionErrored(_ interface{}, err error) {
	r.erroredCalls = append(r.erroredCalls, err)
}
