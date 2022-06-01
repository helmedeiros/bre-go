package json_test

import (
	"fmt"
	"strings"
	"testing"

	bjson "github.com/helmedeiros/bre-go/engine/json"
)

func BenchmarkLoaderTenItems(b *testing.B) {
	src := buildJSON(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)
		if _, err := loader.RuleConfigs(); err != nil {
			b.Fatalf("RuleConfigs: %v", err)
		}
	}
}

func BenchmarkLoaderHundredItems(b *testing.B) {
	src := buildJSON(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)
		if _, err := loader.RuleConfigs(); err != nil {
			b.Fatalf("RuleConfigs: %v", err)
		}
	}
}

func buildJSON(n int) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"name":"rule-%d","amount":%d,"currency":"USD"}`, i, i*10)
	}
	sb.WriteString("]")
	return sb.String()
}
