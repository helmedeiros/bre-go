package priority_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func TestExecuteRecoversFromPanickingAction(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "boom",
		Priority:  10,
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic leaked from Execute: %v", r)
		}
	}()
	_, _ = e.Execute(engine.Request{Input: nil})
}

func TestExecuteReturnsActionPanicError(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	})

	_, err := e.Execute(engine.Request{Input: nil})

	var pe *priority.ActionPanicError
	if !errors.As(err, &pe) {
		t.Fatalf("Execute err: want *ActionPanicError, got %T (%v)", err, err)
	}
}

func TestActionPanicErrorCarriesTheRuleName(t *testing.T) {
	e := priority.New()
	_ = e.AddRule(priority.Rule{
		Name:      "boom",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { panic("kaboom") },
	})

	_, err := e.Execute(engine.Request{Input: nil})

	var pe *priority.ActionPanicError
	_ = errors.As(err, &pe)
	if pe.RuleName() != "boom" {
		t.Fatalf("RuleName: want %q, got %q", "boom", pe.RuleName())
	}
}

func TestActionPanicErrorMessageIncludesRuleAndValue(t *testing.T) {
	pe := &priority.ActionPanicError{Rule: "boom", Value: "kaboom"}

	msg := pe.Error()

	if msg != `priority: action of rule "boom" panicked: kaboom` {
		t.Fatalf("Error: unexpected message: %q", msg)
	}
}
