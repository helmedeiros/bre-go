package metrics_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
	"github.com/helmedeiros/bre-go/observability"
	"github.com/helmedeiros/bre-go/observability/metrics"
)

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

func TestExecuteEmitsOneMetric(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(buildTwoRuleEngine(t), sink)

	if _, err := wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	recs := sink.Records()
	if len(recs) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(recs))
	}
}

func TestMetricCarriesAdapterTypeName(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(buildTwoRuleEngine(t), sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	if got := sink.Records()[0].Adapter; got != "*indexed.Engine" {
		t.Fatalf("Adapter = %q, want *indexed.Engine", got)
	}
}

func TestMetricCarriesMatchedCountAndNames(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(buildTwoRuleEngine(t), sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	m := sink.Records()[0]
	if m.MatchedCount != 1 {
		t.Fatalf("MatchedCount = %d, want 1", m.MatchedCount)
	}
	if len(m.MatchedNames) != 1 || m.MatchedNames[0] != "br" {
		t.Fatalf("MatchedNames = %v, want [br]", m.MatchedNames)
	}
}

func TestMetricRecordsDuration(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(slowEngine{delay: 5 * time.Millisecond}, sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{})

	m := sink.Records()[0]
	if m.Duration < 5*time.Millisecond {
		t.Fatalf("Duration = %v, expected >= 5ms", m.Duration)
	}
}

type slowEngine struct{ delay time.Duration }

func (s slowEngine) Execute(ctx context.Context, _ engine.Request) (engine.Result, error) {
	time.Sleep(s.delay)
	return engine.Result{}, nil
}

func TestSuccessLeavesErrAndCanceledZero(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(buildTwoRuleEngine(t), sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})

	m := sink.Records()[0]
	if m.Err != nil || m.Canceled || m.CancelReason != "" {
		t.Fatalf("success path should have zero error/canceled fields; got %+v", m)
	}
}

type failingEngine struct{}

func (failingEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, errors.New("synthetic failure")
}

func TestErrorPathPopulatesErrButNotCanceled(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(failingEngine{}, sink)

	_, err := wrapped.Execute(context.Background(), engine.Request{})
	if err == nil {
		t.Fatal("expected error")
	}

	m := sink.Records()[0]
	if m.Err == nil {
		t.Fatalf("Err should be set; got %+v", m)
	}
	if m.Canceled || m.CancelReason != "" {
		t.Fatalf("Canceled/CancelReason should be zero for non-cancel error; got %+v", m)
	}
}

type sentinelEngine struct{ err error }

func (s sentinelEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, s.err
}

func TestContextCanceledPopulatesCanceledButNotErr(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(sentinelEngine{err: context.Canceled}, sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{})

	m := sink.Records()[0]
	if !m.Canceled || m.CancelReason != "canceled" {
		t.Fatalf("Canceled/CancelReason = %v/%q, want true/\"canceled\"", m.Canceled, m.CancelReason)
	}
	if m.Err != nil {
		t.Fatalf("Err should be nil on cancellation; got %v", m.Err)
	}
}

func TestDeadlineExceededPopulatesCanceledButNotErr(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(sentinelEngine{err: context.DeadlineExceeded}, sink)

	_, _ = wrapped.Execute(context.Background(), engine.Request{})

	m := sink.Records()[0]
	if !m.Canceled || m.CancelReason != "deadline_exceeded" {
		t.Fatalf("Canceled/CancelReason = %v/%q, want true/\"deadline_exceeded\"", m.Canceled, m.CancelReason)
	}
	if m.Err != nil {
		t.Fatalf("Err should be nil on deadline-exceeded; got %v", m.Err)
	}
}

func TestExecuteReturnsInnerOutputUnchanged(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(buildTwoRuleEngine(t), sink)

	res, err := wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Matched) != 1 || res.Matched[0] != "br" {
		t.Fatalf("caller-visible Matched should be unchanged by wrapper; got %v", res.Matched)
	}
}

func TestExecuteReturnsInnerErrorUnchanged(t *testing.T) {
	sink := &metrics.RecordingSink{}
	wrapped := metrics.Wrap(sentinelEngine{err: context.Canceled}, sink)

	_, err := wrapped.Execute(context.Background(), engine.Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wrapper must return the original context.Canceled sentinel; got %v", err)
	}
}

func TestRuleNamesForwardsToInner(t *testing.T) {
	inner := buildTwoRuleEngine(t)
	wrapped := metrics.Wrap(inner, &metrics.RecordingSink{})

	rl, ok := wrapped.(engine.RuleLister)
	if !ok {
		t.Fatal("wrapper should satisfy RuleLister when inner does")
	}
	names := rl.RuleNames()
	if len(names) != 2 || names[0] != "br" || names[1] != "mercosul" {
		t.Fatalf("RuleNames = %v, want [br mercosul]", names)
	}
}

