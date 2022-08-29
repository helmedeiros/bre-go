// otel-review runs the OTel adapter through a set of realistic
// scenarios, captures the resulting span trees + attributes, and
// prints them as readable JSON. The output is what an operator
// looking at a tracing backend (Jaeger, Tempo, Honeycomb) would
// effectively see for each scenario.
//
// Run: go run ./cmd/otel-review > scenarios.json
//
// Audit: open scenarios.json side-by-side with REPORT.md and check
// whether the captured spans answer the operator questions REPORT.md
// poses for each scenario.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	breotel "github.com/helmedeiros/bre-go/observability/otel"
)

type capturedAttr struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type capturedEvent struct {
	Name       string         `json:"name"`
	Attributes []capturedAttr `json:"attributes,omitempty"`
}

type capturedSpan struct {
	Name        string          `json:"name"`
	TraceID     string          `json:"trace_id"`
	SpanID      string          `json:"span_id"`
	ParentID    string          `json:"parent_id,omitempty"`
	StatusCode  string          `json:"status_code"`
	StatusDesc  string          `json:"status_description,omitempty"`
	Attributes  []capturedAttr  `json:"attributes,omitempty"`
	Events      []capturedEvent `json:"events,omitempty"`
	DurationNs  int64           `json:"duration_ns"`
}

type scenario struct {
	Name        string         `json:"scenario"`
	Description string         `json:"description"`
	Spans       []capturedSpan `json:"spans"`
	Notes       []string       `json:"notes,omitempty"`
}

func main() {
	scenarios := []scenario{
		run("happy-single-match", "One input that matches exactly one rule. Expected: one span, success status, AttrMatchedCount=1, AttrMatchedNames=[<rule name>].", happySingleMatch),
		run("happy-no-match", "One input that matches no rule. Expected: one span, success status, AttrMatchedCount=0, AttrMatchedNames=[].", happyNoMatch),
		run("happy-multi-match", "One input that matches three rules via overlapping conditions. Expected: AttrMatchedNames carries all three in first-match order.", happyMultiMatch),
		run("error-action-panic", "Rule whose Action panics. Expected: span has Error status, recorded error event with the typed *ActionPanicError, AttrMatchedNames still reflects the rule that matched before its action blew up.", errorActionPanic),
		run("ctx-cancellation", "Caller cancels ctx mid-Execute. Expected: span has the cancellation error recorded; status code Error.", ctxCancellation),
		run("correlation-id-set", "Caller stamps a correlation ID on ctx via engine.WithCorrelationID. Expected: span has AttrCorrelationID with the stamped value.", correlationIDSet),
		run("correlation-id-absent", "No correlation ID on ctx. Expected: AttrCorrelationID attribute is absent (not empty-string).", correlationIDAbsent),
		run("nested-under-parent", "Caller wraps Execute in their own parent span. Expected: the rule.engine.execute span is a CHILD of the parent, same trace ID.", nestedUnderParent),
		run("concurrent-executes", "Eight goroutines hammer Execute concurrently with their own parent spans. Expected: 8 distinct rule.engine.execute spans, each parented to its own goroutine's parent.", concurrentExecutes),
		run("high-fanout-100-matches", "One input that matches 100 rules. Expected: AttrMatchedNames is a 100-element string slice. Audit: is this readable in a tracing UI? What's the practical ceiling?", highFanout100Matches),
		run("unicode-rule-name", "Rule name with unicode + emoji + accents. Expected: round-trips through attribute encoding unchanged.", unicodeRuleName),
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(scenarios); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(name, desc string, fn func(*sdktrace.TracerProvider, *tracetest.SpanRecorder) []string) scenario {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	notes := fn(tp, rec)
	return scenario{
		Name:        name,
		Description: desc,
		Spans:       captureSpans(rec.Ended()),
		Notes:       notes,
	}
}

func captureSpans(spans []sdktrace.ReadOnlySpan) []capturedSpan {
	out := make([]capturedSpan, 0, len(spans))
	for _, s := range spans {
		cs := capturedSpan{
			Name:       s.Name(),
			TraceID:    s.SpanContext().TraceID().String(),
			SpanID:     s.SpanContext().SpanID().String(),
			StatusCode: s.Status().Code.String(),
			StatusDesc: s.Status().Description,
			Attributes: capturedAttrs(s.Attributes()),
			DurationNs: s.EndTime().Sub(s.StartTime()).Nanoseconds(),
		}
		if s.Parent().IsValid() {
			cs.ParentID = s.Parent().SpanID().String()
		}
		for _, ev := range s.Events() {
			cs.Events = append(cs.Events, capturedEvent{
				Name:       ev.Name,
				Attributes: capturedAttrs(ev.Attributes),
			})
		}
		out = append(out, cs)
	}
	return out
}

func capturedAttrs(attrs []attribute.KeyValue) []capturedAttr {
	out := make([]capturedAttr, 0, len(attrs))
	for _, a := range attrs {
		var v any
		var typ string
		switch a.Value.Type() {
		case attribute.STRING:
			v = a.Value.AsString()
			typ = "string"
		case attribute.INT64:
			v = a.Value.AsInt64()
			typ = "int64"
		case attribute.STRINGSLICE:
			v = a.Value.AsStringSlice()
			typ = "string_slice"
		case attribute.BOOL:
			v = a.Value.AsBool()
			typ = "bool"
		case attribute.FLOAT64:
			v = a.Value.AsFloat64()
			typ = "float64"
		default:
			v = a.Value.Emit()
			typ = a.Value.Type().String()
		}
		out = append(out, capturedAttr{Key: string(a.Key), Type: typ, Value: v})
	}
	return out
}

// ----- scenarios -----

func happySingleMatch(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
		{Name: "ar", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "AR"}},
	}), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	return nil
}

