// sink-demo proves the v0.18.0 metrics port is small enough that
// arbitrary backends can be written from scratch in ~50 lines. Three
// independent sinks are exercised against the same wrapped engine;
// each one was written without looking at the others and without
// pulling in any external metrics SDK.
//
// Run: go run ./cmd/sink-demo > results/sink-demo.txt
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/observability/metrics"

	"github.com/helmedeiros/bre-go-v18-review/sinks"
)

func main() {
	inner := buildEngine()

	channelSink := sinks.NewChannelSink(1024)
	atomicSink := &sinks.AtomicCounterSink{}
	percentileSink := sinks.NewPercentileSink(1024)

	// Stack three decorators -- each fans out into its own sink. Order
	// doesn't matter; the metric event is emitted once per Execute by
	// the innermost decorator and propagated outward.
	wrapped := metrics.Wrap(
		metrics.Wrap(
			metrics.Wrap(inner, channelSink),
			atomicSink),
		percentileSink)

	inputs := []map[string]string{
		{"country": "BR"}, {"country": "AR"}, {"country": "DE"}, {"country": "BR"},
		{"country": "BR"}, {"country": "UY"}, {"country": "FR"}, {"country": "BR"},
	}

	ctxBg := context.Background()
	canceledCtx, cancel := context.WithCancel(ctxBg)
	cancel()

	for _, in := range inputs {
		_, _ = wrapped.Execute(ctxBg, engine.Request{Input: in})
	}
	_, _ = wrapped.Execute(canceledCtx, engine.Request{Input: map[string]string{"country": "BR"}})

	fmt.Fprintln(os.Stdout, "=== AtomicCounterSink ===")
	executions, matched, errored, canceled, deadlineExceeded := atomicSink.Snapshot()
	fmt.Fprintf(os.Stdout, "executions=%d matched=%d errored=%d canceled=%d deadline_exceeded=%d\n",
		executions, matched, errored, canceled, deadlineExceeded)

	fmt.Fprintln(os.Stdout, "\n=== ChannelSink ===")
	close(channelSink.C)
	count := 0
	for m := range channelSink.C {
		count++
		fmt.Fprintf(os.Stdout, "[%2d] adapter=%s matched=%d duration=%v canceled=%v err=%v\n",
			count, shortAdapterName(m.Adapter), m.MatchedCount, roundForPrint(m.Duration), m.Canceled, m.Err)
	}
	fmt.Fprintf(os.Stdout, "channel-received=%d dropped=%d\n", count, channelSink.Dropped())

	fmt.Fprintln(os.Stdout, "\n=== PercentileSink ===")
	p50, p95, p99 := percentileSink.Percentiles()
	fmt.Fprintf(os.Stdout, "p50=%v p95=%v p99=%v\n", roundForPrint(p50), roundForPrint(p95), roundForPrint(p99))
}

func buildEngine() engine.Engine {
	e := indexed.New()
	for _, r := range []indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
		{Name: "mercosul", Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}}},
	} {
		_ = e.AddRule(r)
	}
	_ = e.Build()
	return e
}

func shortAdapterName(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

func roundForPrint(d time.Duration) time.Duration {
	if d == 0 {
		return 0
	}
	if d < time.Microsecond {
		return d
	}
	return d.Round(100 * time.Nanosecond)
}
