package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const throttleTracerName = "github.com/KasumiMercury/primind-notification-throttling/internal/service/throttle"

func ThrottleTracer() trace.Tracer {
	return otel.Tracer(throttleTracerName)
}

func StartPlanningPhaseSpan(ctx context.Context, startTime, endTime time.Time) (context.Context, trace.Span) {
	return ThrottleTracer().Start(ctx, "throttle.planning_phase",
		trace.WithAttributes(
			attribute.String("phase.start", startTime.Format(time.RFC3339)),
			attribute.String("phase.end", endTime.Format(time.RFC3339)),
			attribute.Int64("phase.window_minutes", int64(endTime.Sub(startTime).Minutes())),
		),
	)
}

func StartCommitPhaseSpan(ctx context.Context, startTime, endTime time.Time) (context.Context, trace.Span) {
	return ThrottleTracer().Start(ctx, "throttle.commit_phase",
		trace.WithAttributes(
			attribute.String("phase.start", startTime.Format(time.RFC3339)),
			attribute.String("phase.end", endTime.Format(time.RFC3339)),
			attribute.Int64("phase.window_minutes", int64(endTime.Sub(startTime).Minutes())),
		),
	)
}

func StartSlotCalculationSpan(ctx context.Context, remindID, lane string) (context.Context, trace.Span) {
	return ThrottleTracer().Start(ctx, "throttle.slot_calculation",
		trace.WithAttributes(
			attribute.String("remind_id", remindID),
			attribute.String("lane", lane),
		),
	)
}

func StartExternalAPISpan(ctx context.Context, operation, url string) (context.Context, trace.Span) {
	return ThrottleTracer().Start(ctx, "throttle.external_api."+operation,
		trace.WithAttributes(
			attribute.String("url", url),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

func StartRedisOperationSpan(ctx context.Context, operation, key string) (context.Context, trace.Span) {
	return ThrottleTracer().Start(ctx, "throttle.redis."+operation,
		trace.WithAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", operation),
			attribute.String("db.key", key),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

func RecordPlanningPhaseResult(span trace.Span, plannedCount, skippedCount, shiftedCount int, err error) {
	span.SetAttributes(
		attribute.Int("planning.planned_count", plannedCount),
		attribute.Int("planning.skipped_count", skippedCount),
		attribute.Int("planning.shifted_count", shiftedCount),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

func RecordCommitPhaseResult(span trace.Span, processedCount, successCount, failedCount, skippedCount, shiftedCount int, err error) {
	span.SetAttributes(
		attribute.Int("commit.processed_count", processedCount),
		attribute.Int("commit.success_count", successCount),
		attribute.Int("commit.failed_count", failedCount),
		attribute.Int("commit.skipped_count", skippedCount),
		attribute.Int("commit.shifted_count", shiftedCount),
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

func RecordSlotCalculationResult(span trace.Span, originalTime, scheduledTime time.Time, wasShifted bool) {
	span.SetAttributes(
		attribute.String("slot.original_time", originalTime.Format(time.RFC3339)),
		attribute.String("slot.scheduled_time", scheduledTime.Format(time.RFC3339)),
		attribute.Bool("slot.was_shifted", wasShifted),
	)
	if wasShifted {
		shiftDuration := scheduledTime.Sub(originalTime)
		span.SetAttributes(attribute.Int64("slot.shift_seconds", int64(shiftDuration.Seconds())))
	}
}
