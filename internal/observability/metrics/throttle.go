package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	throttleMeterName = "throttle.service"
)

type ThrottleMetrics struct {
	packetsProcessed        metric.Int64Counter
	packetsShifted          metric.Int64Counter
	slotCalculationDuration metric.Float64Histogram
	planningPhaseDuration   metric.Float64Histogram
	commitPhaseDuration     metric.Float64Histogram
	laneDistribution        metric.Int64Counter
}

func NewThrottleMetrics() (*ThrottleMetrics, error) {
	meter := otel.Meter(throttleMeterName)

	packetsProcessed, err := meter.Int64Counter(
		"throttle_packets_total",
		metric.WithDescription("Total number of packets processed"),
		metric.WithUnit("{packet}"),
	)
	if err != nil {
		return nil, err
	}

	packetsShifted, err := meter.Int64Counter(
		"throttle_packets_shifted_total",
		metric.WithDescription("Total number of packets that were time-shifted"),
		metric.WithUnit("{packet}"),
	)
	if err != nil {
		return nil, err
	}

	slotCalculationDuration, err := meter.Float64Histogram(
		"throttle_slot_calculation_duration_seconds",
		metric.WithDescription("Time spent calculating slots"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5,
		),
	)
	if err != nil {
		return nil, err
	}

	planningPhaseDuration, err := meter.Float64Histogram(
		"throttle_planning_phase_duration_seconds",
		metric.WithDescription("Planning phase duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
		),
	)
	if err != nil {
		return nil, err
	}

	commitPhaseDuration, err := meter.Float64Histogram(
		"throttle_commit_phase_duration_seconds",
		metric.WithDescription("Commit phase duration"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60,
		),
	)
	if err != nil {
		return nil, err
	}

	laneDistribution, err := meter.Int64Counter(
		"throttle_lane_distribution_total",
		metric.WithDescription("Distribution of packets across lanes"),
		metric.WithUnit("{packet}"),
	)
	if err != nil {
		return nil, err
	}

	return &ThrottleMetrics{
		packetsProcessed:        packetsProcessed,
		packetsShifted:          packetsShifted,
		slotCalculationDuration: slotCalculationDuration,
		planningPhaseDuration:   planningPhaseDuration,
		commitPhaseDuration:     commitPhaseDuration,
		laneDistribution:        laneDistribution,
	}, nil
}

func (m *ThrottleMetrics) RecordPacketProcessed(ctx context.Context, phase, lane, outcome string) {
	m.packetsProcessed.Add(ctx, 1, metric.WithAttributes(
		attribute.String("phase", phase),
		attribute.String("lane", lane),
		attribute.String("outcome", outcome),
	))
}

func (m *ThrottleMetrics) RecordPacketShifted(ctx context.Context, phase, lane string) {
	m.packetsShifted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("phase", phase),
		attribute.String("lane", lane),
	))
}

func (m *ThrottleMetrics) RecordSlotCalculationDuration(ctx context.Context, lane string, duration time.Duration) {
	m.slotCalculationDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("lane", lane),
	))
}

func (m *ThrottleMetrics) RecordPlanningPhaseDuration(ctx context.Context, duration time.Duration) {
	m.planningPhaseDuration.Record(ctx, duration.Seconds())
}

func (m *ThrottleMetrics) RecordCommitPhaseDuration(ctx context.Context, duration time.Duration) {
	m.commitPhaseDuration.Record(ctx, duration.Seconds())
}

func (m *ThrottleMetrics) RecordLaneDistribution(ctx context.Context, lane string) {
	m.laneDistribution.Add(ctx, 1, metric.WithAttributes(
		attribute.String("lane", lane),
	))
}
