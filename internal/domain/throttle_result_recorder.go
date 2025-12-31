package domain

import (
	"context"
	"time"
)

type ThrottleResultRecord struct {
	RunID         string
	VirtualMinute time.Time
	Lane          string
	Phase         string
	BeforeCount   int
	AfterCount    int
	ShiftedCount  int
	PlannedCount  int
}

type ThrottleResultRecorder interface {
	RecordBatchResults(ctx context.Context, records []ThrottleResultRecord) error
	Flush(ctx context.Context) error
	Close() error
}
