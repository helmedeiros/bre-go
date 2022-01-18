package observability_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/observability"
)

func TestNopLoggerSatisfiesLogger(t *testing.T) {
	var _ observability.Logger = observability.NopLogger{}
}

func TestNopLoggerInfoDoesNotPanic(t *testing.T) {
	defer noPanic(t)

	var l observability.Logger = observability.NopLogger{}
	l.Info("hello")
}

func TestNopLoggerInfoWithFieldsDoesNotPanic(t *testing.T) {
	defer noPanic(t)

	var l observability.Logger = observability.NopLogger{}
	l.Info("with fields",
		observability.String("k", "v"),
		observability.Int("n", 7),
		observability.Bool("b", true),
	)
}

func TestNopLoggerErrorDoesNotPanic(t *testing.T) {
	defer noPanic(t)

	var l observability.Logger = observability.NopLogger{}
	l.Error("oh no", observability.Err(errors.New("boom")))
}

func TestStringFieldKey(t *testing.T) {
	got := observability.String("k", "v")
	if got.Key != "k" {
		t.Errorf("Key: want %q, got %q", "k", got.Key)
	}
}

func TestStringFieldValue(t *testing.T) {
	got := observability.String("k", "v")
	if got.Value != "v" {
		t.Errorf("Value: want %q, got %v", "v", got.Value)
	}
}

func TestIntFieldKey(t *testing.T) {
	got := observability.Int("n", 7)
	if got.Key != "n" {
		t.Errorf("Key: want %q, got %q", "n", got.Key)
	}
}

func TestIntFieldValue(t *testing.T) {
	got := observability.Int("n", 7)
	if got.Value != 7 {
		t.Errorf("Value: want 7, got %v", got.Value)
	}
}

func TestBoolFieldKey(t *testing.T) {
	got := observability.Bool("b", true)
	if got.Key != "b" {
		t.Errorf("Key: want %q, got %q", "b", got.Key)
	}
}

func TestBoolFieldValue(t *testing.T) {
	got := observability.Bool("b", true)
	if got.Value != true {
		t.Errorf("Value: want true, got %v", got.Value)
	}
}

func TestErrFieldUsesFixedKey(t *testing.T) {
	got := observability.Err(errors.New("nope"))
	if got.Key != "err" {
		t.Errorf("Key: want %q, got %q", "err", got.Key)
	}
}

func TestErrFieldCarriesTheError(t *testing.T) {
	cause := errors.New("nope")
	got := observability.Err(cause)
	if got.Value != cause {
		t.Errorf("Value: want %v, got %v", cause, got.Value)
	}
}

// noPanic is a deferred guard the NopLogger panic-discipline tests
// share. A test passes when its NopLogger call returns; the only
// failure mode is a panic, which this helper turns into t.Fatalf.
func noPanic(t *testing.T) {
	t.Helper()
	if r := recover(); r != nil {
		t.Fatalf("unexpected panic: %v", r)
	}
}
