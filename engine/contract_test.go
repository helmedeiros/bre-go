package engine_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

func TestEnginePortIsDefined(t *testing.T) {
	var _ engine.Engine = nilEngine{}
	_ = engine.Request{Input: "fact"}
	_ = engine.Result{Output: "decision", Matched: []string{"rule-a"}}
}

type nilEngine struct{}

func (nilEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, nil
}
