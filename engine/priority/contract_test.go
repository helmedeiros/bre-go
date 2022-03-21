package priority_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/enginetest"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func TestContract(t *testing.T) {
	enginetest.RunContractTests(t, func(t *testing.T) (engine.Engine, enginetest.SeedFunc) {
		t.Helper()
		eng := priority.New()
		seed := func(name string, match func(interface{}) bool, action func(interface{}) interface{}) error {
			return eng.AddRule(priority.Rule{Name: name, Condition: match, Action: action})
		}
		return eng, seed
	})
}
