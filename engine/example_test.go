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
	if infoLister, ok := eng.(engine.RuleInfoLister); ok {
		for _, info := range infoLister.RuleInfos() {
			fmt.Printf("  %s -- %s\n", info.Name, info.Description)
		}
	}
	if _, ok := eng.(engine.ListenerHost); ok {
		fmt.Println("supports listeners")
	}
}

func ExampleEngine_capabilityDiscovery() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:        "non-empty",
		Description: "input must not be nil",
		Condition:   func(in interface{}) bool { return in != nil },
	})
	e.AddListener(observability.NopExecutionListener{})

	describe(e)
	// Output:
	// rules: [non-empty]
	//   non-empty -- input must not be nil
	// supports listeners
}
