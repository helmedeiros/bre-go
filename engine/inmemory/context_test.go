package inmemory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func TestExecuteAcceptsNilContext(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
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
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
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
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name: "ctx-aware",
		ConditionContext: func(ctx context.Context, _ interface{}) bool {
			seen = ctx
			return true
		},
	})

	parent := context.WithValue(context.Background(), ctxKey("tenant"), "abc")
	_, _ = e.Execute(parent, engine.Request{Input: nil})

	if seen.Value(ctxKey("tenant")) != "abc" {
		t.Fatalf("ConditionContext did not receive parent ctx value")
	}
}

func TestActionContextReceivesTheContext(t *testing.T) {
	var seen context.Context
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "ctx-aware",
		Condition: func(interface{}) bool { return true },
		ActionContext: func(ctx context.Context, _ interface{}) interface{} {
			seen = ctx
			return "ok"
		},
	})

	parent := context.WithValue(context.Background(), ctxKey("tenant"), "abc")
	_, _ = e.Execute(parent, engine.Request{Input: nil})

	if seen.Value(ctxKey("tenant")) != "abc" {
		t.Fatalf("ActionContext did not receive parent ctx value")
	}
}

func TestConditionContextPreferredOverCondition(t *testing.T) {
	plainCalled := false
	ctxCalled := false
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:             "both",
		Condition:        func(interface{}) bool { plainCalled = true; return true },
		ConditionContext: func(context.Context, interface{}) bool { ctxCalled = true; return true },
	})

	_, _ = e.Execute(context.Background(), engine.Request{Input: nil})

	if plainCalled {
		t.Fatalf("Condition should not be called when ConditionContext is set")
	}
	if !ctxCalled {
		t.Fatalf("ConditionContext should be called when set")
	}
}

func TestAddRuleAcceptsRuleWithOnlyConditionContext(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{
		Name:             "ctx-only",
		ConditionContext: func(context.Context, interface{}) bool { return true },
	})

	if err != nil {
		t.Fatalf("AddRule: want nil for ConditionContext-only rule, got %v", err)
	}
}

func TestAddRuleRejectsBothConditionAndConditionContextNil(t *testing.T) {
	e := inmemory.New()

	err := e.AddRule(inmemory.Rule{Name: "no-condition"})

	if !errors.Is(err, inmemory.ErrNilCondition) {
		t.Fatalf("AddRule: want ErrNilCondition, got %v", err)
	}
}

type ctxKey string
