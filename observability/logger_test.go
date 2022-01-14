package observability_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/observability"
)

func TestNopLoggerSatisfiesLogger(t *testing.T) {
	var _ observability.Logger = observability.NopLogger{}
}

func TestNopLoggerDoesNothing(t *testing.T) {
	// The contract is that NopLogger's methods discard everything
	// without panicking on any input shape. We do not have a way to
	// assert "did nothing" beyond "did not panic" -- so cover the
	// branches and let the deferred recover prove they returned.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NopLogger panicked: %v", r)
		}
	}()

	var l observability.Logger = observability.NopLogger{}
	l.Info("hello")
	l.Info("with fields",
		observability.String("k", "v"),
		observability.Int("n", 7),
		observability.Bool("b", true),
	)
	l.Error("oh no", observability.Err(errors.New("boom")))
}

func TestFieldConstructorsSetExpectedShape(t *testing.T) {
	cases := []struct {
		name string
		got  observability.Field
		want observability.Field
	}{
		{
			name: "string",
			got:  observability.String("k", "v"),
			want: observability.Field{Key: "k", Value: "v"},
		},
		{
			name: "int",
			got:  observability.Int("n", 7),
			want: observability.Field{Key: "n", Value: 7},
		},
		{
			name: "bool",
			got:  observability.Bool("b", true),
			want: observability.Field{Key: "b", Value: true},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s: want %#v, got %#v", c.name, c.want, c.got)
			}
		})
	}
}

func TestErrFieldUsesFixedKey(t *testing.T) {
	e := errors.New("nope")
	got := observability.Err(e)
	if got.Key != "err" {
		t.Errorf("Key: want %q, got %q", "err", got.Key)
	}
	if got.Value != e {
		t.Errorf("Value: want %v, got %v", e, got.Value)
	}
}
