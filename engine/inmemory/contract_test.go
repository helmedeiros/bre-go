package inmemory_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

// TestContract runs the shared engine.Engine contract test suite
// against this adapter. New behavioral guarantees are added in
// enginetest, not here; this file is the wiring.
func TestContract(t *testing.T) {
	enginetest.RunContractTests(t, func(t *testing.T) (engine.Engine, enginetest.SeedFunc) {
		t.Helper()
		eng := inmemory.New()
		seed := func(name string, match func(interface{}) bool, action func(interface{}) interface{}) error {
			return eng.AddRule(inmemory.Rule{Name: name, Condition: match, Action: action})
		}
		return eng, seed
	})
}
