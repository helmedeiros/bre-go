package inmemory_test

import (
	"errors"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func ExampleActionPanicError() {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "divide-by-zero",
		Condition: func(interface{}) bool { return true },
		Action: func(in interface{}) interface{} {
			n := in.(int)
			return 100 / n
		},
	})

	_, err := e.Execute(engine.Request{Input: 0})

	var pe *inmemory.ActionPanicError
	if errors.As(err, &pe) {
		fmt.Printf("rule %q panicked\n", pe.RuleName())
	}
	// Output: rule "divide-by-zero" panicked
}
