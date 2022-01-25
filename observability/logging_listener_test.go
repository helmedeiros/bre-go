package observability_test

import (
	"testing"

	"github.com/helmedeiros/bre-go/observability"
)

type recordingLogger struct {
	infos  []logEvent
	errors []logEvent
}

type logEvent struct {
	msg    string
	fields []observability.Field
}

func (r *recordingLogger) Info(msg string, fields ...observability.Field) {
	r.infos = append(r.infos, logEvent{msg: msg, fields: fields})
}

func (r *recordingLogger) Error(msg string, fields ...observability.Field) {
	r.errors = append(r.errors, logEvent{msg: msg, fields: fields})
}

func TestLoggingListenerSatisfiesExecutionListener(t *testing.T) {
	var _ observability.ExecutionListener = observability.NewLoggingListener(observability.NopLogger{})
}

func TestLoggingListenerEmitsOneInfoPerMatch(t *testing.T) {
	rec := &recordingLogger{}
	l := observability.NewLoggingListener(rec)

	l.OnRuleMatched(observability.Match{Rule: "alpha"})

	if len(rec.infos) != 1 {
		t.Fatalf("infos: want 1, got %d", len(rec.infos))
	}
}

func TestLoggingListenerNeverEmitsErrors(t *testing.T) {
	rec := &recordingLogger{}
	l := observability.NewLoggingListener(rec)

	l.OnRuleMatched(observability.Match{Rule: "alpha"})

	if len(rec.errors) != 0 {
		t.Fatalf("errors: want 0, got %d", len(rec.errors))
	}
}

func TestLoggingListenerCarriesRuleNameInField(t *testing.T) {
	rec := &recordingLogger{}
	l := observability.NewLoggingListener(rec)

	l.OnRuleMatched(observability.Match{Rule: "alpha"})

	if got := fieldValue(rec.infos[0].fields, "rule"); got != "alpha" {
		t.Fatalf("field rule: want %q, got %v", "alpha", got)
	}
}

func TestLoggingListenerDoesNotLeakInput(t *testing.T) {
	rec := &recordingLogger{}
	l := observability.NewLoggingListener(rec)

	l.OnRuleMatched(observability.Match{Rule: "alpha", Input: "ssn:123"})

	if hasField(rec.infos[0].fields, "input") {
		t.Fatalf("input field present in log event; payloads must not leak")
	}
}

func TestLoggingListenerDoesNotLeakOutput(t *testing.T) {
	rec := &recordingLogger{}
	l := observability.NewLoggingListener(rec)

	l.OnRuleMatched(observability.Match{Rule: "alpha", Output: "ssn:123"})

	if hasField(rec.infos[0].fields, "output") {
		t.Fatalf("output field present in log event; payloads must not leak")
	}
}

func fieldValue(fields []observability.Field, key string) interface{} {
	for _, f := range fields {
		if f.Key == key {
			return f.Value
		}
	}
	return nil
}

func hasField(fields []observability.Field, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}
