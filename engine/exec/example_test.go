package exec_test

import (
	"fmt"

	"github.com/helmedeiros/bre-go/engine/exec"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

type order struct {
	amount   int
	currency string
}

func ExampleExecutor() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "high-value-usd",
		Condition: func(in interface{}) bool { return in.(order).amount > 100 && in.(order).currency == "USD" },
		Action:    func(interface{}) interface{} { return "approve" },
	})

	ex := exec.New[order, string](e)
	decision, matched, _ := ex.Execute(order{amount: 250, currency: "USD"})

	fmt.Println(matched, decision)
	// Output: [high-value-usd] approve
}
