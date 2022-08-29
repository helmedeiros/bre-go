package otel_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/observability"
	bretotel "github.com/helmedeiros/bre-go/observability/otel"
)

func newRecorder() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return rec, tp
}

func buildTwoRuleEngine(t *testing.T) *indexed.Engine {
	t.Helper()
	e := indexed.New()
	if err := e.AddRule(indexed.Rule{
		Name:  "br",
		Match: parser.StringCondition{Field: "country", Op: parser.OpEq, Value: "BR"},
	}); err != nil {
		t.Fatalf("AddRule br: %v", err)
	}
	if err := e.AddRule(indexed.Rule{
		Name:  "mercosul",
		Match: parser.SetCondition{Field: "country", Op: parser.OpIn, Values: []string{"BR", "AR", "UY"}},
	}); err != nil {
		t.Fatalf("AddRule mercosul: %v", err)
	}
	if err := e.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return e
}

func TestExecuteEmitsOneSpan(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	if _, err := traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "rule.engine.execute" {
		t.Fatalf("default span name = %q, want rule.engine.execute", spans[0].Name())
	}
}

func TestWithSpanNameOverridesDefault(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"), bretotel.WithSpanName("pricing.rules.execute"))

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "pricing.rules.execute" {
		t.Fatalf("want pricing.rules.execute span, got %v", spans)
	}
}

func TestSpanCarriesAdapterAttribute(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	spans := rec.Ended()
	got := findStringAttr(t, spans[0].Attributes(), bretotel.AttrAdapter)
	if got != "*indexed.Engine" {
		t.Fatalf("AttrAdapter = %q, want *indexed.Engine", got)
	}
}

func TestSpanCarriesMatchedCountAndNames(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	spans := rec.Ended()
	attrs := spans[0].Attributes()
	count := findIntAttr(t, attrs, bretotel.AttrMatchedCount)
	names := findStringSliceAttr(t, attrs, bretotel.AttrMatchedNames)
	if count != 1 {
		t.Fatalf("matched count = %d, want 1", count)
	}
	if len(names) != 1 || names[0] != "br" {
		t.Fatalf("matched names = %v, want [br]", names)
	}
}

func TestSpanCarriesCorrelationIDWhenSet(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	ctx := engine.WithCorrelationID(context.Background(), "req-abc-123")
	_, _ = traced.Execute(ctx, engine.Request{Input: map[string]string{"country": "BR"}})

	spans := rec.Ended()
	got := findStringAttr(t, spans[0].Attributes(), bretotel.AttrCorrelationID)
	if got != "req-abc-123" {
		t.Fatalf("correlation_id = %q, want req-abc-123", got)
	}
}

func TestSpanOmitsCorrelationIDWhenAbsent(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	spans := rec.Ended()
	for _, a := range spans[0].Attributes() {
		if string(a.Key) == bretotel.AttrCorrelationID {
			t.Fatalf("correlation_id attribute should be absent, got %v", a.Value.AsString())
		}
	}
}

type failingEngine struct{}

func (failingEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, errors.New("synthetic failure")
}

type canceledEngine struct{ err error }

func (e canceledEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, e.err
}

func TestSpanRecordsErrorStatus(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(failingEngine{}, tp.Tracer("test"))

	_, err := traced.Execute(context.Background(), engine.Request{})
	if err == nil {
		t.Fatal("expected error from failing engine")
	}

	spans := rec.Ended()
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("span status code = %v, want Error", spans[0].Status().Code)
	}
	if len(spans[0].Events()) == 0 {
		t.Fatalf("span should have a recorded error event")
	}
}

func TestContextCanceledIsNotMarkedAsError(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(canceledEngine{err: context.Canceled}, tp.Tracer("test"))

	_, err := traced.Execute(context.Background(), engine.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	spans := rec.Ended()
	if spans[0].Status().Code == codes.Error {
		t.Fatalf("cancellation should not set Error status; got %v", spans[0].Status().Code)
	}
	attrs := spans[0].Attributes()
	if !findBoolAttr(t, attrs, bretotel.AttrCanceled) {
		t.Fatalf("expected AttrCanceled=true on cancellation")
	}
	if reason := findStringAttr(t, attrs, bretotel.AttrCancelReason); reason != "canceled" {
		t.Fatalf("AttrCancelReason = %q, want \"canceled\"", reason)
	}
	for _, ev := range spans[0].Events() {
		if ev.Name == "exception" {
			t.Fatalf("cancellation should not record an exception event; got %v", ev)
		}
	}
}

func TestDeadlineExceededIsNotMarkedAsError(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(canceledEngine{err: context.DeadlineExceeded}, tp.Tracer("test"))

	_, err := traced.Execute(context.Background(), engine.Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	spans := rec.Ended()
	if spans[0].Status().Code == codes.Error {
		t.Fatalf("deadline-exceeded should not set Error status; got %v", spans[0].Status().Code)
	}
	attrs := spans[0].Attributes()
	if !findBoolAttr(t, attrs, bretotel.AttrCanceled) {
		t.Fatalf("expected AttrCanceled=true on deadline exceeded")
	}
	if reason := findStringAttr(t, attrs, bretotel.AttrCancelReason); reason != "deadline_exceeded" {
		t.Fatalf("AttrCancelReason = %q, want \"deadline_exceeded\"", reason)
	}
}

func TestExecutePropagatesParentSpan(t *testing.T) {
	rec, tp := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), tp.Tracer("test"))

	parentCtx, parent := tp.Tracer("test").Start(context.Background(), "parent-op")
	_, _ = traced.Execute(parentCtx, engine.Request{Input: map[string]string{"country": "BR"}})
	parent.End()

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	var ruleSpan, parentSpan sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() == "rule.engine.execute" {
			ruleSpan = s
		}
		if s.Name() == "parent-op" {
			parentSpan = s
		}
	}
	if ruleSpan == nil || parentSpan == nil {
		t.Fatalf("missing expected spans: rule=%v parent=%v", ruleSpan, parentSpan)
	}
	if ruleSpan.Parent().SpanID() != parentSpan.SpanContext().SpanID() {
		t.Fatalf("rule span parent (%v) != parent span (%v)", ruleSpan.Parent().SpanID(), parentSpan.SpanContext().SpanID())
	}
}

