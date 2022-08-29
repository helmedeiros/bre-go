// Package otel instruments engine.Engine with OpenTelemetry spans.
//
// One span per Execute. Matched rule names are attached as a span
// attribute, not as child spans -- per-match observability would
// produce more trace traffic than the matches themselves. See
// ADR-0042 for the design rationale.
package otel

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/observability"
)

// Standard OTel attribute keys emitted by Wrap'd engines. Exported so
// callers can filter / aggregate on them in their tracing backend.
const (
	AttrAdapter       = "rule.engine.adapter"
	AttrMatchedCount  = "rule.engine.matched.count"
	AttrMatchedNames  = "rule.engine.matched.names"
	AttrCorrelationID = "rule.engine.correlation_id"

	// AttrCanceled signals the caller canceled the context or its
	// deadline elapsed. The span carries this attribute INSTEAD of
	// codes.Error -- cancellation is caller intent, not a failure,
	// and treating it as an error inflates error-rate dashboards.
	AttrCanceled = "rule.engine.canceled"

	// AttrCancelReason names the cancellation source: "canceled"
	// (context.Canceled) or "deadline_exceeded" (context.DeadlineExceeded).
	AttrCancelReason = "rule.engine.cancel.reason"
)

const defaultSpanName = "rule.engine.execute"

// Wrap returns inner decorated with OpenTelemetry spans around every
// Execute. The returned value satisfies engine.Engine. Optional
// capability interfaces (RuleLister, RuleInfoLister, ListenerHost)
// forward to inner via the standard type-assertion idiom on the
// returned value.
func Wrap(inner engine.Engine, tracer trace.Tracer, opts ...Option) engine.Engine {
	t := &tracedEngine{
		inner:    inner,
		tracer:   tracer,
		spanName: defaultSpanName,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Option customizes the wrapper at construction time.
type Option func(*tracedEngine)

// WithSpanName overrides the default span name ("rule.engine.execute").
func WithSpanName(name string) Option {
	return func(t *tracedEngine) { t.spanName = name }
}

type tracedEngine struct {
	inner    engine.Engine
	tracer   trace.Tracer
	spanName string
}

// Execute starts a span, calls inner.Execute, and records the result
// on the span before returning.
func (t *tracedEngine) Execute(ctx context.Context, req engine.Request) (engine.Result, error) {
	ctx, span := t.tracer.Start(ctx, t.spanName)
	defer span.End()

	span.SetAttributes(attribute.String(AttrAdapter, fmt.Sprintf("%T", t.inner)))
	if cid := engine.CorrelationIDFromContext(ctx); cid != "" {
		span.SetAttributes(attribute.String(AttrCorrelationID, cid))
	}

	res, err := t.inner.Execute(ctx, req)

	span.SetAttributes(
		attribute.Int(AttrMatchedCount, len(res.Matched)),
		attribute.StringSlice(AttrMatchedNames, res.Matched),
	)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			span.SetAttributes(
				attribute.Bool(AttrCanceled, true),
				attribute.String(AttrCancelReason, "canceled"),
			)
		case errors.Is(err, context.DeadlineExceeded):
			span.SetAttributes(
				attribute.Bool(AttrCanceled, true),
				attribute.String(AttrCancelReason, "deadline_exceeded"),
			)
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}
	return res, err
}

// RuleNames forwards to inner when it implements engine.RuleLister.
// Returns nil if inner does not.
func (t *tracedEngine) RuleNames() []string {
	if rl, ok := t.inner.(engine.RuleLister); ok {
		return rl.RuleNames()
	}
	return nil
}

// RuleInfos forwards to inner when it implements engine.RuleInfoLister.
// Returns nil if inner does not.
func (t *tracedEngine) RuleInfos() []engine.RuleInfo {
	if rl, ok := t.inner.(engine.RuleInfoLister); ok {
		return rl.RuleInfos()
	}
	return nil
}

// AddListener forwards to inner when it implements engine.ListenerHost.
// No-op if inner does not.
func (t *tracedEngine) AddListener(l observability.ExecutionListener) {
	if lh, ok := t.inner.(engine.ListenerHost); ok {
		lh.AddListener(l)
	}
}

// Unwrap returns the inner engine the tracer is wrapping. Useful for
// callers that need adapter-specific methods (e.g., indexed.Engine.Build).
func (t *tracedEngine) Unwrap() engine.Engine { return t.inner }
