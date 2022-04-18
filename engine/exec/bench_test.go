package exec_test

import (
	"context"
	"testing"

	"github.com/helmedeiros/bre-go/engine/exec"
	"github.com/helmedeiros/bre-go/engine/inmemory"
)

func BenchmarkExecutorWrapperOverhead(b *testing.B) {
	e := inmemory.New()
	_ = e.AddRule(inmemory.Rule{
		Name:      "always",
		Condition: func(interface{}) bool { return true },
		Action:    func(in interface{}) interface{} { return in.(int) + 1 },
	})
	ex := exec.New[int, int](e)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ex.Execute(context.Background(), 42); err != nil {
			b.Fatalf("Execute: unexpected error: %v", err)
		}
	}
}