func TestRuleInfosForwardsToInner(t *testing.T) {
	inner := buildTwoRuleEngine(t)
	wrapped := metrics.Wrap(inner, &metrics.RecordingSink{})

	rl, ok := wrapped.(engine.RuleInfoLister)
	if !ok {
		t.Fatal("wrapper should satisfy RuleInfoLister when inner does")
	}
	if len(rl.RuleInfos()) != 2 {
		t.Fatalf("RuleInfos count != 2")
	}
}

type noOpListener struct{ called int }

func (l *noOpListener) OnRuleMatched(observability.Match) { l.called++ }

func TestAddListenerForwardsToInner(t *testing.T) {
	inner := buildTwoRuleEngine(t)
	wrapped := metrics.Wrap(inner, &metrics.RecordingSink{})

	lh, ok := wrapped.(engine.ListenerHost)
	if !ok {
		t.Fatal("wrapper should satisfy ListenerHost when inner does")
	}
	l := &noOpListener{}
	lh.AddListener(l)

	_, _ = wrapped.Execute(context.Background(), engine.Request{Input: map[string]string{"country": "BR"}})
	if l.called == 0 {
		t.Fatalf("forwarded listener should have fired")
	}
}

type noCapsEngine struct{}

func (noCapsEngine) Execute(context.Context, engine.Request) (engine.Result, error) {
	return engine.Result{}, nil
}

func TestRuleNamesReturnsNilWhenInnerLacksCapability(t *testing.T) {
	wrapped := metrics.Wrap(noCapsEngine{}, &metrics.RecordingSink{})
	if names := wrapped.(interface{ RuleNames() []string }).RuleNames(); names != nil {
		t.Fatalf("expected nil; got %v", names)
	}
}

func TestRuleInfosReturnsNilWhenInnerLacksCapability(t *testing.T) {
	wrapped := metrics.Wrap(noCapsEngine{}, &metrics.RecordingSink{})
	infos := wrapped.(interface {
		RuleInfos() []engine.RuleInfo
	}).RuleInfos()
	if infos != nil {
		t.Fatalf("expected nil; got %v", infos)
	}
}

func TestAddListenerNoOpWhenInnerLacksCapability(t *testing.T) {
	wrapped := metrics.Wrap(noCapsEngine{}, &metrics.RecordingSink{})
	// must not panic
	wrapped.(interface {
		AddListener(observability.ExecutionListener)
	}).AddListener(&noOpListener{})
}

func TestUnwrapReturnsInner(t *testing.T) {
	inner := buildTwoRuleEngine(t)
	wrapped := metrics.Wrap(inner, &metrics.RecordingSink{})

	u, ok := wrapped.(interface{ Unwrap() engine.Engine })
	if !ok {
		t.Fatal("wrapper should satisfy Unwrap")
	}
	if u.Unwrap() != engine.Engine(inner) {
		t.Fatalf("Unwrap should return the inner engine")
	}
}

func TestRecordingSinkRecordsAreDefensiveCopies(t *testing.T) {
	sink := &metrics.RecordingSink{}
	sink.RecordExecution(observability.ExecutionMetric{MatchedCount: 1})

	first := sink.Records()
	first[0].MatchedCount = 999

	second := sink.Records()
	if second[0].MatchedCount != 1 {
		t.Fatalf("Records() must return a defensive copy; got %d after mutation", second[0].MatchedCount)
	}
}

func TestRecordingSinkResetClears(t *testing.T) {
	sink := &metrics.RecordingSink{}
	sink.RecordExecution(observability.ExecutionMetric{MatchedCount: 1})
	sink.RecordExecution(observability.ExecutionMetric{MatchedCount: 2})

	sink.Reset()
	if got := sink.Records(); len(got) != 0 {
		t.Fatalf("after Reset() Records() must be empty; got %v", got)
	}
}

func TestRecordingSinkSafeForConcurrentCalls(t *testing.T) {
	sink := &metrics.RecordingSink{}

	const goroutines = 16
	const perGoroutine = 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				sink.RecordExecution(observability.ExecutionMetric{MatchedCount: 1})
			}
		}()
	}
	wg.Wait()

	if got := len(sink.Records()); got != goroutines*perGoroutine {
		t.Fatalf("expected %d records, got %d", goroutines*perGoroutine, got)
	}
}
