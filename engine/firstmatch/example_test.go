package firstmatch_test

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

func ExampleEngine() {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "premium",
		Condition: func(in interface{}) bool { return in.(int) >= 1000 },
		Action:    func(in interface{}) interface{} { return "premium-tier" },
	})
	_ = e.AddRule(firstmatch.Rule{
		Name:      "standard",
		Condition: func(in interface{}) bool { return in.(int) >= 100 },
		Action:    func(in interface{}) interface{} { return "standard-tier" },
	})
	_ = e.AddRule(firstmatch.Rule{
		Name:      "default",
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { return "default-tier" },
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: 250})

	fmt.Println(res.Matched, res.Output)
	// Output: [standard] standard-tier
}
