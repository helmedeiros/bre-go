package firstmatch_test

import (
	"errors"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/observability"
)

type erroredRecorder struct {
	errors []error
}

func (e *erroredRecorder) OnRuleMatched(observability.Match) {}
func (e *erroredRecorder) OnExecutionStarted(interface{})    {}
func (e *erroredRecorder) OnExecutionFinished(interface{}, interface{}, []string, time.Duration) {
}
func (e *erroredRecorder) OnExecutionErrored(_ interface{}, err error) {
	e.errors = append(e.errors, err)
}

func panickingRule(name string, value interface{}) firstmatch.Rule {
	return firstmatch.Rule{
		Name:      name,
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic(value) },
	}
}

func TestExecuteRecoversFromPanickingAction(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked from Execute: %v", r)
		}
	}()
	_, _ = e.Execute(engine.Request{Input: nil})
}

func TestExecuteReturnsActionPanicError(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	_, err := e.Execute(engine.Request{Input: nil})

	var pe *firstmatch.ActionPanicError
	if !errors.As(err, &pe) {
		t.Fatalf("Execute err: want *ActionPanicError, got %T (%v)", err, err)
	}
}

func TestActionPanicErrorCarriesTheRuleName(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	_, err := e.Execute(engine.Request{Input: nil})

	var pe *firstmatch.ActionPanicError
	_ = errors.As(err, &pe)
	if pe.RuleName() != "boom" {
		t.Fatalf("RuleName: want %q, got %q", "boom", pe.RuleName())
	}
}

func TestExecuteIncludesPanickingRuleInMatched(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("the-one", "different-value"))

	res, _ := e.Execute(engine.Request{Input: nil})

	if len(res.Matched) != 1 || res.Matched[0] != "the-one" {
		t.Fatalf("Matched: want [the-one], got %v", res.Matched)
	}
}

func TestExecuteDoesNotFireOnRuleMatchedForPanickingRule(t *testing.T) {
	counter := &observability.CountingListener{}
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))
	e.AddListener(counter)

	_, _ = e.Execute(engine.Request{Input: nil})

	if counter.Total() != 0 {
		t.Fatalf("CountingListener.Total: want 0 for panicking rule, got %d", counter.Total())
	}
}

func TestActionPanicErrorErrorIncludesRuleAndValue(t *testing.T) {
	e := &firstmatch.ActionPanicError{Rule: "boom", Value: "kaboom"}

	msg := e.Error()

	if msg != `firstmatch: action of rule "boom" panicked: kaboom` {
		t.Fatalf("Error: unexpected message: %q", msg)
	}
}

func TestExecuteFiresOnExecutionFinishedEvenOnPanic(t *testing.T) {
	rec := &erroredRecorder{}
	e := firstmatch.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))
	e.AddListener(rec)

	_, _ = e.Execute(engine.Request{Input: nil})

	if len(rec.errors) != 1 {
		t.Fatalf("OnExecutionErrored: want 1 call, got %d", len(rec.errors))
	}
}
