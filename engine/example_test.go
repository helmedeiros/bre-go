package engine_test

import (
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func describe(eng engine.Engine) {
	if lister, ok := eng.(engine.RuleLister); ok {
		fmt.Println("rules:", lister.RuleNames())
	}
	if _, ok := eng.(engine.ListenerHost); ok {
		fmt.Println("supports listeners")
	}
}

func ExampleEngine_capabilityDiscovery() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "non-empty",
		Condition: func(in interface{}) bool { return in != nil },
	})
	e.AddListener(observability.NopExecutionListener{})

	describe(e)
	// Output:
	// rules: [non-empty]
	// supports listeners
}
