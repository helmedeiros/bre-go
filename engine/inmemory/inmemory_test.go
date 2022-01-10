package inmemory_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func TestNewReturnsAnEngine(t *testing.T) {
	var _ engine.Engine = inmemory.New()
}

func TestEmptyEngineProducesEmptyResult(t *testing.T) {
	e := inmemory.New()

	got := e.Execute(engine.Context{Input: "anything"})

	if got.Output != nil {
		t.Errorf("Output: want nil, got %v", got.Output)
	}
	if len(got.Matched) != 0 {
		t.Errorf("Matched: want empty, got %v", got.Matched)
	}
}

func TestAddRuleAcceptsNamedRule(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{
		Name:      "always-true",
		Condition: func(interface{}) bool { return true },
	})

	if err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
}

func TestAddRuleRejectsEmptyName(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{Name: ""})

	if !errors.Is(err, inmemory.ErrEmptyRuleName) {
		t.Fatalf("AddRule: want ErrEmptyRuleName, got %v", err)
	}
}
