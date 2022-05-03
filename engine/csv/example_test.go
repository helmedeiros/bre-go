package csv_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/csv"
	"github.com/helmedeiros/bre-go/engine/priority"
)

type tierConfig struct {
	Name      string
	Priority  int
	Threshold int
	Decision  string
}

func (c tierConfig) RuleName() string { return c.Name }

func parseTier(columns []string) (tierConfig, error) {
	if len(columns) < 4 {
		return tierConfig{}, fmt.Errorf("expected 4 columns, got %d", len(columns))
	}
	prio := 0
	for _, r := range columns[1] {
		prio = prio*10 + int(r-'0')
	}
	thr := 0
	for _, r := range columns[2] {
		thr = thr*10 + int(r-'0')
	}
	return tierConfig{Name: columns[0], Priority: prio, Threshold: thr, Decision: columns[3]}, nil
}

func ExampleLoader() {
	source := strings.NewReader("default,1,0,standard-tier\npremium,100,1000,premium-tier\n")
	loader := csv.NewLoaderFromReader(source, parseTier)

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
