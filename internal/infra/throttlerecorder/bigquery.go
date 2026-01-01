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
	RecordedAt    time.Time `bigquery:"recorded_at"`
	VirtualMinute time.Time `bigquery:"virtual_minute"`
	Lane          string    `bigquery:"lane"`
	Phase         string    `bigquery:"phase"`
	BeforeCount   int64     `bigquery:"before_count"`
	AfterCount    int64     `bigquery:"after_count"`
	ShiftedCount  int64     `bigquery:"shifted_count"`
	PlannedCount  int64     `bigquery:"planned_count"`
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
		bqRecords = append(bqRecords, &bigQueryRecord{
			RecordedAt:    now,
			VirtualMinute: record.VirtualMinute,
			Lane:          record.Lane,
			Phase:         record.Phase,
			BeforeCount:   int64(record.BeforeCount),
			AfterCount:    int64(record.AfterCount),
			ShiftedCount:  int64(record.ShiftedCount),
			PlannedCount:  int64(record.PlannedCount),
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