func happyNoMatch(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "DE"}})
	return nil
}

func happyMultiMatch(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	rules := []indexed.Rule{
		{Name: "br-equality", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
		{Name: "mercosul", Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}}},
		{Name: "south-america", Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY", "PY", "CL", "PE", "CO", "VE"}}},
	}
	traced := breotel.Wrap(buildEngine(rules), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	return nil
}

func errorActionPanic(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	rules := []indexed.Rule{
		{
			Name:   "explodes",
			Match:  parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"},
			Action: func(input interface{}) interface{} { panic("synthetic action panic") },
		},
	}
	traced := breotel.Wrap(buildEngine(rules), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"k": "v"}})
	return []string{
		"Audit: does the recorded error event include the panicked rule's name? An operator triaging this needs to know WHICH rule panicked, not just that one did.",
	}
}

func ctxCancellation(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	rules := []indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}
	traced := breotel.Wrap(buildEngine(rules), tp.Tracer("review"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled
	_, _ = traced.Execute(ctx, engine.Request{Input: map[string]string{"country": "BR"}})
	return []string{
		"Audit: is the cancellation visible on the span? Status code Error? RecordError event with context.Canceled?",
	}
}

func correlationIDSet(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	ctx := engine.WithCorrelationID(context.Background(), "req-abc-123")
	_, _ = traced.Execute(ctx, engine.Request{Input: map[string]string{"country": "BR"}})
	return nil
}

func correlationIDAbsent(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	return []string{
		"Audit: AttrCorrelationID should be ABSENT from the attribute list (not present with empty value). Confirm.",
	}
}

func nestedUnderParent(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	tracer := tp.Tracer("review")
	parentCtx, parent := tracer.Start(context.Background(), "incoming-http-request")
	_, _ = traced.Execute(parentCtx, engine.Request{Input: map[string]string{"country": "BR"}})
	parent.End()
	return []string{
		"Audit: the rule.engine.execute span MUST have parent_id == incoming-http-request span's span_id, and the same trace_id.",
	}
}

func concurrentExecutes(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "br", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	tracer := tp.Tracer("review")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, parent := tracer.Start(context.Background(), fmt.Sprintf("worker-%d", i))
			defer parent.End()
			_, _ = traced.Execute(ctx, engine.Request{Input: map[string]string{"country": "BR"}})
		}(i)
	}
	wg.Wait()
	return []string{
		"Audit: 8 rule.engine.execute spans, each with a distinct parent_id matching the worker-N span. No span leaks or cross-contamination.",
	}
}

func highFanout100Matches(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	rules := make([]indexed.Rule, 100)
	for i := 0; i < 100; i++ {
		rules[i] = indexed.Rule{
			Name: fmt.Sprintf("rule-%03d", i),
			Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY", "PY", "CL"}},
		}
	}
	traced := breotel.Wrap(buildEngine(rules), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	return []string{
		"Audit: AttrMatchedNames is a 100-element string slice. In a real tracing UI (Jaeger, Tempo, Honeycomb), is this readable? Is there a practical ceiling we should document? Should we truncate at N and add an overflow indicator?",
	}
}

func unicodeRuleName(tp *sdktrace.TracerProvider, _ *tracetest.SpanRecorder) []string {
	traced := breotel.Wrap(buildEngine([]indexed.Rule{
		{Name: "regra-brasileira-ção-☃-\U0001F1E7\U0001F1F7", Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"}},
	}), tp.Tracer("review"))
	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	return []string{
		"Audit: AttrMatchedNames preserves unicode + emoji bytes faithfully.",
	}
}

func buildEngine(rules []indexed.Rule) *indexed.Engine {
	e := indexed.New()
	for _, r := range rules {
		if err := e.AddRule(r); err != nil {
			panic(fmt.Sprintf("AddRule %q: %v", r.Name, err))
		}
	}
	if err := e.Build(); err != nil {
		panic(err)
	}
	return e
}

// Avoid "imported and not used" if a scenario removes its use.
var _ = errors.New
var _ = time.Second
var _ = oteltrace.SpanContext{}
