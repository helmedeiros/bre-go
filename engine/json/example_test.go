package json_test

import (
	"context"
	encjson "encoding/json"
	"fmt"
	"strings"

	"github.com/helmedeiros/bre-go/engine"
	bjson "github.com/helmedeiros/bre-go/engine/json"
	"github.com/helmedeiros/bre-go/engine/priority"
)

type tierConfig struct {
	Name      string
	Priority  int
	Threshold int
	Decision  string
}

func (c tierConfig) RuleName() string { return c.Name }

func parseTier(item encjson.RawMessage) (tierConfig, error) {
	var wire struct {
		Name      string `json:"name"`
		Priority  int    `json:"priority"`
		Threshold int    `json:"threshold"`
		Decision  string `json:"decision"`
	}
	if err := encjson.Unmarshal(item, &wire); err != nil {
		return tierConfig{}, err
	}
	return tierConfig{
		Name:      wire.Name,
		Priority:  wire.Priority,
		Threshold: wire.Threshold,
		Decision:  wire.Decision,
	}, nil
}

func ExampleLoader() {
	source := strings.NewReader(`[
		{"name":"default","priority":1,"threshold":0,"decision":"standard-tier"},
		{"name":"premium","priority":100,"threshold":1000,"decision":"premium-tier"}
	]`)
	loader := bjson.NewLoaderFromReader(source, parseTier)

	eng := priority.New()
	err := engine.Load[tierConfig](loader, func(c tierConfig) error {
		return eng.AddRule(priority.Rule{
			Name:      c.Name,
			Priority:  c.Priority,
			Condition: func(in interface{}) bool { return in.(int) >= c.Threshold },
			Action:    func(interface{}) interface{} { return c.Decision },
		})
	})
	if err != nil {
		fmt.Println("load:", err)
		return
	}

	res, _ := eng.Execute(context.Background(), engine.Request{Input: 1500})
	fmt.Println(res.Matched, res.Output)
	// Output: [premium] premium-tier
}
