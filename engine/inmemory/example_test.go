package inmemory_test

import (
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/observability"
)

func ExampleEngine() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "is-adult",
		Condition: func(in interface{}) bool { return in.(int) >= 18 },
		Action:    func(in interface{}) interface{} { return "adult" },
	})

	res, _ := e.Execute(engine.Request{Input: 21})

	fmt.Println(res.Matched, res.Output)
	// Output: [is-adult] adult
}

type printListener struct{}

func (printListener) OnRuleMatched(m observability.Match) {
	fmt.Printf("matched %s\n", m.Rule)
}

func ExampleEngine_AddListener() {
	e := inmemory.New()
	e.AddListener(printListener{})
	_ = e.AddRule(inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
	})

	_, _ = e.Execute(engine.Request{Input: nil})
	// Output: matched always
}

func ExampleEngine_RuleNames() {
	var eng engine.Engine = newSampleEngine()

	if lister, ok := eng.(engine.RuleLister); ok {
		for _, name := range lister.RuleNames() {
			fmt.Println(name)
		}
	}
	// Output:
	// is-adult
	// is-senior
}

func ExampleEngine_RuleInfos() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:        "is-adult",
		Description: "voting age",
		Tags:        []string{"age", "eligibility"},
		Condition:   func(in interface{}) bool { return in.(int) >= 18 },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:        "is-senior",
		Description: "senior discount eligibility",
		Tags:        []string{"age", "discount"},
		Condition:   func(in interface{}) bool { return in.(int) >= 65 },
	})

	for _, info := range e.RuleInfos() {
		fmt.Printf("%s [%v]: %s\n", info.Name, info.Tags, info.Description)
	}
	// Output:
	// is-adult [[age eligibility]]: voting age
	// is-senior [[age discount]]: senior discount eligibility
}

func newSampleEngine() *inmemory.Engine {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "is-adult",
		Condition: func(in interface{}) bool { return in.(int) >= 18 },
	})
	_ = e.AddRule(inmemory.Rule{
		Name:      "is-senior",
		Condition: func(in interface{}) bool { return in.(int) >= 65 },
	})
	return e
}
