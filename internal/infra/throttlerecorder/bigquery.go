//go:build gcloud

package throttlerecorder

import (
	"context"
	"log/slog"
	"time"

	"cloud.google.com/go/bigquery"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type bigQueryRecord struct {
	RecordedAt   time.Time           `bigquery:"recorded_at"`
	SlotTime     time.Time           `bigquery:"slot_time"`
	SlotUnix     int64               `bigquery:"slot_unix"`
	RunID        bigquery.NullString `bigquery:"run_id"`
	RecordType   string              `bigquery:"record_type"`
	Lane         bigquery.NullString `bigquery:"lane"`
	Phase        bigquery.NullString `bigquery:"phase"`
	BeforeCount  bigquery.NullInt64  `bigquery:"before_count"`
	AfterCount   bigquery.NullInt64  `bigquery:"after_count"`
	ShiftedCount bigquery.NullInt64  `bigquery:"shifted_count"`
	PlannedCount bigquery.NullInt64  `bigquery:"planned_count"`
	SkippedCount bigquery.NullInt64  `bigquery:"skipped_count"`
	FailedCount  bigquery.NullInt64  `bigquery:"failed_count"`
	TargetCount  bigquery.NullInt64  `bigquery:"target_count"`
}

type bigQueryRecorder struct {
	client   *bigquery.Client
	inserter *bigquery.Inserter
	dataset  string
	table    string
}

func NewRecorder(ctx context.Context, cfg *Config) (domain.ThrottleResultRecorder, error) {
	if cfg.Disabled {
		slog.InfoContext(ctx, "throttle result recording disabled")
		return NewNoopRecorder(), nil
	}

	if cfg.BigQueryProjectID == "" {
		slog.WarnContext(ctx, "BigQuery project ID not configured, throttle result recording disabled")
		return NewNoopRecorder(), nil
	}

	client, err := bigquery.NewClient(ctx, cfg.BigQueryProjectID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create BigQuery client, throttle result recording disabled",
			slog.String("error", err.Error()),
			slog.String("project_id", cfg.BigQueryProjectID),
		)
		return NewNoopRecorder(), nil
	}

	table := client.Dataset(cfg.BigQueryDataset).Table(cfg.BigQueryTable)
	inserter := table.Inserter()

	slog.InfoContext(ctx, "throttle result recorder initialized",
		slog.String("type", "bigquery"),
		slog.String("project_id", cfg.BigQueryProjectID),
		slog.String("dataset", cfg.BigQueryDataset),
		slog.String("table", cfg.BigQueryTable),
	)

	return &bigQueryRecorder{
		client:   client,
		inserter: inserter,
		dataset:  cfg.BigQueryDataset,
		table:    cfg.BigQueryTable,
	}, nil
}

func (r *bigQueryRecorder) RecordBatchResults(ctx context.Context, records []domain.ThrottleResultRecord) error {
	if len(records) == 0 {
		return nil
	}

	now := time.Now()
	bqRecords := make([]*bigQueryRecord, 0, len(records))
	for _, record := range records {
		runID := record.RunID
		if runID == "" {
			runID = "default"
		}
		bqRecords = append(bqRecords, &bigQueryRecord{
			RecordedAt:   now,
			SlotTime:     record.SlotTime,
			SlotUnix:     record.SlotTime.Unix(),
			RunID:        bigquery.NullString{StringVal: runID, Valid: true},
			RecordType:   "commit",
			Lane:         bigquery.NullString{StringVal: record.Lane, Valid: true},
			Phase:        bigquery.NullString{StringVal: record.Phase, Valid: true},
			BeforeCount:  bigquery.NullInt64{Int64: int64(record.BeforeCount), Valid: true},
			AfterCount:   bigquery.NullInt64{Int64: int64(record.AfterCount), Valid: true},
			ShiftedCount: bigquery.NullInt64{Int64: int64(record.ShiftedCount), Valid: true},
			PlannedCount: bigquery.NullInt64{Int64: int64(record.PlannedCount), Valid: true},
			SkippedCount: bigquery.NullInt64{Int64: int64(record.SkippedCount), Valid: true},
			FailedCount:  bigquery.NullInt64{Int64: int64(record.FailedCount), Valid: true},
			TargetCount:  bigquery.NullInt64{Valid: false}, // NULL for commit records
		})
	}

	if err := r.inserter.Put(ctx, bqRecords); err != nil {
		slog.WarnContext(ctx, "failed to insert throttle results to BigQuery",
			slog.String("error", err.Error()),
			slog.Int("record_count", len(records)),
		)
	}

	return nil
}

func (r *bigQueryRecorder) RecordSmoothingTargets(ctx context.Context, records []domain.SmoothingTargetRecord) error {
	if len(records) == 0 {
		return nil
	}

	now := time.Now()
	bqRecords := make([]*bigQueryRecord, 0, len(records))
	for _, record := range records {
		runID := record.RunID
		if runID == "" {
			runID = "default"
		}
		bqRecords = append(bqRecords, &bigQueryRecord{
			RecordedAt:   now,
			SlotTime:     record.SlotTime,
			SlotUnix:     record.SlotTime.Unix(),
			RunID:        bigquery.NullString{StringVal: runID, Valid: true},
			RecordType:   "smoothing_target",
			Lane:         bigquery.NullString{Valid: false}, // NULL for smoothing targets
			Phase:        bigquery.NullString{Valid: false}, // NULL for smoothing targets
			BeforeCount:  bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			AfterCount:   bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			ShiftedCount: bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			PlannedCount: bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			SkippedCount: bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			FailedCount:  bigquery.NullInt64{Valid: false},  // NULL for smoothing targets
			TargetCount:  bigquery.NullInt64{Int64: int64(record.TargetCount), Valid: true},
		})
	}

	if err := r.inserter.Put(ctx, bqRecords); err != nil {
		slog.WarnContext(ctx, "failed to insert smoothing targets to BigQuery",
			slog.String("error", err.Error()),
			slog.Int("record_count", len(records)),
		)
	}

	return nil
}

func (r *bigQueryRecorder) FillAllMinutes() bool {
	return false
}

func (r *bigQueryRecorder) Flush(ctx context.Context) error {
	return nil
}

func (r *bigQueryRecorder) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
