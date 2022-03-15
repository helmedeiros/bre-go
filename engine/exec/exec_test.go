package exec_test

import (
	"errors"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/exec"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func newStringEngine(t *testing.T, action func(in interface{}) interface{}) engine.Engine {
	t.Helper()
	e := inmemory.New()
	if err := e.AddRule(inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
		Action:    action,
	}); err != nil {
		t.Fatalf("AddRule: unexpected error: %v", err)
	}
	return e
}

func TestExecuteReturnsTheTypedOutputOnMatch(t *testing.T) {
	eng := newStringEngine(t, func(in interface{}) interface{} {
		return "decision-" + in.(string)
	})
	ex := exec.New[string, string](eng)

	out, _, err := ex.Execute("alpha")
	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out != "decision-alpha" {
		t.Fatalf("Execute output: want %q, got %q", "decision-alpha", out)
	}
}

func TestExecuteReportsMatchedRuleNames(t *testing.T) {
	eng := newStringEngine(t, func(interface{}) interface{} { return "x" })
	ex := exec.New[string, string](eng)

	_, matched, _ := ex.Execute("alpha")

	if len(matched) != 1 || matched[0] != "always" {
		t.Fatalf("Matched: want [always], got %v", matched)
	}
}

func TestExecuteReturnsZeroOutWhenNothingMatches(t *testing.T) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "never",
		Condition: func(interface{}) bool { return false },
	})
	ex := exec.New[string, int](e)

	out, _, err := ex.Execute("alpha")

	if err != nil {
		t.Fatalf("Execute: unexpected error: %v", err)
	}
	if out != 0 {
		t.Fatalf("Execute output: want 0 (zero Out), got %d", out)
	}
}

func TestExecuteReturnsTypeMismatchErrorWhenOutputIsWrongType(t *testing.T) {
	eng := newStringEngine(t, func(interface{}) interface{} { return 42 }) // engine emits int
	ex := exec.New[string, string](eng)                                    // executor expects string

	_, _, err := ex.Execute("alpha")

	var mismatch *exec.OutputTypeMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Execute err: want *OutputTypeMismatchError, got %T (%v)", err, err)
	}
}

func TestOutputTypeMismatchErrorIncludesExpectedAndGotTypeNames(t *testing.T) {
	err := &exec.OutputTypeMismatchError{Expected: "string", Got: "int"}

	msg := err.Error()

	if msg != "exec: output type mismatch: expected string, got int" {
		t.Fatalf("Error: unexpected message: %q", msg)
	}
}

func TestExecutePropagatesEngineErrors(t *testing.T) {
	eng := newStringEngine(t, func(interface{}) interface{} { panic("boom") })
	ex := exec.New[string, string](eng)

	_, _, err := ex.Execute("alpha")

	if err == nil {
		t.Fatalf("Execute err: want non-nil from panicking action, got nil")
	}
}
