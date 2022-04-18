package inmemory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
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

func panickingRule(name string, value interface{}) inmemory.Rule {
	return inmemory.Rule{
		Name:      name,
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic(value) },
	}
}

func TestExecuteRecoversFromPanickingAction(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked from Execute: %v", r)
		}
	}()
	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})
}

func TestExecuteReturnsActionPanicError(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	_, err := e.Execute(context.Background(), engine.Request{Input: nil})

	var pe *inmemory.ActionPanicError
	if !errors.As(err, &pe) {
		t.Fatalf("Execute err: want *ActionPanicError, got %T (%v)", err, err)
	}
}

func TestActionPanicErrorCarriesTheRuleName(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	_, err := e.Execute(context.Background(), engine.Request{Input: nil})

	var pe *inmemory.ActionPanicError
	_ = errors.As(err, &pe)
	if pe.RuleName() != "boom" {
		t.Fatalf("RuleName: want %q, got %q", "boom", pe.RuleName())
	}
}

func TestActionPanicErrorCarriesThePanicValue(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	_, err := e.Execute(context.Background(), engine.Request{Input: nil})

	var pe *inmemory.ActionPanicError
	_ = errors.As(err, &pe)
	if pe.Value != "kaboom" {
		t.Fatalf("Value: want %q, got %v", "kaboom", pe.Value)
	}
}

func TestExecuteStopsAtFirstPanic(t *testing.T) {
	laterActionRan := false
	e := inmemory.New()
	_ = e.AddRule(panickingRule("first", "boom"))
	_ = e.AddRule(inmemory.Rule{
		Name:      "second",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { laterActionRan = true; return nil },
	})

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if laterActionRan {
		t.Fatalf("second rule's action ran; Execute must stop on first panic")
	}
}

func TestExecuteIncludesPanickingRuleInMatched(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))

	res, _ := e.Execute(context.Background(), engine.Request{Input: nil})

	if len(res.Matched) != 1 || res.Matched[0] != "boom" {
		t.Fatalf("Matched: want [boom], got %v", res.Matched)
	}
}

func TestExecuteDoesNotFireOnRuleMatchedForPanickingRule(t *testing.T) {
	counter := &observability.CountingListener{}
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))
	e.AddListener(counter)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if counter.Total() != 0 {
		t.Fatalf("CountingListener.Total: want 0 for panicking rule, got %d", counter.Total())
	}
}

func TestActionPanicErrorErrorIncludesRuleAndValue(t *testing.T) {
	e := &inmemory.ActionPanicError{Rule: "boom", Value: "kaboom"}

	msg := e.Error()

	if msg != `inmemory: action of rule "boom" panicked: kaboom` {
		t.Fatalf("Error: unexpected message: %q", msg)
	}
}

func TestExecuteFiresOnExecutionErroredForPanic(t *testing.T) {
	rec := &erroredRecorder{}
	e := inmemory.New()
	_ = e.AddRule(panickingRule("boom", "kaboom"))
	e.AddListener(rec)

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if len(rec.errors) != 1 {
		t.Fatalf("OnExecutionErrored: want 1 call, got %d", len(rec.errors))
	}
}
