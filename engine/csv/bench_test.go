package csv_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine/csv"
)

func BenchmarkLoaderTenRows(b *testing.B) {
	src := buildCSV(10)
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.RuleConfigs(); err != nil {
			b.Fatalf("RuleConfigs: %v", err)
		}
		// Rewind the reader for the next iteration.
		loader = csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)
	}
}

func BenchmarkLoaderHundredRows(b *testing.B) {
	src := buildCSV(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)
		if _, err := loader.RuleConfigs(); err != nil {
			b.Fatalf("RuleConfigs: %v", err)
		}
	}
}

func buildCSV(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "rule-%d,%d,USD\n", i, i*10)
	}
	return sb.String()
}
