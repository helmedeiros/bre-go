package firstmatch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/firstmatch"
)

func TestExecuteAcceptsNilContext(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})

	var nilCtx context.Context
	res, err := e.Execute(nilCtx, engine.Request{Input: 42})

	if err != nil {
		t.Fatalf("Execute(nil ctx): want no error, got %v", err)
	}
	if len(res.Matched) != 1 {
		t.Fatalf("Matched: want 1, got %v", res.Matched)
	}
}

func TestExecuteReturnsContextErrorOnCancelledContext(t *testing.T) {
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "alpha",
		Condition: func(interface{}) bool { return true },
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.Execute(ctx, engine.Request{Input: 42})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute err: want context.Canceled, got %v", err)
	}
}

func TestConditionContextReceivesTheContext(t *testing.T) {
	var seen context.Context
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name: "ctx-aware",
		ConditionContext: func(ctx context.Context, _ interface{}) bool {
			seen = ctx
			return true
		},
	})

	parent := context.WithValue(context.Background(), ctxKey("k"), "v")
	_, _ = e.Execute(parent, engine.Request{Input: nil})

	if seen.Value(ctxKey("k")) != "v" {
		t.Fatalf("ConditionContext did not receive parent ctx value")
	}
}

func TestActionContextRunsForMatchingRule(t *testing.T) {
	ran := false
	e := firstmatch.New()
	_ = e.AddRule(firstmatch.Rule{
		Name:      "ctx-action",
		Condition: func(interface{}) bool { return true },
		ActionContext: func(context.Context, interface{}) interface{} {
			ran = true
			return "ok"
		},
	})

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if !ran {
		t.Fatalf("ActionContext did not run")
	}
}

func TestAddRuleAcceptsRuleWithOnlyConditionContext(t *testing.T) {
	e := firstmatch.New()

	err := e.AddRule(firstmatch.Rule{
		Name:             "ctx-only",
		ConditionContext: func(context.Context, interface{}) bool { return true },
	})

	if err != nil {
		t.Fatalf("AddRule: want nil for ConditionContext-only rule, got %v", err)
	}
}

type ctxKey string
