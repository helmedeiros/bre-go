package conditions_test

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/conditions"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

type order struct {
	amount   int
	currency string
	flagged  bool
}

func amountOver(n int) func(interface{}) bool {
	return func(in interface{}) bool { return in.(order).amount > n }
}

func currencyIs(c string) func(interface{}) bool {
	return func(in interface{}) bool { return in.(order).currency == c }
}

func flagged(in interface{}) bool { return in.(order).flagged }

func ExampleAnd() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name: "high-value-clean-usd",
		Condition: conditions.And(
			amountOver(100),
			currencyIs("USD"),
			conditions.Not(flagged),
		),
		Action: func(interface{}) interface{} { return "approve" },
	})

	clean := order{amount: 250, currency: "USD", flagged: false}
	res, _ := e.Execute(context.Background(), engine.Request{Input: clean})

	fmt.Println(res.Matched, res.Output)
	// Output: [high-value-clean-usd] approve
}

func ExampleOr() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name: "any-major-currency",
		Condition: conditions.Or(
			currencyIs("USD"),
			currencyIs("EUR"),
			currencyIs("BRL"),
		),
		Action: func(interface{}) interface{} { return "supported" },
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: order{currency: "EUR"}})

	fmt.Println(res.Matched, res.Output)
	// Output: [any-major-currency] supported
}

func ExampleAlways() {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "premium",
		Condition: amountOver(1000),
		Action:    func(interface{}) interface{} { return "premium-tier" },
	})
	_ = e.AddRule(firstmatch.Rule{
		Name:      "default",
		Condition: conditions.Always(),
		Action:    func(interface{}) interface{} { return "default-tier" },
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: order{amount: 50}})

	fmt.Println(res.Matched, res.Output)
	// Output: [default] default-tier
}
