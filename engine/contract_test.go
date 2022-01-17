// The tests in this file describe the engine.Engine contract.
// Future adapters reuse runContractTests (added when the second
// implementation lands -- one impl cannot define the contract on
// its own).
package engine_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

// TestEnginePortIsDefined fixes the existence and shape of the
// package-level names callers depend on. Behavioral tests arrive
// alongside the first adapter.
func TestEnginePortIsDefined(t *testing.T) {
	var _ engine.Engine = nilEngine{}
	_ = engine.Request{Input: "fact"}
	_ = engine.Result{Output: "decision", Matched: []string{"rule-a"}}
}

// nilEngine is a compile-time witness that engine.Engine is
// implementable. It is intentionally trivial -- the first real
// adapter is engine/inmemory.
type nilEngine struct{}

func (nilEngine) Execute(engine.Request) (engine.Result, error) {
	return engine.Result{}, nil
}
