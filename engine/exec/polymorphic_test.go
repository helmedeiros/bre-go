package exec_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/exec"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
	"github.com/helmedeiros/bre-go/engine/inmemory"
	"github.com/helmedeiros/bre-go/engine/priority"
)

func TestExecutorWrapsEveryAdapter(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(t *testing.T) engine.Engine
	}{
		{name: "inmemory", seed: seedTypedInmemory},
		{name: "firstmatch", seed: seedTypedFirstmatch},
		{name: "priority", seed: seedTypedPriority},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ex := exec.New[int, string](tc.seed(t))

			out, matched, err := ex.Execute(context.Background(), 42)
			if err != nil {
				t.Fatalf("Execute: unexpected error: %v", err)
			}
			if out != "answer" {
				t.Fatalf("out: want %q, got %q", "answer", out)
			}
			if len(matched) != 1 || matched[0] != "the-answer" {
				t.Fatalf("matched: want [the-answer], got %v", matched)
			}
		})
	}
}

func seedTypedInmemory(t *testing.T) engine.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(inmemory.Rule{
		Name:      "the-answer",
		Condition: func(in interface{}) bool { return in.(int) == 42 },
		Action:    func(interface{}) interface{} { return "answer" },
	}); err != nil {
		t.Fatalf("inmemory.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedTypedFirstmatch(t *testing.T) engine.Engine {
	t.Helper()
	e := firstmatch.New()
	if err := e.AddRule(firstmatch.Rule{
		Name:      "the-answer",
		Condition: func(in interface{}) bool { return in.(int) == 42 },
		Action:    func(interface{}) interface{} { return "answer" },
	}); err != nil {
		t.Fatalf("firstmatch.AddRule: unexpected error: %v", err)
	}
	return e
}

func seedTypedPriority(t *testing.T) engine.Engine {
	t.Helper()
	e := priority.New()
	if err := e.AddRule(priority.Rule{
		Name:      "the-answer",
		Priority:  1,
		Condition: func(in interface{}) bool { return in.(int) == 42 },
		Action:    func(interface{}) interface{} { return "answer" },
	}); err != nil {
		t.Fatalf("priority.AddRule: unexpected error: %v", err)
	}
	return e
}
