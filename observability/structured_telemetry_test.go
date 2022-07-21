package observability_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helmedeiros/bre-go/observability"
)

func TestNewStructuredTelemetryListenerWithNilSinkPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("nil sink should have panicked")
		}
	}()
	observability.NewStructuredTelemetryListener(nil)
}

func TestOnRuleMatchedIsNoOp(t *testing.T) {
	calls := 0
	l := observability.NewStructuredTelemetryListener(func(observability.TelemetryRecord) {
		calls++
	})
	l.OnRuleMatched(observability.Match{Rule: "any"})
	if calls != 0 {
		t.Fatalf("OnRuleMatched should not emit; got %d calls", calls)
	}
}

func TestOnExecutionStartedIsNoOp(t *testing.T) {
	calls := 0
	l := observability.NewStructuredTelemetryListener(func(observability.TelemetryRecord) {
		calls++
	})
	l.OnExecutionStarted("input")
	if calls != 0 {
		t.Fatalf("OnExecutionStarted should not emit; got %d calls", calls)
	}
}

func TestOnExecutionFinishedEmitsRecordWithFullPayload(t *testing.T) {
	var got observability.TelemetryRecord
	l := observability.NewStructuredTelemetryListener(func(r observability.TelemetryRecord) {
		got = r
	})

	l.OnExecutionFinished("the-input", "the-output", []string{"r1", "r2"}, 5*time.Millisecond)

	if got.Input != "the-input" {
		t.Fatalf("Input: want the-input, got %v", got.Input)
	}
	if got.Output != "the-output" {
		t.Fatalf("Output: want the-output, got %v", got.Output)
	}
	if len(got.Matched) != 2 || got.Matched[0] != "r1" || got.Matched[1] != "r2" {
		t.Fatalf("Matched: %v", got.Matched)
	}
	if got.Duration != 5*time.Millisecond {
		t.Fatalf("Duration: %v", got.Duration)
	}
	if got.Err != nil {
		t.Fatalf("Err: want nil on success path, got %v", got.Err)
	}
}

func TestOnExecutionErroredEmitsRecordWithErrSet(t *testing.T) {
	sentinel := errors.New("boom")
	var got observability.TelemetryRecord
	l := observability.NewStructuredTelemetryListener(func(r observability.TelemetryRecord) {
		got = r
	})

	l.OnExecutionErrored("the-input", sentinel)

	if got.Input != "the-input" {
		t.Fatalf("Input: want the-input, got %v", got.Input)
	}
	if !errors.Is(got.Err, sentinel) {
		t.Fatalf("Err: want sentinel, got %v", got.Err)
	}
	if got.Output != nil {
		t.Fatalf("Output: want nil on error path, got %v", got.Output)
	}
	if got.Matched != nil {
		t.Fatalf("Matched: want nil on error path, got %v", got.Matched)
	}
}

// TestErrorPathEmitsTwoRecords documents the ADR-0038 §2 contract:
// for an Errored Execute, the sink sees both OnExecutionErrored
// AND the OnExecutionFinished that follows it.
func TestErrorPathEmitsTwoRecords(t *testing.T) {
	var recs []observability.TelemetryRecord
	l := observability.NewStructuredTelemetryListener(func(r observability.TelemetryRecord) {
		recs = append(recs, r)
	})

	sentinel := errors.New("boom")
	// Simulate the adapter's notification order on an error path:
	// Errored before Finished.
	l.OnExecutionErrored("in", sentinel)
	l.OnExecutionFinished("in", nil, nil, 3*time.Millisecond)

	if len(recs) != 2 {
		t.Fatalf("expected 2 records on error path, got %d", len(recs))
	}
	if !errors.Is(recs[0].Err, sentinel) {
		t.Fatalf("first record should carry Err, got %v", recs[0].Err)
	}
	if recs[1].Err != nil {
		t.Fatalf("second record should have Err nil, got %v", recs[1].Err)
	}
}

// TestConcurrentEmissionIsSafe drives all four lifecycle methods
// from many goroutines simultaneously. The sink must see every
// emission; the listener must not race on internal state (it has
// none, so this is a sanity check).
func TestConcurrentEmissionIsSafe(t *testing.T) {
	var count uint64
	l := observability.NewStructuredTelemetryListener(func(observability.TelemetryRecord) {
		atomic.AddUint64(&count, 1)
	})

	const goroutines = 16
	const iters = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				l.OnExecutionStarted("in")             // no emission
				l.OnRuleMatched(observability.Match{}) // no emission
				l.OnExecutionFinished("in", "out", []string{"r"}, 1*time.Millisecond)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadUint64(&count); got != goroutines*iters {
		t.Fatalf("emission count: want %d, got %d", goroutines*iters, got)
	}
}

// TestListenerImplementsAllLifecycleInterfaces is a compile-time
// check: the listener must satisfy every observability lifecycle
// role so AddListener routes events to it.
func TestListenerImplementsAllLifecycleInterfaces(t *testing.T) {
	var l observability.ExecutionListener = observability.NewStructuredTelemetryListener(
		func(observability.TelemetryRecord) {},
	)
	var _ observability.ExecutionListener = l
	var _ observability.ExecutionStartedListener = l.(*observability.StructuredTelemetryListener)
	var _ observability.ExecutionFinishedListener = l.(*observability.StructuredTelemetryListener)
	var _ observability.ExecutionErroredListener = l.(*observability.StructuredTelemetryListener)
}