func TestRuleNamesForwardsToInner(t *testing.T) {
	_, tp := newRecorder()
	inner := buildTwoRuleEngine(t)
	traced := bretotel.Wrap(inner, tp.Tracer("test"))

	rl, ok := traced.(engine.RuleLister)
	if !ok {
		t.Fatal("Wrap result should satisfy engine.RuleLister when inner does")
	}
	names := rl.RuleNames()
	if len(names) != 2 || names[0] != "br" || names[1] != "mercosul" {
		t.Fatalf("RuleNames = %v, want [br mercosul]", names)
	}
}

func TestRuleInfosForwardsToInner(t *testing.T) {
	_, tp := newRecorder()
	inner := buildTwoRuleEngine(t)
	traced := bretotel.Wrap(inner, tp.Tracer("test"))

	rl, ok := traced.(engine.RuleInfoLister)
	if !ok {
		t.Fatal("Wrap result should satisfy engine.RuleInfoLister when inner does")
	}
	infos := rl.RuleInfos()
	if len(infos) != 2 {
		t.Fatalf("RuleInfos count = %d, want 2", len(infos))
	}
}

type noOpListener struct{ called int }

func (l *noOpListener) OnRuleMatched(observability.Match) { l.called++ }

func TestAddListenerForwardsToInner(t *testing.T) {
	_, tp := newRecorder()
	inner := buildTwoRuleEngine(t)
	traced := bretotel.Wrap(inner, tp.Tracer("test"))

	lh, ok := traced.(engine.ListenerHost)
	if !ok {
		t.Fatal("Wrap result should satisfy engine.ListenerHost when inner does")
	}
	l := &noOpListener{}
	lh.AddListener(l)

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if l.called == 0 {
		t.Fatalf("forwarded listener should have fired; got %d calls", l.called)
	}
}

type noCapsEngine struct{}

func (noCapsEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, nil
}

func TestRuleNamesReturnsNilWhenInnerLacksCapability(t *testing.T) {
	_, tp := newRecorder()
	traced := bretotel.Wrap(noCapsEngine{}, tp.Tracer("test"))
	if names := traced.(interface{ RuleNames() []string }).RuleNames(); names != nil {
		t.Fatalf("RuleNames on no-caps inner = %v, want nil", names)
	}
}

func TestRuleInfosReturnsNilWhenInnerLacksCapability(t *testing.T) {
	_, tp := newRecorder()
	traced := bretotel.Wrap(noCapsEngine{}, tp.Tracer("test"))
	infos := traced.(interface {
		RuleInfos() []engine.RuleInfo
	}).RuleInfos()
	if infos != nil {
		t.Fatalf("RuleInfos on no-caps inner = %v, want nil", infos)
	}
}

func TestAddListenerNoOpWhenInnerLacksCapability(t *testing.T) {
	_, tp := newRecorder()
	traced := bretotel.Wrap(noCapsEngine{}, tp.Tracer("test"))
	// Must not panic.
	traced.(interface {
		AddListener(observability.ExecutionListener)
	}).AddListener(&noOpListener{})
}

func TestUnwrapReturnsInner(t *testing.T) {
	_, tp := newRecorder()
	inner := buildTwoRuleEngine(t)
	traced := bretotel.Wrap(inner, tp.Tracer("test"))

	type unwrapper interface{ Unwrap() engine.Engine }
	u, ok := traced.(unwrapper)
	if !ok {
		t.Fatal("Wrap result should satisfy Unwrap")
	}
	if u.Unwrap() != engine.Engine(inner) {
		t.Fatalf("Unwrap returned %v, want inner engine", u.Unwrap())
	}
}

func TestNoOpTracerProducesNoSpans(t *testing.T) {
	rec, _ := newRecorder()
	traced := bretotel.Wrap(buildTwoRuleEngine(t), oteltrace.NewNoopTracerProvider().Tracer("noop"))

	_, _ = traced.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	if len(rec.Ended()) != 0 {
		t.Fatalf("noop tracer should produce no spans; got %d", len(rec.Ended()))
	}
}

func findStringAttr(t *testing.T, attrs []attribute.KeyValue, key string) string {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	t.Fatalf("attribute %q not found", key)
	return ""
}

func findBoolAttr(t *testing.T, attrs []attribute.KeyValue, key string) bool {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsBool()
		}
	}
	t.Fatalf("attribute %q not found", key)
	return false
}

func findIntAttr(t *testing.T, attrs []attribute.KeyValue, key string) int64 {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	t.Fatalf("attribute %q not found", key)
	return 0
}

func findStringSliceAttr(t *testing.T, attrs []attribute.KeyValue, key string) []string {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsStringSlice()
		}
	}
	t.Fatalf("attribute %q not found", key)
	return nil
}
