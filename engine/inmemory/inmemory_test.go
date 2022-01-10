package inmemory_test

import (
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
