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
