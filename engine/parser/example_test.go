package parser_test

import (
	"context"
	"fmt"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// ExampleParse compiles a single expression string into a Predicate
// and evaluates it directly against a fact map.
func ExampleParse() {
	pred, _ := parser.Parse(`origin == "DE" AND tier IN ("vip", "premium")`)

	matches := pred(map[string]interface{}{
		"origin": "DE",
		"tier":   "vip",
	})

	fmt.Println(matches)
	// Output: true
}

// ExampleAsCondition wires a parsed Predicate into an inmemory.Engine
// rule via the AsCondition bridge. Demonstrates the production wiring
// where the condition string would come from CSV / JSON.
func ExampleAsCondition() {
	type request struct {
		Origin string
		Tier   string
	}
	factOf := func(in interface{}) map[string]interface{} {
		r := in.(request)
		return map[string]interface{}{"origin": r.Origin, "tier": r.Tier}
	}

	pred, _ := parser.Parse(`origin == "DE" AND tier IN ("vip", "premium")`)

	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "de-vip",
		Condition: parser.AsCondition(pred, factOf),
		Action:    func(interface{}) interface{} { return "approve" },
	})

	res, _ := e.Execute(context.Background(), engine.Request{Input: request{Origin: "DE", Tier: "vip"}})

	fmt.Println(res.Matched, res.Output)
	// Output: [de-vip] approve
}
