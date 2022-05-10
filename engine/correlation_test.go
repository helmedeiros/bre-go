package engine_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

func TestCorrelationIDFromContextIsEmptyOnFreshContext(t *testing.T) {
	if got := engine.CorrelationIDFromContext(context.Background()); got != "" {
		t.Fatalf("CorrelationIDFromContext: want empty on fresh ctx, got %q", got)
	}
}

func TestWithCorrelationIDRoundTripsThroughContext(t *testing.T) {
	ctx := engine.WithCorrelationID(context.Background(), "req-42")

	if got := engine.CorrelationIDFromContext(ctx); got != "req-42" {
		t.Fatalf("CorrelationIDFromContext: want %q, got %q", "req-42", got)
	}
}

func TestNestedWithCorrelationIDOverwritesOuter(t *testing.T) {
	outer := engine.WithCorrelationID(context.Background(), "outer")
	inner := engine.WithCorrelationID(outer, "inner")

	if got := engine.CorrelationIDFromContext(inner); got != "inner" {
		t.Fatalf("inner CorrelationIDFromContext: want %q, got %q", "inner", got)
	}
}

type otherKey struct{}

func TestCorrelationIDDoesNotCollideWithOtherContextKeys(t *testing.T) {
	ctx := context.WithValue(context.Background(), otherKey{}, "elsewhere")
	ctx = engine.WithCorrelationID(ctx, "ours")

	if got := engine.CorrelationIDFromContext(ctx); got != "ours" {
		t.Fatalf("CorrelationIDFromContext: want %q, got %q", "ours", got)
	}
	if got, _ := ctx.Value(otherKey{}).(string); got != "elsewhere" {
		t.Fatalf("foreign key value: want %q (untouched), got %q", "elsewhere", got)
	}
}
