package conditions_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine/conditions"
)

func BenchmarkRawPredicate(b *testing.B) {
	p := func(in interface{}) bool { return in.(int) > 0 }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p(42)
	}
}

func BenchmarkAndOverFiveTruePredicates(b *testing.B) {
	t := constTrue
	p := conditions.And(t, t, t, t, t)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p(42)
	}
}

func BenchmarkOrShortCircuitsOnFirstTrue(b *testing.B) {
	t := constTrue
	f := constFalse
	p := conditions.Or(t, f, f, f, f)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p(42)
	}
}

func BenchmarkNot(b *testing.B) {
	p := conditions.Not(constTrue)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p(42)
	}
}

func BenchmarkAlways(b *testing.B) {
	p := conditions.Always()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p(42)
	}
}
