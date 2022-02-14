package conditions_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/conditions"
)

func TestAndWithNoArgumentsIsTrue(t *testing.T) {
	if !conditions.And()("x") {
		t.Fatalf("And(): want true, got false")
	}
}

func TestAndWithAllTrueIsTrue(t *testing.T) {
	if !conditions.And(constTrue, constTrue, constTrue)("x") {
		t.Fatalf("And(true,true,true): want true, got false")
	}
}

func TestAndIsFalseOnFirstFalse(t *testing.T) {
	if conditions.And(constTrue, constFalse, constTrue)("x") {
		t.Fatalf("And(true,false,true): want false, got true")
	}
}

func TestAndShortCircuitsOnFalse(t *testing.T) {
	calls := 0
	counted := func(interface{}) bool { calls++; return true }

	conditions.And(constFalse, counted)("x")

	if calls != 0 {
		t.Fatalf("calls after False: want 0, got %d", calls)
	}
}

func TestOrWithNoArgumentsIsFalse(t *testing.T) {
	if conditions.Or()("x") {
		t.Fatalf("Or(): want false, got true")
	}
}

func TestOrWithAllFalseIsFalse(t *testing.T) {
	if conditions.Or(constFalse, constFalse, constFalse)("x") {
		t.Fatalf("Or(false,false,false): want false, got true")
	}
}

func TestOrIsTrueOnFirstTrue(t *testing.T) {
	if !conditions.Or(constFalse, constTrue, constFalse)("x") {
		t.Fatalf("Or(false,true,false): want true, got false")
	}
}

func TestOrShortCircuitsOnTrue(t *testing.T) {
	calls := 0
	counted := func(interface{}) bool { calls++; return false }

	conditions.Or(constTrue, counted)("x")

	if calls != 0 {
		t.Fatalf("calls after True: want 0, got %d", calls)
	}
}

func TestNotInvertsTrue(t *testing.T) {
	if conditions.Not(constTrue)("x") {
		t.Fatalf("Not(true): want false, got true")
	}
}

func TestNotInvertsFalse(t *testing.T) {
	if !conditions.Not(constFalse)("x") {
		t.Fatalf("Not(false): want true, got false")
	}
}

func TestAlwaysIsTrue(t *testing.T) {
	if !conditions.Always()("x") {
		t.Fatalf("Always(): want true, got false")
	}
}

func TestNeverIsFalse(t *testing.T) {
	if conditions.Never()("x") {
		t.Fatalf("Never(): want false, got true")
	}
}

func TestAndOrNotComposeToBuildDeMorgan(t *testing.T) {
	notAOrNotB := conditions.Or(conditions.Not(constTrue), conditions.Not(constFalse))
	notAAndB := conditions.Not(conditions.And(constTrue, constFalse))

	if notAOrNotB("x") != notAAndB("x") {
		t.Fatalf("De Morgan: NOT(A AND B) != NOT(A) OR NOT(B)")
	}
}

func constTrue(interface{}) bool  { return true }
func constFalse(interface{}) bool { return false }
