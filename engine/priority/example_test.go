package priority_test

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func ExampleEngine() {
	e := priority.New()

	_ = e.AddRule(priority.Rule{
		Name:      "default",
		Priority:  0,
		Condition: func(interface{}) bool { return true },
		Action:    func(interface{}) interface{} { return "standard-tier" },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "vip",
		Priority:  100,
		Condition: func(in interface{}) bool { return in.(string) == "vip-token" },
		Action:    func(interface{}) interface{} { return "vip-tier" },
	})
	_ = e.AddRule(priority.Rule{
		Name:      "blocklisted",
		Priority:  1000,
		Condition: func(in interface{}) bool { return in.(string) == "blocked-token" },
		Action:    func(interface{}) interface{} { return "denied" },
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: "vip-token"})

	fmt.Println(res.Matched, res.Output)
	// Output: [vip] vip-tier
}
