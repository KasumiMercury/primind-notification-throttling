//go:build !gcloud

package throttlerecorder

import (
	"context"
	"log/slog"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type influxDBRecorder struct {
	client         influxdb2.Client
	writeAPI       api.WriteAPIBlocking
	bucket         string
	org            string
	fillAllMinutes bool
}

func NewRecorder(ctx context.Context, cfg *Config) (domain.ThrottleResultRecorder, error) {
	if cfg.Disabled {
		slog.InfoContext(ctx, "throttle result recording disabled")
		return NewNoopRecorder(), nil
	}

	if cfg.InfluxDBToken == "" || cfg.InfluxDBOrg == "" {
		slog.WarnContext(ctx, "InfluxDB token or org not configured, throttle result recording disabled",
			slog.String("url", cfg.InfluxDBURL),
		)
		return NewNoopRecorder(), nil
	}

	client := influxdb2.NewClient(cfg.InfluxDBURL, cfg.InfluxDBToken)
	writeAPI := client.WriteAPIBlocking(cfg.InfluxDBOrg, cfg.InfluxDBBucket)

	slog.InfoContext(ctx, "throttle result recorder initialized",
		slog.String("type", "influxdb"),
		slog.String("url", cfg.InfluxDBURL),
		slog.String("bucket", cfg.InfluxDBBucket),
	)

	return &influxDBRecorder{
		client:         client,
		writeAPI:       writeAPI,
		bucket:         cfg.InfluxDBBucket,
		org:            cfg.InfluxDBOrg,
		fillAllMinutes: cfg.FillAllMinutes,
	}, nil
}

func (r *influxDBRecorder) FillAllMinutes() bool {
	return r.fillAllMinutes
}

func (r *influxDBRecorder) RecordBatchResults(ctx context.Context, records []domain.ThrottleResultRecord) error {
	if len(records) == 0 {
		return nil
	}

	for _, record := range records {
		runID := record.RunID
		if runID == "" {
			runID = "default"
		}

		// Use real time as timestamp to prevent overwrites between iterations
		pointTime := time.Now()

		point := influxdb2.NewPoint(
			"throttle_result",
			map[string]string{
				"run_id": runID,
				"lane":   record.Lane,
				"phase":  record.Phase,
				"slot":   record.SlotTime.UTC().Format(time.RFC3339),
			},
			map[string]any{
				"before_count":  record.BeforeCount,
				"after_count":   record.AfterCount,
				"shifted_count": record.ShiftedCount,
				"planned_count": record.PlannedCount,
				"skipped_count": record.SkippedCount,
				"failed_count":  record.FailedCount,
				"slot_unix":     record.SlotTime.Unix(),
			},
			pointTime,
		)

		if err := r.writeAPI.WritePoint(ctx, point); err != nil {
			slog.WarnContext(ctx, "failed to write throttle result to InfluxDB",
				slog.String("error", err.Error()),
				slog.String("lane", record.Lane),
				slog.String("phase", record.Phase),
				slog.Time("slot", record.SlotTime),
			)
		}
	}

	return nil
}

func (r *influxDBRecorder) RecordSmoothingTargets(ctx context.Context, records []domain.SmoothingTargetRecord) error {
	if len(records) == 0 {
		return nil
	}

	for _, record := range records {
		runID := record.RunID
		if runID == "" {
			runID = "default"
		}

		pointTime := time.Now()

		point := influxdb2.NewPoint(
			"smoothing_target",
			map[string]string{
				"run_id": runID,
				"slot":   record.SlotTime.UTC().Format(time.RFC3339),
			},
			map[string]any{
				"target_count": record.TargetCount,
				"slot_unix":    record.SlotTime.Unix(),
			},
			pointTime,
		)

		if err := r.writeAPI.WritePoint(ctx, point); err != nil {
			slog.WarnContext(ctx, "failed to write smoothing target to InfluxDB",
				slog.String("error", err.Error()),
				slog.Time("slot", record.SlotTime),
			)
		}
	}

	return nil
}

func (r *influxDBRecorder) Flush(ctx context.Context) error {
	return nil
}

func (r *influxDBRecorder) Close() error {
	if r.client != nil {
		r.client.Close()
	}
	return nil
}
